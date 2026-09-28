package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// The extension bridge accepts only paired, structured fixed-page samples on loopback.
func startExtensionBridge(ctx context.Context, client *http.Client, cfg config) {
	pair := os.Getenv("AIHUB_EXTENSION_PAIR_CODE")
	if len(pair) < 32 {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:47831")
	if err != nil {
		log.Print("extension bridge unavailable")
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sample/cursor", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !strings.HasPrefix(origin, "chrome-extension://") && !strings.HasPrefix(origin, "moz-extension://") {
			http.Error(w, "extension required", 403)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-AIHub-Pair")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", 405)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-AIHub-Pair")), []byte(pair)) != 1 {
			http.Error(w, "pairing required", 401)
			return
		}
		var q struct {
			UsedPercent float64   `json:"used_percent"`
			ObservedAt  time.Time `json:"observed_at"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&q) != nil || q.UsedPercent < 0 || q.UsedPercent > 100 || q.ObservedAt.IsZero() {
			http.Error(w, "invalid sample", 400)
			return
		}
		payload, _ := json.Marshal(map[string]any{"provider_slug": "cursor", "sample_kind": "fixed_page_poc", "used_percent": q.UsedPercent, "observed_at": q.ObservedAt.UTC()})
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(cfg.Server, "/")+"/api/v1/connectors/sample", bytes.NewReader(payload))
		if err != nil {
			http.Error(w, "unavailable", 502)
			return
		}
		request.Header.Set("Authorization", "Bearer "+cfg.Token)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			http.Error(w, "unavailable", 502)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			http.Error(w, "server rejected sample", response.StatusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, io.LimitReader(response.Body, 1024))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 25 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	go func() { _ = srv.Serve(listener) }()
}
