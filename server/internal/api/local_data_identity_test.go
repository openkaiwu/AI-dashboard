package api_test

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/testdb"
	"testing"
	"time"
)

func snapshotAt(t time.Time, used float64, reset int64) map[string]any {
	return map[string]any{"observed_at": t, "source": "codex_app_server", "status": "ok",
		"buckets": []any{map[string]any{"id": "codex", "primary": map[string]any{"used_percent": used, "duration_minutes": 10080, "resets_at": reset}}}}
}

// Acceptance (plan 4.5): replaying the same upload ten times must not change
// any stored quantity, and each replay returns the original acceptance.
func TestLocalDataIdempotentReception(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	pc := login(t, ts, true, "pc-a")
	at := pc["token"].(string)
	_, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "PC A"})
	bt := bridge["token"].(string)
	now := time.Now().UTC()
	reset := now.Add(120 * time.Hour).Unix()
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bt, snapshotAt(now, 30, reset)); code != 200 {
		t.Fatal(code, out)
	}
	backup := map[string]any{
		"version": 1, "generated_at": now, "operation_id": "op-replay-1",
		"consumption": map[string]any{"10080:daily": map[string]any{"data_as_of": now, "series": []any{
			map[string]any{"bucket_start": now.Format("2006-01-02"), "consumed_pp": 3, "coverage_hours": 1}}}},
		"recent_snapshots": []any{snapshotAt(now.Add(-2*time.Minute), 28, reset)},
	}
	code, first := call(t, ts, "PUT", "/api/v1/local-data", at, backup)
	if code != 200 || first["duplicate"] == true {
		t.Fatal(code, first)
	}
	var history, receptions int
	if e := database.QueryRow(`SELECT count(*) FROM codex_history`).Scan(&history); e != nil {
		t.Fatal(e)
	}
	if e := database.QueryRow(`SELECT count(*) FROM local_data_receptions`).Scan(&receptions); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 9; i++ {
		code, out := call(t, ts, "PUT", "/api/v1/local-data", at, backup)
		if code != 200 || out["duplicate"] != true {
			t.Fatal("replay was not recognized as a duplicate reception", i, code, out)
		}
		if out["synced_at"] != first["synced_at"] {
			t.Fatal("replay changed reception timestamp", out, first)
		}
	}
	var historyAfter, receptionsAfter int
	if e := database.QueryRow(`SELECT count(*) FROM codex_history`).Scan(&historyAfter); e != nil {
		t.Fatal(e)
	}
	if e := database.QueryRow(`SELECT count(*) FROM local_data_receptions`).Scan(&receptionsAfter); e != nil {
		t.Fatal(e)
	}
	if historyAfter != history || receptionsAfter != receptions {
		t.Fatalf("replayed upload changed storage: history %d->%d, receptions %d->%d", history, historyAfter, receptions, receptionsAfter)
	}
}

func TestLocalDataDelayedQueueReplay(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	at := login(t, ts, true, "pc-delay")["token"].(string)
	_, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "PC"})
	now := time.Now().UTC()
	reset := now.Add(96 * time.Hour).Unix()
	call(t, ts, "POST", "/api/v1/codex/snapshot", bridge["token"].(string), snapshotAt(now, 30, reset))
	created := now.Add(-24 * time.Hour)
	backup := map[string]any{"version": 1, "generated_at": created, "operation_id": "offline-replay", "consumption": map[string]any{"10080:daily": map[string]any{"series": []any{}}}, "recent_snapshots": []any{snapshotAt(created.Add(-time.Minute), 20, reset)}}
	for i := 0; i < 2; i++ {
		if code, out := call(t, ts, "PUT", "/api/v1/local-data", at, backup); code != 200 {
			t.Fatal("delayed queue rejected", code, out)
		}
	}
}

// Acceptance (plan 4.5): a device uploading later with older data cannot push
// the account's data time or displayed values back.
func TestLocalDataLateOlderUploadDoesNotRegress(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	fresh := login(t, ts, true, "pc-new")
	// Same device replays older persisted state; the API permits one desktop.
	stale := fresh
	ft, st := fresh["token"].(string), stale["token"].(string)
	now := time.Now().UTC()
	freshBackup := map[string]any{"version": 1, "generated_at": now, "operation_id": "op-fresh",
		"consumption": map[string]any{"10080:daily": map[string]any{"data_as_of": now, "series": []any{
			map[string]any{"bucket_start": now.Format("2006-01-02"), "consumed_pp": 25, "coverage_hours": 2}}}}}
	old := now.Add(-2 * time.Hour)
	staleBackup := map[string]any{"version": 1, "generated_at": old, "operation_id": "op-stale",
		"consumption": map[string]any{"10080:daily": map[string]any{"data_as_of": old, "series": []any{
			map[string]any{"bucket_start": old.Format("2006-01-02"), "consumed_pp": 80, "coverage_hours": 2}}}}}
	if code, out := call(t, ts, "PUT", "/api/v1/local-data", ft, freshBackup); code != 200 {
		t.Fatal(code, out)
	}
	// The stale device uploads AFTER the fresh one (late upload, older data).
	if code, out := call(t, ts, "PUT", "/api/v1/local-data", st, staleBackup); code != 200 {
		t.Fatal(code, out)
	}
	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?window=10080&granularity=daily&days=7", ft, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	if out["source"] != "local_backup" {
		t.Fatal("unexpected source", out["source"])
	}
	asOf, e := time.Parse(time.RFC3339, out["data_as_of"].(string))
	if e != nil || now.Sub(asOf) > 2*time.Minute {
		t.Fatalf("data time regressed behind the late stale upload: %v (%v)", out["data_as_of"], e)
	}
	today := now.Format("2006-01-02")
	for _, row := range out["series"].([]any) {
		entry := row.(map[string]any)
		if entry["bucket_start"] == today {
			if entry["consumed_pp"].(float64) != 25 {
				t.Fatalf("displayed value regressed: %v", entry)
			}
			return
		}
	}
	t.Fatal("today's bucket missing")
}

