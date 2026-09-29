package api_test

import (
	"aihub.dev/server/internal/testdb"
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// loginAs creates a dedicated account and returns its session (token + user info).
func loginAs(t *testing.T, ts *httptest.Server, adminToken, email string) map[string]any {
	t.Helper()
	if code, out := call(t, ts, "POST", "/api/v1/admin/users", adminToken, map[string]string{"email": email, "password": "test-password-123"}); code != 201 {
		t.Fatalf("create %s %d: %v", email, code, out)
	}
	_, session := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": email, "password": "test-password-123", "device_name": email, "device_kind": "mobile", "installation_id": "install-" + email})
	return session
}

func TestWorkspaceACLGate(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	owner := login(t, ts, true, "desktop")
	ot := owner["token"].(string)

	editorSession := loginAs(t, ts, ot, "m5-editor@example.com")
	viewerSession := loginAs(t, ts, ot, "m5-viewer@example.com")
	outsiderSession := loginAs(t, ts, ot, "m5-outsider@example.com")
	editor := editorSession["token"].(string)
	viewer := viewerSession["token"].(string)
	outsider := outsiderSession["token"].(string)
	editorEmail := editorSession["user"].(map[string]any)["email"].(string)

	// 1. Workspace + membership lifecycle: invite by email, accept by that account only.
	code, ws := call(t, ts, "POST", "/api/v1/workspaces", ot, map[string]string{"name": "团队空间"})
	if code != 201 {
		t.Fatalf("workspace %d: %v", code, ws)
	}
	wsID := ws["id"].(string)
	for _, role := range []string{"editor", "viewer"} {
		code, inv := call(t, ts, "POST", "/api/v1/workspaces/"+wsID+"/invites", ot, map[string]string{"role": role, "email": emailFor(role)})
		if code != 201 {
			t.Fatalf("invite %d: %v", code, inv)
		}
	}
	_, invites := call(t, ts, "GET", "/api/v1/workspaces/"+wsID+"/invites", ot, nil)
	inviteRows := invites["invites"].([]any)
	if len(inviteRows) != 2 {
		t.Fatalf("invites wrong: %v", invites)
	}
	var editorInvite, viewerInvite string
	for _, row := range inviteRows {
		m := row.(map[string]any)
		if m["email"] == "m5-editor@example.com" {
			editorInvite = m["id"].(string)
		}
		if m["email"] == "m5-viewer@example.com" {
			viewerInvite = m["id"].(string)
		}
	}
	if code, _ := call(t, ts, "POST", "/api/v1/invites/"+editorInvite+"/accept", outsider, nil); code != 404 {
		t.Fatalf("wrong account accepted invite %d", code)
	}
	if code, _ := call(t, ts, "POST", "/api/v1/invites/"+editorInvite+"/accept", editor, nil); code != 200 {
		t.Fatal("editor accept")
	}
	if code, _ := call(t, ts, "POST", "/api/v1/invites/"+viewerInvite+"/accept", viewer, nil); code != 200 {
		t.Fatal("viewer accept")
	}

	// 2. Owner shares a conversation and a config asset into the workspace.
	if code, _ := importFile(t, ts, ot, "chatgpt_export", "testdata/chatgpt_export.json"); code != 202 {
		t.Fatal("import rejected")
	}
	processImports(t, database)
	_, list := call(t, ts, "GET", "/api/v1/conversations", ot, nil)
	convID := list["conversations"].([]any)[0].(map[string]any)["id"].(string)
	if code, out := call(t, ts, "PATCH", "/api/v1/conversations/"+convID, ot, map[string]any{"workspace_id": wsID}); code != 200 {
		t.Fatalf("share conv %d: %v", code, out)
	}
	code, asset := call(t, ts, "POST", "/api/v1/config-assets/import", ot, map[string]string{"platform": "claude_desktop", "name": "共享配置", "content": `{"mcpServers":{"fs":{"command":"npx","args":["-y","@x"]}}}`})
	if code != 201 {
		t.Fatalf("asset %d: %v", code, asset)
	}
	assetID := asset["id"].(string)
	if code, out := call(t, ts, "PATCH", "/api/v1/config-assets/"+assetID, ot, map[string]any{"workspace_id": wsID}); code != 200 {
		t.Fatalf("share asset %d: %v", code, out)
	}

	// 3. ACL: members read shared resources, outsiders do not; viewers cannot comment.
	for _, who := range []struct{ name, token string }{{"editor", editor}, {"viewer", viewer}} {
		if code, _ := call(t, ts, "GET", "/api/v1/conversations/"+convID, who.token, nil); code != 200 {
			t.Fatalf("%s cannot read shared conversation", who.name)
		}
		if code, _ := call(t, ts, "GET", "/api/v1/config-assets/"+assetID, who.token, nil); code != 200 {
			t.Fatalf("%s cannot read shared asset", who.name)
		}
	}
	if code, _ := call(t, ts, "GET", "/api/v1/conversations/"+convID, outsider, nil); code != 404 {
		t.Fatal("outsider read leaked")
	}

	// 4. Comments: editor may, viewer may not; listing is member-only.
	code, _ = call(t, ts, "POST", "/api/v1/workspace-comments", editor, map[string]string{"target_type": "conversation", "target_id": convID, "body": "这个分支需要复核"})
	if code != 201 {
		t.Fatalf("editor comment %d", code)
	}
	if code, _ := call(t, ts, "POST", "/api/v1/workspace-comments", viewer, map[string]string{"target_type": "conversation", "target_id": convID, "body": "viewer"}); code != 403 {
		t.Fatalf("viewer commented %d", code)
	}
	if code, out := call(t, ts, "GET", "/api/v1/workspace-comments?target_type=conversation&target_id="+convID, viewer, nil); code != 200 || len(out["comments"].([]any)) != 1 {
		t.Fatalf("comment list %d: %v", code, out)
	}
	if code, _ := call(t, ts, "GET", "/api/v1/workspace-comments?target_type=conversation&target_id="+convID, outsider, nil); code != 404 {
		t.Fatal("outsider comment list leaked")
	}

	// 5. Cross-workspace isolation: members of one workspace see nothing of unshared rows.
	code, _ = call(t, ts, "POST", "/api/v1/workspaces", ot, map[string]string{"name": "另一个空间"})
	if code != 201 {
		t.Fatal("ws2")
	}
	_, memberList := call(t, ts, "GET", "/api/v1/workspaces/"+wsID+"/members", editor, nil)
	if len(memberList["members"].([]any)) != 3 {
		t.Fatalf("members wrong: %v", memberList)
	}

	// 6. Change hints: live delivery on a held connection, then Last-Event-ID replay.
	conn := openSSE(t, ts, editor, "")
	hello := conn.next(t, true)
	if hello.Type != "hello" {
		t.Fatalf("first event not hello: %v", hello)
	}
	if code, _ := call(t, ts, "POST", "/api/v1/workspace-comments", ot, map[string]string{"target_type": "conversation", "target_id": convID, "body": "owner note"}); code != 201 {
		t.Fatal("owner comment")
	}
	live := conn.next(t, false)
	if live.Type != "comment" || live.Seq <= hello.Seq {
		t.Fatalf("no live comment hint: %v", live)
	}
	conn.close()
	// Reconnect one event behind: the buffer replays the missed hint.
	resumed := openSSE(t, ts, editor, strconv.FormatUint(live.Seq-1, 10))
	replayed := resumed.next(t, false)
	if replayed.Type != live.Type || replayed.Seq != live.Seq {
		t.Fatalf("replay mismatch: %v vs %v", replayed, live)
	}
	resumed.close()

	// 7. Removing a member revokes access immediately; audit trail records the lifecycle.
	var editorID string
	if e := database.QueryRow(`SELECT id FROM users WHERE email=$1`, editorEmail).Scan(&editorID); e != nil {
		t.Fatal(e)
	}
	if code, _ := call(t, ts, "DELETE", "/api/v1/workspaces/"+wsID+"/members/"+editorID, ot, nil); code != 200 {
		t.Fatal("remove member")
	}
	if code, _ := call(t, ts, "GET", "/api/v1/conversations/"+convID, editor, nil); code != 404 {
		t.Fatal("removed member still reads shared resource")
	}
	var audits int
	if e := database.QueryRow(`SELECT count(*) FROM audit_events WHERE action IN ('workspace_created','workspace_invited','workspace_member_added','workspace_member_removed','resource_shared','workspace_comment')`).Scan(&audits); e != nil || audits < 6 {
		t.Fatalf("audit trail incomplete: %d %v", audits, e)
	}
}

func emailFor(role string) string {
	if role == "editor" {
		return "m5-editor@example.com"
	}
	return "m5-viewer@example.com"
}

type sseEvent struct {
	Seq  uint64
	Type string
}

// sseConn holds one change-hint connection across multiple reads.
type sseConn struct {
	t      *testing.T
	res    *http.Response
	reader *bufio.Reader
}

func openSSE(t *testing.T, ts *httptest.Server, token, lastID string) *sseConn {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/change-hints", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	if res.StatusCode != 200 {
		t.Fatalf("change hints status %d", res.StatusCode)
	}
	return &sseConn{t: t, res: res, reader: bufio.NewReader(res.Body)}
}

func (c *sseConn) next(t *testing.T, expectHello bool) sseEvent {
	t.Helper()
	for {
		line, e := c.reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		if !strings.HasPrefix(line, "event: ") {
			continue
		}
		eventType := strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		dataLine, _ := c.reader.ReadString('\n')
		var payload struct {
			Seq uint64 `json:"seq"`
		}
		_ = json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))), &payload)
		if expectHello {
			// A fresh connection replays buffered hints before the hello; skip until it.
			if eventType != "hello" {
				continue
			}
			return sseEvent{Seq: payload.Seq, Type: eventType}
		}
		return sseEvent{Seq: payload.Seq, Type: eventType}
	}
}

func (c *sseConn) close() { c.res.Body.Close() }
