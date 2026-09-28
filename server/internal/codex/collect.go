package codex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"time"
)

type rpcWindow struct {
	Used     *float64 `json:"usedPercent"`
	Duration *int64   `json:"windowDurationMins"`
	Reset    *int64   `json:"resetsAt"`
}
type rpcBucket struct {
	ID        string     `json:"limitId"`
	Primary   *rpcWindow `json:"primary"`
	Secondary *rpcWindow `json:"secondary"`
}
type rpcLimits struct {
	Single  *rpcBucket           `json:"rateLimits"`
	Many    map[string]rpcBucket `json:"rateLimitsByLimitId"`
	Credits *struct {
		AvailableCount int `json:"availableCount"`
		Items          []struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			ExpiresAt *int64 `json:"expiresAt"`
		} `json:"credits"`
	} `json:"rateLimitResetCredits"`
}

// Normalize intentionally ignores emails, tokens, credits, account IDs and all other raw fields.
func Normalize(raw []byte, now time.Time) (Snapshot, error) {
	var limits rpcLimits
	out := Snapshot{ObservedAt: now.UTC(), Source: "codex_app_server", Status: "ok", Buckets: []Bucket{}}
	if json.Unmarshal(raw, &limits) != nil {
		return out, errors.New("invalid Codex response")
	}
	if limits.Credits != nil {
		out.ResetCredits = &ResetCredits{AvailableCount: limits.Credits.AvailableCount, Items: []ResetCredit{}}
		for _, c := range limits.Credits.Items {
			if c.Status == "available" {
				hash := sha256.Sum256([]byte(c.ID))
				out.ResetCredits.Items = append(out.ResetCredits.Items, ResetCredit{ID: fmt.Sprintf("%x", hash[:16]), ExpiresAt: c.ExpiresAt})
			}
		}
	}
	if len(limits.Many) == 0 && limits.Single != nil {
		id := limits.Single.ID
		if id == "" {
			id = "codex"
		}
		limits.Many = map[string]rpcBucket{id: *limits.Single}
	}
	convert := func(w *rpcWindow) *Window {
		if w == nil || w.Used == nil || w.Duration == nil {
			return nil
		}
		return &Window{UsedPercent: *w.Used, DurationMinutes: *w.Duration, ResetsAt: w.Reset}
	}
	keys := []string{}
	for k := range limits.Many {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b := limits.Many[k]
		out.Buckets = append(out.Buckets, Bucket{ID: k, Primary: convert(b.Primary), Secondary: convert(b.Secondary)})
	}
	if len(out.Buckets) == 0 {
		return out, errors.New("quota unavailable")
	}
	return out, out.Validate(now)
}

// Collect invokes only initialize and account/rateLimits/read. No turns or login changes.
// CommandContext guarantees a hung child is killed on timeout; stderr is never uploaded/logged.
func Collect(ctx context.Context, executable string) (Snapshot, error) {
	cmd := exec.CommandContext(ctx, executable, "app-server")
	hideWindow(cmd)
	cmd.Stderr = io.Discard
	input, e := cmd.StdinPipe()
	if e != nil {
		return Snapshot{}, e
	}
	output, e := cmd.StdoutPipe()
	if e != nil {
		return Snapshot{}, e
	}
	if e = cmd.Start(); e != nil {
		return Snapshot{}, errors.New("cannot start Codex app-server")
	}
	defer func() { input.Close(); cmd.Process.Kill(); cmd.Wait() }()
	enc := json.NewEncoder(input)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	send := func(id int, method string, params any) error {
		return enc.Encode(map[string]any{"id": id, "method": method, "params": params})
	}
	read := func(id int) (json.RawMessage, error) {
		for scanner.Scan() {
			var msg struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &msg) != nil {
				continue
			}
			if msg.ID != nil && *msg.ID == id {
				if len(msg.Error) > 0 {
					return nil, errors.New("Codex read unavailable; check local login")
				}
				return msg.Result, nil
			}
		}
		return nil, errors.New("Codex read timed out or disconnected")
	}
	if e = send(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aihub_bridge", "title": "AI Hub local quota bridge", "version": "0.3.0"}}); e != nil {
		return Snapshot{}, e
	}
	if _, e = read(1); e != nil {
		return Snapshot{}, e
	}
	if e = enc.Encode(map[string]string{"method": "initialized"}); e != nil {
		return Snapshot{}, e
	}
	if e = send(2, "account/rateLimits/read", nil); e != nil {
		return Snapshot{}, e
	}
	raw, e := read(2)
	if e != nil {
		return Snapshot{}, e
	}
	return Normalize(raw, time.Now())
}
