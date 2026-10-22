package api_test

import (
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/testdb"
	"net/url"
	"testing"
	"time"
)

func TestAdvisorDeliveryReservationAndRecovery(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a, b := login(t, ts, true, "pc"), login(t, ts, false, "phone")
	at, bt := a["token"].(string), b["token"].(string)
	p := codex.Defaults()
	p.QuietStart = 0
	p.QuietEnd = 0
	call(t, ts, "PATCH", "/api/v1/codex/preferences", at, p)
	_, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "Work PC"})
	observed := time.Now().UTC()
	reset := observed.Add(10 * time.Hour).Unix()
	snapshot := map[string]any{"observed_at": observed, "source": "codex_app_server", "status": "ok", "buckets": []any{
		map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 95, "duration_minutes": 10080, "resets_at": reset}},
		map[string]any{"id": "extra", "primary": map[string]any{"used_percent": 95, "duration_minutes": 10080, "resets_at": reset}},
	}}
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot); code != 200 {
		t.Fatal(code, out)
	}
	code, out := call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	alerts := out["alerts"].([]any)
	if len(alerts) != 2 {
		t.Fatal(out)
	}
	first, second := alerts[0].(map[string]any), alerts[1].(map[string]any)
	id := first["id"].(string)
	path := "/api/v1/codex/alerts/" + url.PathEscape(id) + "/delivery"
	body := map[string]any{"action": "claim", "token": first["notified_at"]}

	// Two clients of one device cannot consume the same event twice.
	_, pc := call(t, ts, "POST", path, at, body)
	if pc["claimed"] != true {
		t.Fatal(pc)
	}
	_, duplicate := call(t, ts, "POST", path, at, body)
	if duplicate["claimed"] != false {
		t.Fatal("same-device reservation duplicated", duplicate)
	}
	_, phone := call(t, ts, "POST", path, bt, body)
	if phone["claimed"] != true {
		t.Fatal("desktop receipt blocked phone", phone)
	}
	if code, out := call(t, ts, "POST", path, bt, map[string]any{"action": "release", "claim_id": pc["claim_id"]}); code != 409 {
		t.Fatal("foreign receipt modified", code, out)
	}
	for _, pair := range []struct {
		token string
		claim any
	}{{at, pc["claim_id"]}, {bt, phone["claim_id"]}} {
		if code, out := call(t, ts, "POST", path, pair.token, map[string]any{"action": "delivered", "claim_id": pair.claim}); code != 200 {
			t.Fatal(code, out)
		}
		_, again := call(t, ts, "POST", path, pair.token, body)
		if again["claimed"] != false || again["reason"] != "delivered" {
			t.Fatal("delivered event repeated", again)
		}
	}
	secondPath := "/api/v1/codex/alerts/" + url.PathEscape(second["id"].(string)) + "/delivery"
	_, spaced := call(t, ts, "POST", secondPath, bt, map[string]any{"action": "claim", "token": second["notified_at"]})
	if spaced["reason"] != "spacing" {
		t.Fatal(spaced)
	}
	// Lease recovery stays per-device, and a higher-priority event bypasses cadence.
	if _, err := database.Exec(`UPDATE codex_alert_deliveries SET delivered_at=NULL,lease_until=now()-interval '1 second',claimed_at=now()-interval '1 hour' WHERE alert_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, recovered := call(t, ts, "POST", path, at, body)
	if recovered["claimed"] != true {
		t.Fatal("expired lease failed to recover", recovered)
	}
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), at, map[string]string{"action": "read"})
	_, read := call(t, ts, "POST", path, bt, body)
	if read["claimed"] != false {
		t.Fatal("read state did not propagate", read)
	}
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), at, map[string]string{"action": "snooze"})
	_, snoozed := call(t, ts, "POST", path, bt, body)
	if snoozed["claimed"] != false {
		t.Fatal("snoozed event delivered", snoozed)
	}
}
