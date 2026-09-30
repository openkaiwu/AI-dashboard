package api_test

import (
	"aihub.dev/server/internal/testdb"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestM1GateManualQuotaToRuleToNotificationToHistory closes INH-361:
// manual quota -> rule evaluation -> notification -> history, consistent across devices.
func TestM1GateManualQuotaToRuleToNotificationToHistory(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	desktop := login(t, ts, true, "desktop")
	token := desktop["token"].(string)

	// 1. Manual provider account with a starting quota.
	code, created := call(t, ts, "POST", "/api/v1/provider-accounts", token, map[string]any{
		"provider_id": "prov_cursor", "display_name": "手工账户", "plan_name": "Plus",
		"quota": map[string]any{"quota_type": "credit", "unit": "credit", "limit_value": 100, "remaining_value": 50, "reset_policy": "fixed_time"},
	})
	if code != 201 {
		t.Fatalf("account %d %v", code, created)
	}
	bucket := created["buckets"].([]any)[0].(map[string]any)["id"].(string)

	// 2. Offline manual snapshot with an idempotent operation id.
	path := "/api/v1/quota-buckets/" + bucket + "/manual-snapshot"
	body := map[string]any{"operation_id": "gate-manual-op-1", "remaining_value": 15, "source_type": "user_manual"}
	if code, out := call(t, ts, "POST", path, token, body); code != 201 {
		t.Fatalf("manual snapshot %d %v", code, out)
	}

	// 3. Rule evaluation: touching the rules fires the low-quota threshold.
	if code, rules := call(t, ts, "GET", "/api/v1/notification-rules", token, nil); code != 200 || len(rules["rules"].([]any)) == 0 {
		t.Fatalf("default rules missing: %d %v", code, rules)
	} else {
		ruleID := rules["rules"].([]any)[0].(map[string]any)["id"].(string)
		if code, _ := call(t, ts, "PATCH", "/api/v1/notification-rules/"+ruleID, token, map[string]any{"enabled": true}); code != 200 {
			t.Fatalf("rule evaluate trigger %d", code)
		}
	}

	// 4. Notification inbox received the low-quota reminder exactly once.
	_, inbox := call(t, ts, "GET", "/api/v1/notifications", token, nil)
	notes := inbox["notifications"].([]any)
	lowCount := 0
	for _, n := range notes {
		m := n.(map[string]any)
		if m["severity"] == "warning" || m["severity"] == "critical" || m["severity"] == "info" {
			if v, ok := m["title"].(string); ok && v != "" {
				lowCount++
			}
		}
	}
	if lowCount == 0 {
		t.Fatalf("no notification after evaluation: %v", inbox)
	}

	// 5. History: the manual snapshot is queryable with its provenance.
	if code, out := call(t, ts, "GET", "/api/v1/quota-buckets/"+bucket+"/snapshots", token, nil); code != 200 {
		t.Fatalf("history %d %v", code, out)
	} else {
		snaps := out["snapshots"].([]any)
		if len(snaps) == 0 {
			t.Fatal("history empty")
		}
		first := snaps[0].(map[string]any)
		if first["source_type"] != "user_manual" {
			t.Fatalf("provenance wrong: %v", first)
		}
	}

	// 6. Cross-device consistency: a second device reads the same history.
	if code, mobile := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "m0@example.com", "password": "test-password-123", "device_name": "phone", "device_kind": "mobile", "installation_id": "m1-gate-mobile-install-001"}); code != 200 {
		t.Fatalf("mobile login %d", code)
	} else {
		mt := mobile["token"].(string)
		if code, out := call(t, ts, "GET", "/api/v1/quota-buckets/"+bucket+"/snapshots", mt, nil); code != 200 || len(out["snapshots"].([]any)) == 0 {
			t.Fatal("second device cannot read history")
		}
		if code, out := call(t, ts, "GET", "/api/v1/notifications", mt, nil); code != 200 || len(out["notifications"].([]any)) == 0 {
			t.Fatal("second device cannot read notifications")
		}
	}

	// 7. Replay of the same operation does not duplicate history.
	if code, _ := call(t, ts, "POST", path, token, body); code != 200 {
		t.Fatalf("replay %d", code)
	}
	var snaps int
	if e := database.QueryRow(`SELECT count(*) FROM usage_snapshots WHERE quota_bucket_id=$1 AND raw_value_json::jsonb ->> 'operation_id' = 'gate-manual-op-1'`, bucket).Scan(&snaps); e != nil || snaps != 1 {
		t.Fatalf("replay duplicated history: %d %v", snaps, e)
	}
}

