package api_test

import (
	"aihub.dev/server/internal/testdb"
	"sync"
	"testing"
	"time"
)

func TestDeviceBindingAndManualRetry(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "owner")
	token := admin["token"].(string)
	if code, _ := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{}); code != 403 {
		t.Fatalf("registration %d", code)
	}
	const contenders = 2
	codes := make([]int, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "m0@example.com", "password": "test-password-123", "device_name": "replacement", "device_kind": "desktop", "installation_id": []string{"another-installation-a", "another-installation-b"}[i]})
		}(i)
	}
	wg.Wait()
	for _, code := range codes {
		if code != 409 {
			t.Fatalf("occupied desktop accepted: %v", codes)
		}
	}
	code, created := call(t, ts, "POST", "/api/v1/provider-accounts", token, map[string]any{"provider_id": "prov_cursor", "display_name": "manual", "plan_name": "Plus", "quota": map[string]any{"quota_type": "credit", "unit": "credit", "limit_value": 100, "remaining_value": 50}})
	if code != 201 {
		t.Fatalf("create account %d %v", code, created)
	}
	bucket := created["buckets"].([]any)[0].(map[string]any)["id"].(string)
	body := map[string]any{"operation_id": "same-offline-operation", "remaining_value": 40, "source_type": "file_import"}
	path := "/api/v1/quota-buckets/" + bucket + "/manual-snapshot"
	if code, _ = call(t, ts, "POST", path, token, body); code != 201 {
		t.Fatalf("first write %d", code)
	}
	if code, _ = call(t, ts, "POST", path, token, body); code != 200 {
		t.Fatalf("retry %d", code)
	}
	var count int
	if e := database.QueryRow(`SELECT count(*) FROM usage_snapshots WHERE quota_bucket_id=$1 AND source_type='file_import'`, bucket).Scan(&count); e != nil || count != 1 {
		t.Fatalf("duplicate snapshot %d %v", count, e)
	}
	body["remaining_value"] = 30
	if code, _ = call(t, ts, "POST", path, token, body); code != 409 {
		t.Fatalf("operation collision %d", code)
	}
}

func TestConnectorSampleIsIsolatedFromQuota(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	desktop := login(t, ts, true, "desktop")
	userToken := desktop["token"].(string)
	code, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", userToken, map[string]string{"name": "desktop"})
	if code != 201 {
		t.Fatalf("bridge: %d %v", code, bridge)
	}
	bridgeToken := bridge["token"].(string)
	sample := map[string]any{"provider_slug": "cursor", "sample_kind": "fixed_page_poc", "used_percent": 37.5, "observed_at": time.Now().UTC().Format(time.RFC3339)}
	if code, out := call(t, ts, "POST", "/api/v1/connectors/sample", bridgeToken, sample); code != 200 || out["sample_only"] != true {
		t.Fatalf("sample: %d %v", code, out)
	}
	if code, out := call(t, ts, "POST", "/api/v1/connectors/sample", bridgeToken, sample); code != 200 || out["applied"] != false {
		t.Fatalf("duplicate: %d %v", code, out)
	}
	sample["cookie"] = "secret"
	if code, _ := call(t, ts, "POST", "/api/v1/connectors/sample", bridgeToken, sample); code != 400 {
		t.Fatalf("secret field accepted: %d", code)
	}
	var count int
	_ = database.QueryRow("SELECT count(*) FROM usage_snapshots").Scan(&count)
	if count != 0 {
		t.Fatalf("sample changed quota: %d", count)
	}
	delete(sample, "cookie")
	if code, _ := call(t, ts, "POST", "/api/v1/connectors/sample", userToken, sample); code != 401 {
		t.Fatalf("account token accepted as bridge: %d", code)
	}
}

func TestNotificationSnoozeAndDismissSync(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	desktop := login(t, ts, true, "desktop")
	token := desktop["token"].(string)
	uid := desktop["user"].(map[string]any)["id"].(string)
	_, err := database.Exec(`INSERT INTO notifications(id,user_id,title,body,severity,dedupe_key,status,created_at) VALUES('ntf_action_test',$1,'Quota','Low','warning','once','unread',$2)`, uid, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/notifications/ntf_action_test/action"
	if code, _ := call(t, ts, "POST", path, token, map[string]string{"action": "snooze"}); code != 200 {
		t.Fatalf("snooze %d", code)
	}
	_, out := call(t, ts, "GET", "/api/v1/notifications", token, nil)
	status := out["notifications"].([]any)[0].(map[string]any)["status"]
	if status != "snoozed" {
		t.Fatalf("snooze status %v", status)
	}
	_, err = database.Exec(`UPDATE notifications SET snoozed_until=now()-interval '1 second' WHERE id='ntf_action_test'`)
	if err != nil {
		t.Fatal(err)
	}
	_, out = call(t, ts, "GET", "/api/v1/notifications", token, nil)
	status = out["notifications"].([]any)[0].(map[string]any)["status"]
	if status != "unread" {
		t.Fatalf("snooze did not expire: %v", status)
	}
	if code, _ := call(t, ts, "POST", path, token, map[string]string{"action": "dismiss"}); code != 200 {
		t.Fatalf("dismiss %d", code)
	}
	_, out = call(t, ts, "GET", "/api/v1/notifications", token, nil)
	status = out["notifications"].([]any)[0].(map[string]any)["status"]
	if status != "dismissed" {
		t.Fatalf("dismiss status %v", status)
	}
}

func TestAccountPatchPreservesOwnershipAndAutomaticPlan(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	owner := login(t, ts, true, "desktop")
	token := owner["token"].(string)
	code, created := call(t, ts, "POST", "/api/v1/provider-accounts", token, map[string]any{"provider_id": "prov_cursor", "display_name": "initial", "plan_name": "Manual Plus"})
	if code != 201 {
		t.Fatalf("create %d %v", code, created)
	}
	id := created["id"].(string)
	path := "/api/v1/provider-accounts/" + id
	patch := map[string]string{"display_name": "renamed", "region": "CN", "plan_name": "Manual Pro"}
	if code, _ = call(t, ts, "PATCH", path, token, patch); code != 200 {
		t.Fatalf("patch %d", code)
	}
	if code, _ = call(t, ts, "PATCH", path, token, patch); code != 200 {
		t.Fatalf("idempotent patch %d", code)
	}
	var name, plan string
	if err := database.QueryRow(`SELECT display_name FROM provider_accounts WHERE id=$1`, id).Scan(&name); err != nil || name != "renamed" {
		t.Fatalf("name %q %v", name, err)
	}
	if err := database.QueryRow(`SELECT plan_name FROM entitlements WHERE provider_account_id=$1`, id).Scan(&plan); err != nil || plan != "Manual Pro" {
		t.Fatalf("plan %q %v", plan, err)
	}
	if _, err := database.Exec(`UPDATE entitlements SET source_type='cursor_local' WHERE provider_account_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	patch["plan_name"] = "Injected"
	if code, _ = call(t, ts, "PATCH", path, token, patch); code != 409 {
		t.Fatalf("automatic plan patch %d", code)
	}
	if err := database.QueryRow(`SELECT plan_name FROM entitlements WHERE provider_account_id=$1`, id).Scan(&plan); err != nil || plan != "Manual Pro" {
		t.Fatalf("automatic plan changed %q %v", plan, err)
	}
	if code, _ = call(t, ts, "PATCH", "/api/v1/provider-accounts/no-such-account", token, patch); code != 404 {
		t.Fatalf("unknown account patch %d", code)
	}
}
