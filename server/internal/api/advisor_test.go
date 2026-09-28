package api_test

import (
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/testdb"
	"net/url"
	"testing"
	"time"
)

func TestAdvisorPersistenceAndSettings(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "pc")
	b := login(t, ts, false, "phone")
	at, bt := a["token"].(string), b["token"].(string)
	initial := codex.Defaults()
	initial.QuietStart = 0
	initial.QuietEnd = 0
	call(t, ts, "PATCH", "/api/v1/codex/preferences", at, initial)
	_, c := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "pc"})
	reset := time.Now().Add(10 * time.Hour).Unix()
	sample := map[string]any{"observed_at": time.Now().UTC(), "source": "codex_app_server", "status": "ok", "buckets": []any{map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 20, "duration_minutes": 10080, "resets_at": reset}}}}
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", c["token"].(string), sample); code != 200 {
		t.Fatal(code, out)
	}
	code, out := call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	alerts := out["alerts"].([]any)
	if len(alerts) != 1 {
		t.Fatal(out)
	}
	alert := alerts[0].(map[string]any)
	id := alert["id"].(string)
	_, again := call(t, ts, "GET", "/api/v1/codex/overview", bt, nil)
	if again["alerts"].([]any)[0].(map[string]any)["notified_at"] != alert["notified_at"] {
		t.Fatal("repeated poll notified again")
	}
	if code, _ = call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), bt, map[string]string{"action": "snooze"}); code != 200 {
		t.Fatal(code)
	}
	_, again = call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	if again["alerts"].([]any)[0].(map[string]any)["snoozed_until"] == nil {
		t.Fatal("snooze not shared")
	}

	if _, err := database.Exec(`UPDATE codex_alerts SET snoozed_until=now()-interval '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, woke := call(t, ts, "GET", "/api/v1/codex/overview", bt, nil)
	if woke["alerts"].([]any)[0].(map[string]any)["notified_at"] == alert["notified_at"] {
		t.Fatal("snooze did not rearm notification")
	}
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), bt, map[string]string{"action": "dismiss"})
	_, again = call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	if again["alerts"].([]any)[0].(map[string]any)["dismissed"] != true {
		t.Fatal("dismiss lost")
	}
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), bt, map[string]string{"action": "restore"})
	_, again = call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	restored := again["alerts"].([]any)[0].(map[string]any)
	if restored["dismissed"] != false || restored["snoozed_until"] != nil {
		t.Fatal("restore did not clear shared dismissal and snooze")
	}
	p := codex.Defaults()
	p.Enabled = false
	code, _ = call(t, ts, "PATCH", "/api/v1/codex/preferences", at, p)
	if code != 200 {
		t.Fatal(code)
	}
	_, again = call(t, ts, "GET", "/api/v1/codex/overview", bt, nil)
	if len(again["alerts"].([]any)) != 0 || again["preferences"].(map[string]any)["enabled"] != false {
		t.Fatal("disabled alerts still active")
	}
	p.CooldownHours = 0
	if code, _ = call(t, ts, "PATCH", "/api/v1/codex/preferences", at, p); code != 400 {
		t.Fatal("invalid settings accepted")
	}
}
