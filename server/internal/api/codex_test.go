package api_test

import (
	"aihub.dev/server/internal/testdb"
	"testing"
	"time"
)

func TestCodexBridgeAcrossDevices(t *testing.T) {
	ts := server(t, testdb.Open(t))
	desktop := login(t, ts, true, "desktop")
	mobile := login(t, ts, false, "mobile")
	at, bt := desktop["token"].(string), mobile["token"].(string)
	code, created := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "workstation"})
	if code != 201 {
		t.Fatal(code, created)
	}
	token, id := created["token"].(string), created["id"].(string)
	sample := map[string]any{"observed_at": time.Now().UTC().Add(-time.Minute), "source": "codex_app_server", "status": "ok", "buckets": []any{map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 30, "duration_minutes": 10080, "resets_at": 1790475828}, "secondary": nil}}}
	code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", token, sample)
	if code != 200 || out["applied"] != true {
		t.Fatal(code, out)
	}
	_, out = call(t, ts, "POST", "/api/v1/codex/snapshot", token, sample)
	if out["applied"] != false {
		t.Fatal("retry applied twice")
	}
	code, out = call(t, ts, "GET", "/api/v1/codex/bridges", bt, nil)
	if code != 200 || len(out["bridges"].([]any)) != 1 {
		t.Fatal(code, out)
	}
	if code, _ = call(t, ts, "GET", "/api/v1/codex/bridges", token, nil); code != 401 {
		t.Fatal("bridge token can read user data")
	}
	_, other := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"email": "other@example.com", "password": "test-password-123"})
	_, out = call(t, ts, "GET", "/api/v1/codex/bridges", other["token"].(string), nil)
	if len(out["bridges"].([]any)) != 0 {
		t.Fatal("tenant leak")
	}
	if code, _ = call(t, ts, "DELETE", "/api/v1/codex/bridges/"+id, other["token"].(string), nil); code != 404 {
		t.Fatal("cross user revoke")
	}
	sample["token"] = "must-never-store"
	if code, _ = call(t, ts, "POST", "/api/v1/codex/snapshot", token, sample); code != 400 {
		t.Fatal("unknown secret field accepted")
	}
	delete(sample, "token")
	sample["observed_at"] = time.Now().UTC().Add(-time.Hour)
	_, out = call(t, ts, "POST", "/api/v1/codex/snapshot", token, sample)
	if out["applied"] != false {
		t.Fatal("old sample replaced newer")
	}
	if code, _ = call(t, ts, "DELETE", "/api/v1/codex/bridges/"+id, bt, nil); code != 200 {
		t.Fatal("revoke failed")
	}
	if code, _ = call(t, ts, "POST", "/api/v1/codex/snapshot", token, sample); code != 401 {
		t.Fatal("revoked bridge can upload")
	}
}
