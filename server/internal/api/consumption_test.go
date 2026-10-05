package api_test

import (
	"aihub.dev/server/internal/testdb"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func usageSnapshot(observed time.Time, resetsAt int64, used float64) map[string]any {
	return map[string]any{
		"observed_at": observed.UTC().Format(time.RFC3339),
		"source":      "codex_app_server",
		"status":      "ok",
		"buckets": []map[string]any{{
			"id": "codex",
			"primary": map[string]any{
				"used_percent":       used,
				"duration_minutes":   10080,
				"resets_at":          resetsAt,
			},
		}},
	}
}

func sumConsumed(t *testing.T, ts *httptest.Server, token, granularity string) float64 {
	t.Helper()
	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?granularity="+granularity, token, nil)
	if code != 200 {
		t.Fatalf("consumption %d", code)
	}
	total := 0.0
	for _, e := range out["series"].([]any) {
		if e.(map[string]any)["consumed_pp"] != nil {
			total += e.(map[string]any)["consumed_pp"].(float64)
		}
	}
	return total
}

// TestConsumptionRollupAndAPI covers the consumption pipeline end to end:
// per-sample deltas, reset detection, the rollup-backed daily view and the
// raw-generation window view.
func TestConsumptionRollupAndAPI(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	token := admin["token"].(string)
	bridgeToken := bridgeFor(t, ts, token)

	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Hour)
	resetsA := base.Add(96 * time.Hour).Unix()
	resetsB := base.Add(97 * time.Hour).Unix()

	// s1: first sighting (no consumption counted).
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(base, resetsA, 10)); code != 200 {
		t.Fatalf("s1 %d %v", code, out)
	}
	// s2: same generation, +8pp consumed.
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(base.Add(time.Hour), resetsA, 18)); code != 200 {
		t.Fatalf("s2 %d %v", code, out)
	}
	// s3: the window reset (new generation), level drops to 3.
	if code, out := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(base.Add(2*time.Hour), resetsB, 3)); code != 200 {
		t.Fatalf("s3 %d %v", code, out)
	}

	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?granularity=daily", token, nil)
	if code != 200 {
		t.Fatalf("daily %d", code)
	}
	total := 0.0
	resetsTotal := 0
	for _, e := range out["series"].([]any) {
		entry := e.(map[string]any)
		if entry["consumed_pp"] != nil {
			total += entry["consumed_pp"].(float64)
		}
		if entry["resets"] != nil {
			resetsTotal += int(entry["resets"].(float64))
		}
	}
	if total != 8 {
		t.Fatalf("daily consumed total %v, want 8", total)
	}
	if resetsTotal != 1 {
		t.Fatalf("daily resets %v, want 1", resetsTotal)
	}

	// The window view returns two generations (the reset breaks the line).
	code, out = call(t, ts, "GET", "/api/v1/codex/consumption?granularity=window&window=10080", token, nil)
	if code != 200 {
		t.Fatalf("window %d", code)
	}
	gens := out["generations"].([]any)
	if len(gens) != 2 {
		t.Fatalf("generations %d, want 2", len(gens))
	}

	// The rollup table itself carries the deltas.
	var consumed float64
	if e := database.QueryRow(`SELECT COALESCE(sum(consumed_pp),0) FROM codex_usage_rollup`).Scan(&consumed); e != nil {
		t.Fatal(e)
	}
	if consumed != 8 {
		t.Fatalf("rollup consumed %v, want 8", consumed)
	}
}


