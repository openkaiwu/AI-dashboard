package api_test

import (
	"aihub.dev/server/internal/testdb"
	"testing"
)

// TestInviteRegistrationFlow covers invite-code registration and the
// administrator review lifecycle end to end.
func TestInviteRegistrationFlow(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "owner")
	token := admin["token"].(string)

	// Garbage invite codes are rejected without creating anything.
	if code, _ := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": "NOT-A-CODE", "email": "a@example.com", "password": "test-password-123"}); code != 400 {
		t.Fatalf("garbage invite %d", code)
	}

	// Mint a single-use invite and register with it.
	code, invite := call(t, ts, "POST", "/api/v1/admin/invites", token, map[string]any{"note": "guest", "days": 7, "max_uses": 1})
	if code != 201 {
		t.Fatalf("invite create %d", code)
	}
	inviteCode := invite["code"].(string)
	if invite["expires_at"] == nil || invite["id"] == nil {
		t.Fatal("invite payload incomplete")
	}

	code, _ = call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": inviteCode, "email": "member@example.com", "password": "test-password-123", "note": "请审核"})
	if code != 201 {
		t.Fatalf("register %d", code)
	}

	// Pending accounts cannot sign in and report the review state.
	code, out := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "member@example.com", "password": "test-password-123", "device_name": "phone", "device_kind": "mobile", "installation_id": "member-installation-01"})
	if code != 403 || out["error"] != "account_pending" {
		t.Fatalf("pending login %d %v", code, out)
	}

	// Duplicate registration on a pending email is refused.
	if code, out := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": inviteCode, "email": "member@example.com", "password": "test-password-123"}); code != 409 || out["error"] != "email_pending_review" {
		t.Fatalf("duplicate register %d %v", code, out)
	}

	// The administrator sees exactly one pending application.
	code, out = call(t, ts, "GET", "/api/v1/admin/registrations", token, nil)
	if code != 200 {
		t.Fatalf("registrations %d", code)
	}
	regs := out["registrations"].([]any)
	if len(regs) != 1 {
		t.Fatalf("registrations %d", len(regs))
	}
	reg := regs[0].(map[string]any)
	if reg["email"] != "member@example.com" || reg["note"] != "请审核" || reg["invite_note"] != "guest" {
		t.Fatalf("registration payload %v", reg)
	}
	regID := reg["id"].(string)

	// Unauthenticated callers never reach the invite or review endpoints.
	code, _ = call(t, ts, "GET", "/api/v1/admin/invites", "", nil)
	if code != 401 {
		t.Fatalf("unauthenticated invites %d", code)
	}

	code, _ = call(t, ts, "POST", "/api/v1/admin/registrations/"+regID+"/approve", token, nil)
	if code != 200 {
		t.Fatalf("approve %d", code)
	}
	if code, _ := call(t, ts, "POST", "/api/v1/admin/registrations/"+regID+"/approve", token, nil); code != 409 {
		t.Fatalf("re-approve %d", code)
	}

	// Approval unlocks sign-in and the normal device binding path.
	code, out = call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "member@example.com", "password": "test-password-123", "device_name": "phone", "device_kind": "mobile", "installation_id": "member-installation-01"})
	if code != 200 {
		t.Fatalf("member login %d %v", code, out)
	}
	memberToken := out["token"].(string)
	if code, _ := call(t, ts, "GET", "/api/v1/admin/invites", memberToken, nil); code != 403 {
		t.Fatalf("member invites %d", code)
	}

	// A single-use code is exhausted after one registration.
	code, out = call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": inviteCode, "email": "other@example.com", "password": "test-password-123"})
	if code != 400 || out["error"] != "invite_exhausted" {
		t.Fatalf("exhausted invite %d %v", code, out)
	}

	// Rejection records a reason and locks the email out.
	code, invite2 := call(t, ts, "POST", "/api/v1/admin/invites", token, map[string]any{"note": "guest2"})
	if code != 201 {
		t.Fatalf("invite2 create %d", code)
	}
	code, _ = call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": invite2["code"].(string), "email": "rejected@example.com", "password": "test-password-123"})
	if code != 201 {
		t.Fatalf("register2 %d", code)
	}
	code, out = call(t, ts, "GET", "/api/v1/admin/registrations", token, nil)
	if code != 200 {
		t.Fatalf("registrations2 %d", code)
	}
	var rejectedID string
	for _, item := range out["registrations"].([]any) {
		entry := item.(map[string]any)
		if entry["email"] == "rejected@example.com" {
			rejectedID = entry["id"].(string)
		}
	}
	if rejectedID == "" {
		t.Fatal("application missing")
	}
	if code, _ := call(t, ts, "POST", "/api/v1/admin/registrations/"+rejectedID+"/reject", token, map[string]string{"reason": ""}); code != 400 {
		t.Fatalf("empty reason %d", code)
	}
	if code, _ := call(t, ts, "POST", "/api/v1/admin/registrations/"+rejectedID+"/reject", token, map[string]string{"reason": "资料不全"}); code != 200 {
		t.Fatalf("reject %d", code)
	}
	if code, out := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "rejected@example.com", "password": "test-password-123", "device_name": "phone", "device_kind": "mobile", "installation_id": "rejected-installation-1"}); code != 403 || out["error"] != "account_not_authorized" {
		t.Fatalf("rejected login %d %v", code, out)
	}
	if code, out := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": invite2["code"].(string), "email": "rejected@example.com", "password": "test-password-123"}); code != 409 || out["error"] != "email_rejected" {
		t.Fatalf("re-register rejected %d %v", code, out)
	}

	// Revoked codes stop working immediately.
	code, invite3 := call(t, ts, "POST", "/api/v1/admin/invites", token, map[string]any{})
	if code != 201 {
		t.Fatalf("invite3 create %d", code)
	}
	code, _ = call(t, ts, "PATCH", "/api/v1/admin/invites/"+invite3["id"].(string)+"/status", token, map[string]string{"status": "revoked"})
	if code != 200 {
		t.Fatalf("revoke %d", code)
	}
	if code, out := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": invite3["code"].(string), "email": "revoked@example.com", "password": "test-password-123"}); code != 400 || out["error"] != "invite_revoked" {
		t.Fatalf("revoked invite %d %v", code, out)
	}

	// Expired codes are refused as well.
	code, invite4 := call(t, ts, "POST", "/api/v1/admin/invites", token, map[string]any{})
	if code != 201 {
		t.Fatalf("invite4 create %d", code)
	}
	if _, e := database.Exec(`UPDATE invite_codes SET expires_at=now()-interval '1 hour' WHERE id=$1`, invite4["id"].(string)); e != nil {
		t.Fatal(e)
	}
	if code, out := call(t, ts, "POST", "/api/v1/auth/register", "", map[string]string{"invite_code": invite4["code"].(string), "email": "expired@example.com", "password": "test-password-123"}); code != 400 || out["error"] != "invite_expired" {
		t.Fatalf("expired invite %d %v", code, out)
	}

	// The invite list reflects consumption and revocation.
	code, out = call(t, ts, "GET", "/api/v1/admin/invites", token, nil)
	if code != 200 {
		t.Fatalf("invite list %d", code)
	}
	if len(out["invites"].([]any)) != 4 {
		t.Fatalf("invite count %d", len(out["invites"].([]any)))
	}

	// Operations snapshot reports the pending backlog.
	code, out = call(t, ts, "GET", "/api/v1/admin/operations", token, nil)
	if code != 200 {
		t.Fatalf("operations %d", code)
	}
	accounts := out["accounts"].(map[string]any)
	if accounts["pending_registrations"].(float64) != 0 {
		t.Fatalf("pending registrations %v", accounts["pending_registrations"])
	}
}
