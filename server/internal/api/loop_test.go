package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"aihub.dev/server/internal/api"
	"aihub.dev/server/internal/clock"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/migrations"
)

func TestQuotaLoop(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	sqlText, err := migrations.FS.ReadFile("001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(t.Context(), database, string(sqlText)); err != nil {
		t.Fatal(err)
	}
	if err := api.SeedProviders(t.Context(), database); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	h := api.New(database, clock.Frozen{T: now}, "").Handler()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	token := postJSON(t, ts, "/api/v1/auth/register", "", map[string]string{
		"email": "dev@example.com", "password": "password1",
	})["token"].(string)

	me := getJSON(t, ts, "/api/v1/me", token)
	if me["email"] != "dev@example.com" {
		t.Fatalf("me email = %v", me["email"])
	}

	created := postJSON(t, ts, "/api/v1/provider-accounts", token, map[string]any{
		"provider_id":  "prov_cursor",
		"display_name": "工作号",
		"plan_name":    "Pro",
		"quota": map[string]any{
			"quota_type":      "credit",
			"unit":            "credit",
			"limit_value":     100,
			"remaining_value": 10,
			"reset_policy":    "fixed_time",
		},
	})
	if created["computed_status"] != "low" {
		t.Fatalf("status = %v", created["computed_status"])
	}

	dash := getJSON(t, ts, "/api/v1/dashboard", token)
	counts := dash["counts"].(map[string]any)
	if counts["low"].(float64) < 1 {
		t.Fatalf("expected low count, got %#v", counts)
	}

	inbox := getJSON(t, ts, "/api/v1/notifications", token)
	notes := inbox["notifications"].([]any)
	if len(notes) == 0 {
		t.Fatal("expected a low-quota notification")
	}

	rules := getJSON(t, ts, "/api/v1/notification-rules", token)
	first := rules["rules"].([]any)[0].(map[string]any)
	preview := postJSON(t, ts, "/api/v1/notification-rules/"+first["id"].(string)+"/preview", token, map[string]any{})
	if _, ok := preview["would_fire"]; !ok {
		t.Fatalf("preview missing would_fire: %#v", preview)
	}
}

func postJSON(t *testing.T, ts *httptest.Server, path, token string, body any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 300 {
		t.Fatalf("%s %s -> %d %#v", http.MethodPost, path, res.StatusCode, out)
	}
	return out
}

func getJSON(t *testing.T, ts *httptest.Server, path, token string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 300 {
		t.Fatalf("GET %s -> %d %#v", path, res.StatusCode, out)
	}
	return out
}
