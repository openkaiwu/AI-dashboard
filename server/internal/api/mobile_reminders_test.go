package api_test

import (
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/testdb"
	"testing"
	"time"
)

func TestManualSyncSuppliesObservedRapidConsumptionEvidence(t *testing.T) {
	db := testdb.Open(t)
	ts := server(t, db)
	pc, phone := login(t, ts, true, "pc"), login(t, ts, false, "phone")
	at, bt := pc["token"].(string), phone["token"].(string)
	p := codex.Defaults()
	p.QuietStart = 0
	p.QuietEnd = 0
	call(t, ts, "PATCH", "/api/v1/codex/preferences", at, p)
	call(t, ts, "POST", "/api/v1/mobile/background-token", bt, nil)
	_, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "PC"})
	now := time.Now().UTC()
	reset := now.Add(120 * time.Hour).Unix()
	makeSample := func(at time.Time, used float64) codex.Snapshot {
		return codex.Snapshot{ObservedAt: at, Source: "codex_app_server", Status: "ok", Buckets: []codex.Bucket{{ID: "codex", Primary: &codex.Window{UsedPercent: used, DurationMinutes: 10080, ResetsAt: &reset}}}}
	}
	current := makeSample(now, 35.1)
	call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), current)
	uid := pc["user"].(map[string]any)["id"].(string)
	if _, err := db.Exec(`INSERT INTO notifications(id,user_id,title,body,severity,dedupe_key,status,created_at) VALUES('mirror-news',$1,'External reset news','page only','warning','codex_alert:news:fixture','unread',$2)`, uid, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	_, before := call(t, ts, "GET", "/api/v1/mobile/reminders", bt, nil)
	if len(before["events"].([]any)) != 0 {
		t.Fatal("invented consumption from one sample", before)
	}
	body := map[string]any{"version": 1, "generated_at": now, "consumption": map[string]any{"10080:daily": map[string]any{}}, "recent_snapshots": []codex.Snapshot{makeSample(now.Add(-4*time.Minute), 30), makeSample(now.Add(-2*time.Minute), 32), current}}
	if code, out := call(t, ts, "PUT", "/api/v1/local-data", at, body); code != 200 {
		t.Fatal(code, out)
	}
	_, after := call(t, ts, "GET", "/api/v1/mobile/reminders", bt, nil)
	if len(after["events"].([]any)) != 1 {
		t.Fatal("manual evidence did not create rapid event", after)
	}
}

func TestMobileReminderBatchIsAccountWideAndPermanent(t *testing.T) {
	db := testdb.Open(t)
	ts := server(t, db)
	pc, phone := login(t, ts, true, "pc"), login(t, ts, false, "phone")
	at, bt := pc["token"].(string), phone["token"].(string)
	p := codex.Defaults()
	p.QuietStart = 0
	p.QuietEnd = 0
	call(t, ts, "PATCH", "/api/v1/codex/preferences", at, p)
	if code, _ := call(t, ts, "GET", "/api/v1/mobile/reminders", at, nil); code != 403 {
		t.Fatal("desktop can consume phone reminder")
	}
	call(t, ts, "POST", "/api/v1/mobile/background-token", bt, nil)
	_, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "PC"})
	now := time.Now().UTC()
	reset := now.Add(10 * time.Hour).Unix()
	snapshot := map[string]any{"observed_at": now, "source": "codex_app_server", "status": "ok", "buckets": []any{
		map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 100, "duration_minutes": 10080, "resets_at": reset}},
		map[string]any{"id": "other", "primary": map[string]any{"used_percent": 95, "duration_minutes": 10080, "resets_at": reset}},
	}}
	call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot)
	code, feed := call(t, ts, "GET", "/api/v1/mobile/reminders", bt, nil)
	if code != 200 || feed["primary"] != true {
		t.Fatal(code, feed)
	}
	events := feed["events"].([]any)
	if len(events) != 2 {
		t.Fatal(feed)
	}
	ids := []string{}
	for _, event := range events {
		ids = append(ids, event.(map[string]any)["id"].(string))
	}
	_, claim := call(t, ts, "POST", "/api/v1/mobile/reminders", bt, map[string]any{"action": "reserve", "ids": ids})
	if claim["claimed"] != true {
		t.Fatal(claim)
	}
	// Lost submit receipt must not permit the same notification again.
	_, again := call(t, ts, "GET", "/api/v1/mobile/reminders", bt, nil)
	if len(again["events"].([]any)) != 0 {
		t.Fatal("reserved event replayed", again)
	}
	if _, err := db.Exec(`UPDATE devices SET revoked_at=now() WHERE kind='mobile'`); err != nil {
		t.Fatal(err)
	}
	_, phone2 := func() (int, map[string]any) { return 0, login(t, ts, false, "phone-2") }()
	token2 := phone2["token"].(string)
	call(t, ts, "PUT", "/api/v1/mobile/reminders", token2, nil)
	_, next := call(t, ts, "GET", "/api/v1/mobile/reminders", token2, nil)
	if len(next["events"].([]any)) != 0 {
		t.Fatal("switching phone replayed event", next)
	}
	// Correcting a future reset doesn't manufacture a fresh event.
	snapshot["observed_at"] = now.Add(time.Second)
	snapshot["buckets"].([]any)[0].(map[string]any)["primary"].(map[string]any)["resets_at"] = reset + 60
	call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot)
	_, corrected := call(t, ts, "GET", "/api/v1/mobile/reminders", token2, nil)
	if len(corrected["events"].([]any)) != 0 {
		t.Fatal("reset correction rearmed event", corrected)
	}
	// Priority escalation does not bypass the account hard cooldown.
	expires := now.Add(45 * time.Minute).Unix()
	snapshot["observed_at"] = now.Add(2 * time.Second)
	snapshot["reset_credits"] = map[string]any{"available_count": 1, "items": []any{map[string]any{"id": "one-card", "expires_at": expires}}}
	call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot)
	_, cardFeed := call(t, ts, "GET", "/api/v1/mobile/reminders", token2, nil)
	cardEvents := cardFeed["events"].([]any)
	if len(cardEvents) != 1 {
		t.Fatal(cardFeed)
	}
	cardID := cardEvents[0].(map[string]any)["id"].(string)
	_, blocked := call(t, ts, "POST", "/api/v1/mobile/reminders", token2, map[string]any{"action": "reserve", "ids": []string{cardID}})
	if blocked["reason"] != "rate_limited" {
		t.Fatal("priority bypassed hard cooldown", blocked)
	}
	if _, err := db.Exec(`UPDATE mobile_reminder_receipts SET reserved_at=now()-interval '31 minutes'`); err != nil {
		t.Fatal(err)
	}
	_, cardClaim := call(t, ts, "POST", "/api/v1/mobile/reminders", token2, map[string]any{"action": "reserve", "ids": []string{cardID}})
	if cardClaim["claimed"] != true {
		t.Fatal(cardClaim)
	}
	// The card's last-chance priority must not create another identity.
	snapshot["observed_at"] = now.Add(3 * time.Second)
	snapshot["reset_credits"].(map[string]any)["items"].([]any)[0].(map[string]any)["expires_at"] = now.Add(5 * time.Minute).Unix()
	call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot)
	_, lastChance := call(t, ts, "GET", "/api/v1/mobile/reminders", token2, nil)
	if len(lastChance["events"].([]any)) != 0 {
		t.Fatal("card urgency rearmed", lastChance)
	}

}
