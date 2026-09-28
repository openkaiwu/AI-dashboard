package api_test

import (
	"aihub.dev/server/internal/testdb"
	"testing"
	"time"
)

func TestCursorSnapshotWritesGenericQuota(t *testing.T) {
	ts := server(t, testdb.Open(t))
	user := login(t, ts, true, "cursor-user")
	at := user["token"].(string)
	code, created := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "cursor-desk"})
	if code != 201 {
		t.Fatal(code, created)
	}
	token := created["token"].(string)
	sample := map[string]any{
		"observed_at":   time.Now().UTC().Add(-time.Minute),
		"source":        "cursor_api2",
		"status":        "ok",
		"plan_name":     "pro",
		"limit_usd":     400.0,
		"remaining_usd": 167.78,
		"used_percent":  58.05,
	}
	code, out := call(t, ts, "POST", "/api/v1/cursor/snapshot", token, sample)
	if code != 200 || out["applied"] != true {
		t.Fatal(code, out)
	}
	code, dash := call(t, ts, "GET", "/api/v1/dashboard", at, nil)
	if code != 200 {
		t.Fatal(code)
	}
	found := false
	for _, raw := range dash["accounts"].([]any) {
		acct := raw.(map[string]any)
		prov := acct["provider"].(map[string]any)
		if prov["slug"] == "cursor" {
			found = true
			buckets := acct["buckets"].([]any)
			if len(buckets) == 0 {
				t.Fatal("missing cursor bucket")
			}
			b := buckets[0].(map[string]any)
			if b["source_type"] != "cursor_api2" {
				t.Fatalf("source=%v", b["source_type"])
			}
		}
	}
	if !found {
		t.Fatal("cursor account missing from dashboard")
	}
	sample["access_token"] = "must-not-upload"
	code, _ = call(t, ts, "POST", "/api/v1/cursor/snapshot", token, sample)
	if code != 400 {
		t.Fatal("unknown secret field accepted")
	}
	code, dash = call(t, ts, "GET", "/api/v1/dashboard", at, nil)
	for _, raw := range dash["accounts"].([]any) {
		acct := raw.(map[string]any)
		if acct["provider"].(map[string]any)["slug"] != "cursor" {
			continue
		}
		for _, b := range acct["buckets"].([]any) {
			note := b.(map[string]any)["note"]
			if note != nil && note != "" && note != "stale" {
				t.Fatalf("unexpected note leak: %v", note)
			}
		}
	}
	unavail := map[string]any{
		"observed_at": time.Now().UTC(),
		"source":      "cursor_api2",
		"status":      "unavailable",
	}
	code, out = call(t, ts, "POST", "/api/v1/cursor/snapshot", token, unavail)
	if code != 200 || out["applied"] != false {
		t.Fatal("unavailable should not write quota numbers", code, out)
	}
	staleSample := map[string]any{
		"observed_at": time.Now().UTC().Add(-2 * time.Hour),
		"source":      "cursor_api2",
		"status":      "stale",
		"plan_name":   "pro",
		"limit_usd":   400.0,
		"remaining_usd": 5.0,
		"used_percent": 98.75,
		"collection_status": "api unreachable",
	}
	code, out = call(t, ts, "POST", "/api/v1/cursor/snapshot", token, staleSample)
	if code != 200 || out["applied"] != true {
		t.Fatal("stale upload failed", code, out)
	}
	_, dash = call(t, ts, "GET", "/api/v1/dashboard", at, nil)
	for _, raw := range dash["accounts"].([]any) {
		acct := raw.(map[string]any)
		if acct["provider"].(map[string]any)["slug"] != "cursor" {
			continue
		}
		b := acct["buckets"].([]any)[0].(map[string]any)
		if b["collection_status"] != "stale" {
			t.Fatalf("expected stale collection_status, got %v", b["collection_status"])
		}
		if acct["computed_status"] == "low" {
			t.Fatal("stale cursor snapshot must not surface as low quota")
		}
	}
}