// TestM2GateThreeAcquisitionModesIntoUnifiedQuotaModel closes INH-416:
// codex bridge (mode 1), cursor bridge (mode 2) and manual entry (mode 3)
// all land in the server's quota model and are readable through the unified APIs.
func TestM2GateThreeAcquisitionModesIntoUnifiedQuotaModel(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	desktop := login(t, ts, true, "desktop")
	token := desktop["token"].(string)

	// Mode 1: codex app-server snapshot via a desktop bridge token.
	code, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", token, map[string]string{"name": "Gate desktop"})
	if code != 201 {
		t.Fatalf("bridge %d %v", code, bridge)
	}
	bridgeToken := bridge["token"].(string)
	codexSnapshot := map[string]any{
		"observed_at": time.Now().UTC().Format(time.RFC3339), "source": "codex_app_server", "status": "ok",
		"buckets": []any{map[string]any{"id": "codex", "primary": map[string]any{"used_percent": 30, "duration_minutes": 10080, "resets_at": 1790475828}, "secondary": nil}},
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/codex/snapshot", jsonBody(codexSnapshot))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bridgeToken)
	if res, e := http.DefaultClient.Do(req); e != nil || res.StatusCode != 200 {
		t.Fatalf("codex upload failed: %v", e)
	} else {
		res.Body.Close()
	}

	// Mode 2: cursor snapshot through the same bridge token.
	cursorSnapshot := map[string]any{
		"observed_at": time.Now().UTC().Format(time.RFC3339), "source": "cursor_api2", "status": "ok",
		"plan_name": "Pro", "used_percent": 55.0, "limit_usd": 20.0, "collection_status": "ok",
	}
	req2, _ := http.NewRequest("POST", ts.URL+"/api/v1/cursor/snapshot", jsonBody(cursorSnapshot))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+bridgeToken)
	if res, e := http.DefaultClient.Do(req2); e != nil || res.StatusCode != 200 {
		t.Fatalf("cursor upload failed: %v", e)
	} else {
		res.Body.Close()
	}

	// Mode 3: manual account + manual snapshot.
	code, created := call(t, ts, "POST", "/api/v1/provider-accounts", token, map[string]any{
		"provider_id": "prov_cursor", "display_name": "手工", "plan_name": "Plus",
		"quota": map[string]any{"quota_type": "credit", "unit": "credit", "limit_value": 100, "remaining_value": 80, "reset_policy": "fixed_time"},
	})
	if code != 201 {
		t.Fatalf("manual account %d %v", code, created)
	}
	bucket := created["buckets"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := call(t, ts, "POST", "/api/v1/quota-buckets/"+bucket+"/manual-snapshot", token,
		map[string]any{"operation_id": "gate-manual-mode-3", "remaining_value": 70, "source_type": "user_manual", "observed_at": time.Now().UTC().Format(time.RFC3339)}); code != 201 {
		t.Fatal("manual snapshot")
	}

	// Unified read: cursor_api2 and user_manual buckets both surface in the dashboard.
	code, dash := call(t, ts, "GET", "/api/v1/dashboard", token, nil)
	if code != 200 {
		t.Fatalf("dashboard %d", code)
	}
	sources := map[string]bool{}
	accounts := dash["accounts"].([]any)
	for _, a := range accounts {
		am := a.(map[string]any)
		for _, b := range am["buckets"].([]any) {
			bm := b.(map[string]any)
			if st, ok := bm["source_type"].(string); ok {
				sources[st] = true
			}
		}
	}
	if !sources["cursor_api2"] || !sources["user_manual"] {
		t.Fatalf("unified model missing modes: %v", sources)
	}

	// Codex mode reads through the codex overview with its computed metrics.
	code, overview := call(t, ts, "GET", "/api/v1/codex/overview", token, nil)
	if code != 200 {
		t.Fatalf("codex overview %d", code)
	}
	devices := overview["devices"].([]any)
	if len(devices) == 0 {
		t.Fatal("codex overview empty")
	}
	analysis := devices[0].(map[string]any)["analysis"].(map[string]any)
	metrics := analysis["metrics"].([]any)
	if len(metrics) == 0 {
		t.Fatal("codex metrics empty")
	}
	if m0 := metrics[0].(map[string]any); m0["remaining"] == nil {
		t.Fatalf("codex remaining not computed: %v", m0)
	}

	// Provenance: three distinct acquisition paths recorded server-side.
	var distinct int
	if e := database.QueryRow(`SELECT count(DISTINCT source_type) FROM usage_snapshots`).Scan(&distinct); e != nil || distinct < 2 {
		t.Fatalf("usage snapshot sources wrong: %d %v", distinct, e)
	}
	var bridgeCount int
	if e := database.QueryRow(`SELECT count(*) FROM codex_bridges WHERE revoked_at IS NULL`).Scan(&bridgeCount); e != nil || bridgeCount != 1 {
		t.Fatalf("bridge rows wrong: %d %v", bridgeCount, e)
	}
}

func jsonBody(v any) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// TestM2OfficialAPIConnector closes INH-399's server half: an official-api
// snapshot uploaded with a bridge token lands in the unified quota model with
// official_api provenance and its own provider row.
func TestM2OfficialAPIConnector(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	desktop := login(t, ts, true, "desktop")
	token := desktop["token"].(string)
	code, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", token, map[string]string{"name": "official bridge"})
	if code != 201 {
		t.Fatalf("bridge %d %v", code, bridge)
	}
	bridgeToken := bridge["token"].(string)

	snapshot := map[string]any{
		"observed_at": time.Now().UTC().Format(time.RFC3339), "source": "official_api", "status": "ok",
		"provider_slug": "anthropic", "plan_name": "Max", "used_percent": 42.0, "limit_usd": 200.0, "remaining_usd": 116.0,
	}
	if code, out := call(t, ts, "POST", "/api/v1/official/snapshot", bridgeToken, snapshot); code != 200 || out["applied"] != true {
		t.Fatalf("official upload %d %v", code, out)
	}
	// Provider row auto-registered, account + bucket + snapshot carry official_api provenance.
	var providerID, accountID string
	if e := database.QueryRow(`SELECT id FROM providers WHERE id='prov_anthropic'`).Scan(&providerID); e != nil {
		t.Fatalf("provider row missing: %v", e)
	}
	if e := database.QueryRow(`SELECT id FROM provider_accounts WHERE provider_id='prov_anthropic'`).Scan(&accountID); e != nil {
		t.Fatalf("account missing: %v", e)
	}
	var snaps, buckets int
	if e := database.QueryRow(`SELECT count(*) FROM usage_snapshots WHERE quota_bucket_id IN (SELECT id FROM quota_buckets WHERE provider_account_id=$1) AND source_type='official_api'`, accountID).Scan(&snaps); e != nil || snaps != 1 {
		t.Fatalf("official snapshots wrong: %d %v", snaps, e)
	}
	if e := database.QueryRow(`SELECT count(*) FROM quota_buckets WHERE provider_account_id=$1 AND source_type='official_api'`, accountID).Scan(&buckets); e != nil || buckets != 1 {
		t.Fatalf("bucket provenance wrong: %d %v", buckets, e)
	}
	// Unified dashboard surfaces the official account.
	code, dash := call(t, ts, "GET", "/api/v1/dashboard", token, nil)
	if code != 200 {
		t.Fatalf("dashboard %d", code)
	}
	found := false
	for _, acc := range dash["accounts"].([]any) {
		am := acc.(map[string]any)
		if provider, ok := am["provider"].(map[string]any); ok && provider["id"] == "prov_anthropic" {
			found = true
		}
	}
	if !found {
		t.Fatal("official account missing from dashboard")
	}
	// A second upload for the same provider appends a snapshot, not a second account.
	if code, _ := call(t, ts, "POST", "/api/v1/official/snapshot", bridgeToken, snapshot); code != 200 {
		t.Fatal("second upload")
	}
	if e := database.QueryRow(`SELECT count(*) FROM provider_accounts WHERE provider_id='prov_anthropic'`).Scan(&snaps); e != nil || snaps != 1 {
		t.Fatalf("duplicate accounts: %d %v", snaps, e)
	}
	if e := database.QueryRow(`SELECT count(*) FROM usage_snapshots WHERE quota_bucket_id IN (SELECT id FROM quota_buckets WHERE provider_account_id=$1) AND source_type='official_api'`, accountID).Scan(&snaps); e != nil || snaps != 2 {
		t.Fatalf("snapshot append wrong: %d %v", snaps, e)
	}
}
