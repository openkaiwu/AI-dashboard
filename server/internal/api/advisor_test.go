package api_test

import (
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/connector"
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
	sample := map[string]any{"observed_at": time.Now().UTC(), "source": "codex_app_server", "status": "ok", "buckets": []any{map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 95, "duration_minutes": 10080, "resets_at": reset}}}}
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", c["token"].(string), sample); code != 200 {
		t.Fatal(code, out)
	}
	if _, err := (&connector.Service{DB: database}).Evaluate(t.Context(), a["user"].(map[string]any)["id"].(string), time.Now()); err != nil {
		t.Fatal("evaluate advisor", err)
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
	// Even an unread event older than the legacy cooldown must keep its generation.
	if _, err := database.Exec(`UPDATE codex_alerts SET notified_at=now()-interval '2 days' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var oldToken time.Time
	if err := database.QueryRow(`SELECT notified_at FROM codex_alerts WHERE id=$1`, id).Scan(&oldToken); err != nil {
		t.Fatal(err)
	}
	sample["observed_at"] = time.Now().UTC()
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", c["token"].(string), sample); code != 200 {
		t.Fatal(code, out)
	}
	for _, token := range []string{at, bt, at} {
		_, current := call(t, ts, "GET", "/api/v1/codex/overview", token, nil)
		event := current["alerts"].([]any)[0].(map[string]any)
		stamp, err := time.Parse(time.RFC3339Nano, event["notified_at"].(string))
		if err != nil || !stamp.Equal(oldToken) || event["read_at"] != nil {
			t.Fatal("unread event automatically rearmed after elapsed time or fresh data", event, err)
		}
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
	if code, _ = call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), at, map[string]string{"action": "read"}); code != 200 {
		t.Fatal("read action failed", code)
	}
	if _, err := database.Exec(`UPDATE codex_alerts SET notified_at=now()-interval '2 days' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, acknowledged := call(t, ts, "GET", "/api/v1/codex/overview", bt, nil)
	read := acknowledged["alerts"].([]any)[0].(map[string]any)
	if read["read_at"] == nil || read["notified_at"] == nil {
		t.Fatal("read state not shared", read)
	}
	before := read["notified_at"]
	_, acknowledged = call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	if acknowledged["alerts"].([]any)[0].(map[string]any)["notified_at"] != before {
		t.Fatal("read event rearmed after cooldown")
	}
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), at, map[string]string{"action": "restore"})
	_, acknowledged = call(t, ts, "GET", "/api/v1/codex/overview", bt, nil)
	if acknowledged["alerts"].([]any)[0].(map[string]any)["read_at"] != nil {
		t.Fatal("restore did not clear read state")
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

func TestAdvisorQuietHoursAndRiskSnooze(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "pc")
	token := a["token"].(string)
	p := codex.Defaults()
	p.UTCOffset = 0
	p.QuietStart = time.Now().UTC().Hour()
	p.QuietEnd = (p.QuietStart + 1) % 24
	call(t, ts, "PATCH", "/api/v1/codex/preferences", token, p)
	_, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", token, map[string]string{"name": "pc"})
	reset := time.Now().Add(120 * time.Hour).Unix()
	sample := map[string]any{"observed_at": time.Now().UTC(), "source": "codex_app_server", "status": "ok", "buckets": []any{map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 95, "duration_minutes": 10080, "resets_at": reset}}}}
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), sample); code != 200 {
		t.Fatal(code, out)
	}
	_, out := call(t, ts, "GET", "/api/v1/codex/overview", token, nil)
	alert := out["alerts"].([]any)[0].(map[string]any)
	if alert["notified_at"] != nil {
		t.Fatal("quiet hours armed a new notification", alert)
	}
	p.QuietEnd = p.QuietStart
	call(t, ts, "PATCH", "/api/v1/codex/preferences", token, p)
	_, out = call(t, ts, "GET", "/api/v1/codex/overview", token, nil)
	alert = out["alerts"].([]any)[0].(map[string]any)
	if alert["notified_at"] == nil {
		t.Fatal("quiet end failed to arm active event", alert)
	}
	id := alert["id"].(string)
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), token, map[string]string{"action": "snooze"})
	_, out = call(t, ts, "GET", "/api/v1/codex/overview", token, nil)
	if out["alerts"].([]any)[0].(map[string]any)["notified_at"] != nil {
		t.Fatal("snoozed event rearmed early")
	}
	if _, err := database.Exec(`UPDATE codex_alerts SET snoozed_until=now()-interval '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, out = call(t, ts, "GET", "/api/v1/codex/overview", token, nil)
	if out["alerts"].([]any)[0].(map[string]any)["notified_at"] == nil {
		t.Fatal("one-hour snooze still waiting for six-hour cooldown")
	}
}
