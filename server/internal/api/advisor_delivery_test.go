package api_test

import (
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/testdb"
	"net/url"
	"sync"
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
	type result struct {
		token string
		code  int
		body  map[string]any
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, token := range []string{at, bt} {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			code, res := call(t, ts, "POST", path, token, body)
			results <- result{token, code, res}
		}(token)
	}
	wg.Wait()
	close(results)
	var winner result
	count := 0
	for res := range results {
		if res.code != 200 {
			t.Fatal(res)
		}
		if res.body["claimed"] == true {
			count++
			winner = res
		}
	}
	if count != 1 {
		t.Fatalf("two devices must reserve exactly once: %d", count)
	}
	loser := at
	if winner.token == at {
		loser = bt
	}
	claim := winner.body["claim_id"]
	if code, out := call(t, ts, "POST", path, loser, map[string]any{"action": "release", "claim_id": claim}); code != 409 {
		t.Fatal("another device released the reservation", code, out)
	}
	if code, out := call(t, ts, "POST", path, winner.token, map[string]any{"action": "release", "claim_id": claim}); code != 200 {
		t.Fatal(code, out)
	}
	_, retry := call(t, ts, "POST", path, loser, body)
	if retry["claimed"] != true {
		t.Fatal("released popup did not retry", retry)
	}
	if _, err := database.Exec(`UPDATE codex_alert_deliveries SET lease_until=now()-interval '1 second' WHERE alert_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, recovered := call(t, ts, "POST", path, winner.token, body)
	if recovered["claimed"] != true {
		t.Fatal("lost client lease did not recover", recovered)
	}
	if code, out := call(t, ts, "POST", path, winner.token, map[string]any{"action": "delivered", "claim_id": recovered["claim_id"]}); code != 200 {
		t.Fatal(code, out)
	}
	_, again := call(t, ts, "POST", path, loser, body)
	if again["claimed"] != false {
		t.Fatal("delivered generation repeated", again)
	}
	secondPath := "/api/v1/codex/alerts/" + url.PathEscape(second["id"].(string)) + "/delivery"
	_, spaced := call(t, ts, "POST", secondPath, loser, map[string]any{"action": "claim", "token": second["notified_at"]})
	if spaced["claimed"] != false || spaced["reason"] != "spacing" {
		t.Fatal("same-tier backlog ignored spacing", spaced)
	}

	expiry := observed.Add(30 * time.Minute).Unix()
	snapshot["observed_at"] = observed.Add(time.Millisecond)
	snapshot["reset_credits"] = map[string]any{"available_count": 1, "items": []any{map[string]any{"id": "safe-card-hash", "expires_at": expiry}}}
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot); code != 200 {
		t.Fatal(code, out)
	}
	_, overview := call(t, ts, "GET", "/api/v1/codex/overview", at, nil)
	found := false
	for _, item := range overview["alerts"].([]any) {
		alert := item.(map[string]any)
		if alert["advice"].(map[string]any)["kind"] != "credit" {
			continue
		}
		urgentPath := "/api/v1/codex/alerts/" + url.PathEscape(alert["id"].(string)) + "/delivery"
		_, urgent := call(t, ts, "POST", urgentPath, loser, map[string]any{"action": "claim", "token": alert["notified_at"]})
		if urgent["claimed"] != true {
			t.Fatal("urgent expiry blocked by ordinary cadence", urgent)
		}
		found = true
	}
	if !found {
		t.Fatal("urgent expiry missing")
	}
	_, stale := call(t, ts, "POST", path, at, map[string]any{"action": "claim", "token": time.Now().Add(-time.Hour)})
	if stale["claimed"] != false {
		t.Fatal("stale generation reserved", stale)
	}
	// Simulate a displayed, still-unread event from days ago with fresh quota data.
	if _, err := database.Exec(`WITH aged AS (
		UPDATE codex_alerts SET notified_at=now()-interval '2 days' WHERE id=$1 RETURNING user_id,id,notified_at
	) UPDATE codex_alert_deliveries d SET token=aged.notified_at,delivered_at=aged.notified_at,
	claimed_at=aged.notified_at,lease_until=aged.notified_at FROM aged
	WHERE d.user_id=aged.user_id AND d.alert_id=aged.id`, id); err != nil {
		t.Fatal(err)
	}
	var oldToken time.Time
	if err := database.QueryRow(`SELECT notified_at FROM codex_alerts WHERE id=$1`, id).Scan(&oldToken); err != nil {
		t.Fatal(err)
	}
	snapshot["observed_at"] = time.Now().UTC()
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshot); code != 200 {
		t.Fatal(code, out)
	}
	_, fresh := call(t, ts, "GET", "/api/v1/codex/overview", loser, nil)
	found = false
	for _, item := range fresh["alerts"].([]any) {
		event := item.(map[string]any)
		if event["id"] != id {
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, event["notified_at"].(string))
		if err != nil || !stamp.Equal(oldToken) {
			t.Fatal("displayed event rearmed after legacy cooldown", event, err)
		}
		body["token"] = event["notified_at"]
		found = true
	}
	if !found {
		t.Fatal("ongoing risk event disappeared", fresh)
	}
	for _, device := range []string{at, bt} {
		_, repeated := call(t, ts, "POST", path, device, body)
		if repeated["claimed"] != false || repeated["reason"] != "reserved" {
			t.Fatal("already displayed unread event repeated on another device", repeated)
		}
	}
	call(t, ts, "POST", "/api/v1/codex/alerts/"+url.PathEscape(id), at, map[string]string{"action": "read"})
	_, read := call(t, ts, "POST", path, bt, body)
	if read["claimed"] != false {
		t.Fatal("read event reserved", read)
	}
}
