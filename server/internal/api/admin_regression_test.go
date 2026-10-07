package api_test

import (
	"aihub.dev/server/internal/testdb"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestAdminLifecycleRegression(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "owner")
	token := admin["token"].(string)
	for _, endpoint := range []string{"users", "telemetry", "operations", "invites", "registrations?status=all", "radar-token"} {
		if code, _ := call(t, ts, "GET", "/api/v1/admin/"+endpoint, token, nil); code != 200 {
			t.Fatalf("admin %s: %d", endpoint, code)
		}
	}
	for _, body := range []map[string]any{{"days": -1}, {"days": 366}, {"max_uses": 101}, {"note": strings.Repeat("x", 121)}} {
		if code, _ := call(t, ts, "POST", "/api/v1/admin/invites", token, body); code != 400 {
			t.Fatalf("invalid invitation accepted: %d", code)
		}
	}
	code, invite := call(t, ts, "POST", "/api/v1/admin/invites", token, map[string]any{"days": 1, "max_uses": 1, "note": "regression"})
	if code != 201 {
		t.Fatalf("create %d", code)
	}
	inviteID := invite["id"].(string)
	for _, status := range []string{"revoked", "active"} {
		if code, _ := call(t, ts, "PATCH", "/api/v1/admin/invites/"+inviteID+"/status", token, map[string]string{"status": status}); code != 200 {
			t.Fatalf("%s %d", status, code)
		}
	}
	_, listed := call(t, ts, "GET", "/api/v1/admin/invites", token, nil)
	if strings.Contains(fmt.Sprint(listed), invite["code"].(string)) {
		t.Fatal("invite plaintext leaked in list")
	}

	// A restored single-use invitation still cannot admit concurrent registrations twice.
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": invite["code"].(string), "email": fmt.Sprintf("concurrent-%d@example.com", i), "password": "test-password-123"})
		}(i)
	}
	wg.Wait()
	success := 0
	for _, code := range codes {
		if code == 201 {
			success++
		} else if code != 400 {
			t.Fatalf("unexpected concurrent response %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("single-use invite admitted %d registrations", success)
	}
	_, out := call(t, ts, "GET", "/api/v1/admin/registrations?status=all", token, nil)
	reg := out["registrations"].([]any)[0].(map[string]any)
	regID := reg["id"].(string)
	if code, _ := call(t, ts, "POST", "/api/v1/admin/registrations/"+regID+"/approve", token, map[string]any{}); code != 200 {
		t.Fatalf("approve %d", code)
	}
	_, out = call(t, ts, "GET", "/api/v1/admin/registrations?status=all", token, nil)
	if out["registrations"].([]any)[0].(map[string]any)["status"] != "approved" {
		t.Fatal("reviewed application missing")
	}
	code, member := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": reg["email"].(string), "password": "test-password-123", "device_kind": "desktop", "device_name": "test", "installation_id": "regression-installation-01"})
	if code != 200 {
		t.Fatalf("member login %d", code)
	}
	memberToken := member["token"].(string)
	userID := member["user"].(map[string]any)["id"].(string)
	for _, endpoint := range []string{"users", "telemetry", "operations", "invites", "registrations", "radar-token"} {
		if code, _ := call(t, ts, "GET", "/api/v1/admin/"+endpoint, memberToken, nil); code != 403 {
			t.Fatalf("member reached admin %s: %d", endpoint, code)
		}
	}
	for _, status := range []string{"disabled", "active"} {
		if code, _ := call(t, ts, "PATCH", "/api/v1/admin/users/"+userID+"/status", token, map[string]string{"status": status}); code != 200 {
			t.Fatalf("account %s %d", status, code)
		}
	}
	if code, _ := call(t, ts, "POST", "/api/v1/admin/users/"+userID+"/password", token, map[string]string{"password": "new-test-password-456"}); code != 200 {
		t.Fatalf("password reset %d", code)
	}
	if code, _ := call(t, ts, "GET", "/api/v1/me", memberToken, nil); code != 401 {
		t.Fatalf("old session survives password reset %d", code)
	}
	code, member = call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": reg["email"].(string), "password": "new-test-password-456", "device_kind": "desktop", "device_name": "test", "installation_id": "regression-installation-01"})
	if code != 200 {
		t.Fatalf("new password login %d", code)
	}
	deviceID := member["device_id"].(string)
	if code, _ := call(t, ts, "GET", "/api/v1/admin/users/"+userID+"/devices", token, nil); code != 200 {
		t.Fatalf("list devices %d", code)
	}
	if code, _ := call(t, ts, "DELETE", "/api/v1/admin/users/"+userID+"/devices/"+deviceID, token, nil); code != 200 {
		t.Fatalf("unbind %d", code)
	}
	if code, _ := call(t, ts, "GET", "/api/v1/me", member["token"].(string), nil); code != 401 {
		t.Fatalf("unbound session remains active %d", code)
	}
}