// TestConsumptionHourlyAndCompare covers the hourly granularity (one-day
// panel) and the weekly compare payload (one-week panel).
func TestConsumptionHourlyAndCompare(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	token := admin["token"].(string)
	bridgeToken := bridgeFor(t, ts, token)

	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Hour)
	resetsA := base.Add(96 * time.Hour).Unix()
	resetsB := resetsA + 3600
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(base, resetsA, 10)); code != 200 {
		t.Fatalf("s1")
	}
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(base.Add(time.Hour), resetsA, 18)); code != 200 {
		t.Fatalf("s2")
	}
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(base.Add(2*time.Hour), resetsB, 3)); code != 200 {
		t.Fatalf("s3")
	}

	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?granularity=hourly&hours=24", token, nil)
	if code != 200 {
		t.Fatalf("hourly %d", code)
	}
	total, resetsTotal := 0.0, 0
	for _, e := range out["hourly"].([]any) {
		h := e.(map[string]any)
		total += h["consumed_pp"].(float64)
		resetsTotal += int(h["resets"].(float64))
	}
	if total != 8 || resetsTotal != 1 {
		t.Fatalf("hourly total %v resets %v", total, resetsTotal)
	}
	if out["plan_type"] == nil {
		t.Fatalf("hourly missing plan_type")
	}

	code, out = call(t, ts, "GET", "/api/v1/codex/consumption?granularity=weekly&compare=true", token, nil)
	if code != 200 || out["compare"] == nil || out["plan_type"] == nil {
		t.Fatalf("weekly compare %d %v", code, out)
	}
	code, out = call(t, ts, "GET", "/api/v1/codex/consumption?granularity=window&window=300", token, nil)
	if code != 200 || out["plan_type"] == nil {
		t.Fatalf("window plan_type %d %v", code, out)
	}
}

// TestConsumptionBackfill verifies that raw history inserted before the
// rollup existed is replayed into the daily view by the self-healing path.
func TestConsumptionBackfill(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	token := admin["token"].(string)
	if code, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", token, map[string]string{"name": "desktop"}); code != 201 {
		t.Fatalf("bridge %d %v", code, bridge)
	}
	var bridgeID string
	if e := database.QueryRow(`SELECT id FROM codex_bridges WHERE user_id=(SELECT id FROM users WHERE email='m0@example.com') LIMIT 1`).Scan(&bridgeID); e != nil {
		t.Fatal(e)
	}

	// Legacy raw history written directly, bypassing the bridge upload.
	observed := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	resets := observed.Add(96 * time.Hour).Unix()
	for i, used := range []float64{5, 15, 30} {
		snap := map[string]any{"observed_at": observed.Add(time.Duration(i) * time.Hour).Format(time.RFC3339), "source": "codex_app_server", "status": "ok",
			"buckets": []map[string]any{{"id": "codex", "primary": map[string]any{"used_percent": used, "duration_minutes": 10080, "resets_at": resets}}}}
		raw, _ := json.Marshal(snap)
		if _, e := database.Exec(`INSERT INTO codex_history(bridge_id,observed_at,snapshot) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, bridgeID, observed.Add(time.Duration(i)*time.Hour), string(raw)); e != nil {
			t.Fatal(e)
		}
	}

	// No rollup rows yet; the first consumption call replays the history.
	var rollups int
	if e := database.QueryRow(`SELECT count(*) FROM codex_usage_rollup`).Scan(&rollups); e != nil {
		t.Fatal(e)
	}
	if rollups != 0 {
		t.Fatalf("pre-existing rollups %d", rollups)
	}
	if total := sumConsumed(t, ts, token, "daily"); total != 25 {
		t.Fatalf("backfilled consumed %v, want 25", total)
	}
	if e := database.QueryRow(`SELECT count(*) FROM codex_usage_rollup`).Scan(&rollups); e != nil {
		t.Fatal(e)
	}
	if rollups == 0 {
		t.Fatalf("backfill did not persist rollups")
	}
}

// TestConsumptionAuthAndIsolation covers the 401 path and per-user isolation.
func TestConsumptionAuthAndIsolation(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	token := admin["token"].(string)
	bridgeToken := bridgeFor(t, ts, token)
	if code, _ := call(t, ts, "POST", "/api/v1/codex/snapshot", bridgeToken, usageSnapshot(time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(96*time.Hour).Unix(), 42)); code != 200 {
		t.Fatalf("upload")
	}

	if code, _ := call(t, ts, "GET", "/api/v1/codex/consumption?granularity=daily", "", nil); code != 401 {
		t.Fatalf("unauthenticated %d", code)
	}

	// Another account must not see the admin's consumption.
	other := loginDesk(t, ts, token, "consumption-other@example.com")
	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?granularity=daily", other["token"].(string), nil)
	if code != 200 {
		t.Fatalf("other overview %d", code)
	}
	for _, e := range out["series"].([]any) {
		if e.(map[string]any)["consumed_pp"] != nil {
			t.Fatalf("cross-user leak: %v", e)
		}
	}

	// Invalid granularity.
	if code, _ := call(t, ts, "GET", "/api/v1/codex/consumption?granularity=nope", token, nil); code != 400 {
		t.Fatalf("invalid granularity %d", code)
	}
}
