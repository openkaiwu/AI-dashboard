package api_test

import (
	"aihub.dev/server/internal/testdb"
	"database/sql"
	"net/http/httptest"
	"testing"
	"time"
)

func radarNews(checkedAt time.Time) map[string]any {
	return map[string]any{
		"checked_at": checkedAt.UTC().Format(time.RFC3339),
		"status":     "checked",
		"items": []map[string]any{{
			"url":          "https://x.com/thsottiaux/status/2106239435461579088",
			"published_at": checkedAt.UTC().Add(-30 * time.Minute).Format(time.RFC3339),
			"summary":      "Tibo 称 Pro 500 重置问题已全部修复。",
		}},
	}
}

func radarSnapshot(observed, checkedAt time.Time) map[string]any {
	return map[string]any{
		"observed_at": observed.UTC().Format(time.RFC3339),
		"source":      "codex_app_server",
		"status":      "ok",
		"buckets":     []any{},
		"news":        radarNews(checkedAt),
	}
}

func bridgeFor(t *testing.T, ts *httptest.Server, accountToken string) string {
	t.Helper()
	code, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", accountToken, map[string]string{"name": "radar-desktop"})
	if code != 201 {
		t.Fatalf("bridge %d %v", code, bridge)
	}
	return bridge["token"].(string)
}

// loginDesk creates an account through the administrator and signs in from a
// desktop device, which the bridge pairing endpoint requires.
func loginDesk(t *testing.T, ts *httptest.Server, adminToken, email string) map[string]any {
	t.Helper()
	if code, out := call(t, ts, "POST", "/api/v1/admin/users", adminToken, map[string]string{"email": email, "password": "test-password-123"}); code != 201 {
		t.Fatalf("create %s %d: %v", email, code, out)
	}
	_, session := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": email, "password": "test-password-123", "device_name": "radar-desktop", "device_kind": "desktop", "installation_id": "install-" + email})
	return session
}

func globalCheckedAt(t *testing.T, database *sql.DB) string {
	t.Helper()
	var checked string
	if e := database.QueryRow(`SELECT payload->>'checked_at' FROM global_codex_news WHERE id=1`).Scan(&checked); e != nil {
		t.Fatalf("global row: %v", e)
	}
	return checked
}

// TestGlobalNewsPromotion covers T1/T2/T4: only administrator snapshots promote
// news into the global row, and older delayed retries never roll it back.
func TestGlobalNewsPromotion(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	adminToken := admin["token"].(string)
	bridgeToken := bridgeFor(t, ts, adminToken)

	// An administrator snapshot promotes its news into the global row.
	fresh := time.Now().UTC().Add(-10 * time.Minute)
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, radarSnapshot(time.Now(), fresh)); code != 200 {
		t.Fatalf("admin upload %d %v", code, out)
	}
	if got := globalCheckedAt(t, database); got != fresh.Format(time.RFC3339) {
		t.Fatalf("global checked_at %s != %s", got, fresh.Format(time.RFC3339))
	}

	// A delayed retry carrying older news never rolls the radar back.
	// The retry carries a newer observed_at (so it is applied) but older news.
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, radarSnapshot(time.Now().Add(time.Second), fresh.Add(-2*time.Hour))); code != 200 {
		t.Fatalf("older upload rejected path changed")
	}
	if got := globalCheckedAt(t, database); got != fresh.Format(time.RFC3339) {
		t.Fatalf("monotonic guard broken: %s", got)
	}

	// A member's news is never promoted, however fresh it is.
	member := loginDesk(t, ts, adminToken, "radar-member@example.com")
	memberBridge := bridgeFor(t, ts, member["token"].(string))
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", memberBridge, radarSnapshot(time.Now(), time.Now())); code != 200 {
		t.Fatalf("member upload")
	}
	if got := globalCheckedAt(t, database); got != fresh.Format(time.RFC3339) {
		t.Fatalf("member news promoted: %s", got)
	}
}

// TestGlobalNewsDistribution covers T3/T5/T7: every account's report carries
// the promoted radar, zero-device accounts included, and a device's own newer
// check always wins over the global copy.
func TestGlobalNewsDistribution(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	adminToken := admin["token"].(string)
	bridgeToken := bridgeFor(t, ts, adminToken)
	promoted := time.Now().UTC().Add(-10 * time.Minute)
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, radarSnapshot(time.Now(), promoted)); code != 200 {
		t.Fatalf("admin upload")
	}

	// A member whose own snapshot has no news sees the promoted radar,
	// including generated news advice.
	viewer := loginDesk(t, ts, adminToken, "radar-viewer@example.com")
	viewerBridge := bridgeFor(t, ts, viewer["token"].(string))
	plain := map[string]any{"observed_at": time.Now().UTC().Format(time.RFC3339), "source": "codex_app_server", "status": "ok", "buckets": []any{}}
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", viewerBridge, plain); code != 200 {
		t.Fatalf("viewer upload")
	}
	code, out := call(t, ts, "GET", "/api/v1/codex/overview", viewer["token"].(string), nil)
	if code != 200 {
		t.Fatalf("overview %d", code)
	}
	if out["news"] == nil {
		t.Fatalf("overview missing top-level news")
	}
	devices := out["devices"].([]any)
	if len(devices) == 0 {
		t.Fatalf("viewer devices missing")
	}
	snap := devices[0].(map[string]any)["snapshot"].(map[string]any)
	if snap["news"] == nil {
		t.Fatalf("device snapshot missing promoted news")
	}
	hasNewsAdvice := false
	for _, a := range devices[0].(map[string]any)["analysis"].(map[string]any)["advice"].([]any) {
		if a.(map[string]any)["kind"] == "news" {
			hasNewsAdvice = true
		}
	}
	if !hasNewsAdvice {
		t.Fatalf("member report lacks news advice")
	}

	// A member's own newer check wins over the promoted copy.
	own := time.Now().UTC()
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", viewerBridge, radarSnapshot(own.Add(time.Second), own)); code != 200 {
		t.Fatalf("viewer own news upload")
	}
	_, out = call(t, ts, "GET", "/api/v1/codex/overview", viewer["token"].(string), nil)
	snap = out["devices"].([]any)[0].(map[string]any)["snapshot"].(map[string]any)
	if got := snap["news"].(map[string]any)["checked_at"]; got != own.Format(time.RFC3339) {
		t.Fatalf("own newer news should win: got %q want %q", got, own.Format(time.RFC3339))
	}

	// An account without any bridge still receives the radar.
	solo := loginDesk(t, ts, adminToken, "radar-solo@example.com")
	code, out = call(t, ts, "GET", "/api/v1/codex/overview", solo["token"].(string), nil)
	if code != 200 {
		t.Fatalf("solo overview %d", code)
	}
	if out["news"] == nil {
		t.Fatalf("zero-device overview missing news")
	}
	if devices := out["devices"].([]any); len(devices) != 0 {
		t.Fatalf("expected zero devices, got %d", len(devices))
	}
}
