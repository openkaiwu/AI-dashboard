// AI Hub bridge only opens outbound HTTP(S); it never exposes local credentials on a network port.
package main

import (
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/cursor"
	"aihub.dev/server/internal/provider"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type config struct {
	Server          string `json:"server"`
	Token           string `json:"token"`
	CodexPath       string `json:"codex_path"`
	CursorEnabled   *bool  `json:"cursor_enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func validServer(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/") && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")))
}

func cursorEnabled(cfg config) bool {
	if cfg.CursorEnabled != nil {
		return *cfg.CursorEnabled
	}
	return cursorAvailable()
}

func run() error {
	path := flag.String("config", "bridge.json", "Local configuration file")
	once := flag.Bool("once", false, "Collect and upload once")
	probe := flag.String("probe", "", "Read quota only without uploading: codex, cursor, or absolute codex executable path")
	flag.Parse()

	if *probe != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		switch strings.ToLower(strings.TrimSpace(*probe)) {
		case "cursor":
			token, err := cursorAuthToken()
			if err != nil {
				return err
			}
			v, err := cursor.Collect(ctx, token, cursorPlanHint())
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(v)
		case "codex":
			return errors.New("probe codex requires absolute codex executable path")
		default:
			v, e := codex.Collect(ctx, *probe)
			if e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(v)
		}
	}

	raw, e := os.ReadFile(*path)
	if e != nil {
		return errors.New("cannot read bridge config")
	}
	var cfg config
	if json.Unmarshal(raw, &cfg) != nil || !validServer(cfg.Server) || cfg.Token == "" {
		return errors.New("invalid config: HTTPS server and bridge token required")
	}
	if cfg.IntervalSeconds < 300 {
		cfg.IntervalSeconds = 300
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for {
		if cfg.CodexPath != "" && filepath.IsAbs(cfg.CodexPath) {
			uploadCodex(ctx, client, cfg, *path, *once)
		}
		if cursorEnabled(cfg) {
			uploadCursor(ctx, client, cfg, *path, *once)
		}
		if *once {
			return nil
		}
		timer := time.NewTimer(time.Duration(cfg.IntervalSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func uploadCodex(ctx context.Context, client *http.Client, cfg config, configPath string, once bool) {
	sampleCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	sample, e := codex.Collect(sampleCtx, cfg.CodexPath)
	cancel()
	if e != nil {
		sample = codex.Snapshot{ObservedAt: time.Now().UTC(), Source: "codex_app_server", Status: "unavailable", Buckets: []codex.Bucket{}}
		fmt.Println("Codex quota unavailable; check desktop login and executable path")
	}
	payload, _ := json.Marshal(sample)
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), "codex-news.json")); err == nil && len(data) < 32768 {
		var news codex.News
		if json.Unmarshal(data, &news) == nil && news.Validate(time.Now()) {
			sample.News = &news
			payload, _ = json.Marshal(sample)
		}
	}
	postSnapshot(ctx, client, cfg, provider.SlugCodex, payload, once, func() string {
		return fmt.Sprintf("codex synced: status=%s buckets=%d", sample.Status, len(sample.Buckets))
	})
}

func uploadCursor(ctx context.Context, client *http.Client, cfg config, configPath string, once bool) {
	sample := collectCursor(configPath)
	payload, _ := json.Marshal(sample)
	postSnapshot(ctx, client, cfg, provider.SlugCursor, payload, once, func() string {
		return fmt.Sprintf("cursor synced: status=%s", sample.Status)
	})
}

func postSnapshot(ctx context.Context, client *http.Client, cfg config, slug string, payload []byte, once bool, okLine func() string) {
	var path string
	for _, c := range provider.LocalConnectors() {
		if c.Slug() == slug {
			path = c.UploadPath()
			break
		}
	}
	if path == "" {
		return
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.Server, "/")+path, bytes.NewReader(payload))
	if e != nil {
		if once {
			fmt.Fprintln(os.Stderr, "invalid upload request")
		}
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	response, e := client.Do(req)
	if e != nil {
		fmt.Printf("%s upload unavailable; will retry next interval\n", slug)
		if once {
			fmt.Fprintln(os.Stderr, "upload failed")
		}
		return
	}
	io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		fmt.Fprintln(os.Stderr, "bridge authorization revoked; create a new connection")
		os.Exit(1)
	}
	if response.StatusCode != 200 {
		if once {
			fmt.Fprintln(os.Stderr, "upload rejected")
		} else {
			fmt.Printf("%s upload rejected; retrying later\n", slug)
		}
		return
	}
	fmt.Printf("%s %s\n", time.Now().UTC().Format(time.RFC3339), okLine())
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