// Plan section 2 risk "同步部分成功": when the recent-sample write cannot
// proceed (device has no bridge), nothing at all is stored - the backup row
// is rolled back with the rest of the transaction.
func TestLocalDataAtomicWhenBridgeMissing(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	pc := login(t, ts, true, "pc-lonely")
	at := pc["token"].(string)
	now := time.Now().UTC()
	reset := now.Add(48 * time.Hour).Unix()
	backup := map[string]any{"version": 1, "generated_at": now, "operation_id": "op-atomic",
		"consumption": map[string]any{"10080:daily": map[string]any{"series": []any{
			map[string]any{"bucket_start": now.Format("2006-01-02"), "consumed_pp": 5, "coverage_hours": 1}}}},
		"recent_snapshots": []any{snapshotAt(now.Add(-1*time.Minute), 20, reset)}}
	if code, out := call(t, ts, "PUT", "/api/v1/local-data", at, backup); code != 409 {
		t.Fatal("expected bridge_required rejection", code, out)
	}
	var backups int
	if e := database.QueryRow(`SELECT count(*) FROM local_data_backups`).Scan(&backups); e != nil {
		t.Fatal(e)
	}
	if backups != 0 {
		t.Fatalf("rejected upload left %d backup rows behind", backups)
	}
	if code, out := call(t, ts, "GET", "/api/v1/local-data", at, nil); code != 200 || out["empty"] != true {
		t.Fatal("partial backup survived the rejected upload", code, out)
	}
}

// Acceptance (plan 4.5): two collectors observing the same account must not
// be counted as two consumptions. The public API keeps a single active
// bridge per account, so this guard inserts two active bridges directly and
// verifies the account view stays single-sourced.
func TestConsumptionTwoActiveBridgesNotDoubleCounted(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "pc-a")
	at := a["token"].(string)
	var uid string
	if e := database.QueryRow(`SELECT id FROM users WHERE email='m0@example.com'`).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	var devA, devB string
	if e := database.QueryRow(`SELECT id FROM devices WHERE user_id=$1 AND name='pc-a'`, uid).Scan(&devA); e != nil {
		t.Fatal(e)
	}
	// Two collectors on the authorized desktop exercise source overlap without
	// weakening the account's one-active-desktop authorization invariant.
	devB = devA
	tokenA, tokenB := "bridge-token-a", "bridge-token-b"
	for _, row := range []struct{ id, device, token string }{
		{"bridge-a", devA, tokenA}, {"bridge-b", devB, tokenB},
	} {
		if _, e := database.Exec(`INSERT INTO codex_bridges(id,user_id,name,token_hash,device_id) VALUES($1,$2,$3,$4,$5)`, row.id, uid, row.device, auth.Hash(row.token), row.device); e != nil {
			t.Fatal(e)
		}
	}
	now := time.Now().UTC()
	reset := now.Add(96 * time.Hour).Unix()
	// Both collectors watch the same account: 10 percentage points consumed
	// inside the hour, each reporting its own sample pair.
	for _, post := range []struct {
		token string
		at    time.Time
		used  float64
	}{{tokenA, now.Add(-8 * time.Minute), 10}, {tokenA, now.Add(-6 * time.Minute), 20}, {tokenB, now.Add(-5 * time.Minute), 15}, {tokenB, now.Add(-3 * time.Minute), 25}} {
		if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", post.token, snapshotAt(post.at, post.used, reset)); code != 200 {
			t.Fatal(code, out)
		}
	}
	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?window=10080&granularity=daily&days=7", at, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	if out["source"] != "server_rollup" {
		t.Fatal("unexpected source", out["source"])
	}
	today := now.Format("2006-01-02")
	for _, row := range out["series"].([]any) {
		entry := row.(map[string]any)
		if entry["bucket_start"] == today {
			if got := entry["consumed_pp"].(float64); got != 10 {
				t.Fatalf("two collectors were summed: today consumed %v, expected the single-source 10", got)
			}
			return
		}
	}
	t.Fatal("today's bucket missing")
}
