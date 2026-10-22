package api_test

import (
	"aihub.dev/server/internal/testdb"
	"testing"
	"time"
)

func TestLocalBackupAndScopedBackgroundCredential(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	pc, phone := login(t, ts, true, "pc"), login(t, ts, false, "phone")
	at, bt := pc["token"].(string), phone["token"].(string)
	backup := map[string]any{"version": 1, "generated_at": time.Now().UTC(), "consumption": map[string]any{"10080:daily": map[string]any{"data_as_of": time.Now().UTC(), "series": []any{map[string]any{"bucket_start": "2026-10-15", "consumed_pp": 12, "coverage_hours": 2}}}}}
	if code, out := call(t, ts, "PUT", "/api/v1/local-data", at, backup); code != 200 {
		t.Fatal(code, out)
	}
	if code, out := call(t, ts, "PUT", "/api/v1/local-data", bt, backup); code != 403 {
		t.Fatal("phone overwrote desktop archive", code, out)
	}
	code, out := call(t, ts, "GET", "/api/v1/codex/consumption?window=10080&granularity=daily", bt, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	// The daily view answers with the full calendar plus source metadata; the
	// uploaded aggregate fills exactly the bucket it covered.
	if out["source"] != "local_backup" || out["data_as_of"] == nil {
		t.Fatal("source metadata missing", out["source"], out["data_as_of"])
	}
	found := false
	for _, row := range out["series"].([]any) {
		entry := row.(map[string]any)
		if entry["bucket_start"] == "2026-10-15" {
			found = entry["consumed_pp"].(float64) == 12 && entry["coverage_hours"].(float64) == 2
		}
	}
	if !found {
		t.Fatal("backup bucket not merged into the series")
	}
	if code, _ := call(t, ts, "POST", "/api/v1/mobile/background-token", at, nil); code != 403 {
		t.Fatal("desktop background credential", code)
	}
	code, out = call(t, ts, "POST", "/api/v1/mobile/background-token", bt, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	token := out["token"].(string)
	for _, endpoint := range []string{"overview", "local-data", "notifications"} {
		if code, out := call(t, ts, "GET", "/api/v1/background/"+endpoint, token, nil); code != 200 {
			t.Fatal(endpoint, code, out)
		}
	}
	if code, _ := call(t, ts, "GET", "/api/v1/me", token, nil); code != 401 {
		t.Fatal("scoped credential used for account access", code)
	}
	call(t, ts, "DELETE", "/api/v1/mobile/background-token", bt, nil)
	if code, _ := call(t, ts, "GET", "/api/v1/background/overview", token, nil); code != 401 {
		t.Fatal("revoked credential still works", code)
	}
}
