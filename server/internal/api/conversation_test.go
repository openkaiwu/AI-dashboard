package api_test

import (
	"aihub.dev/server/internal/conversation"
	"aihub.dev/server/internal/testdb"
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// rawCall exchanges raw bytes (imports, exports) instead of JSON envelopes.
func rawCall(t *testing.T, ts *httptest.Server, method, path, token string, body []byte) (int, []byte) {
	t.Helper()
	r, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer " + token)
	}
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	out := new(bytes.Buffer)
	_, _ = out.ReadFrom(res.Body)
	return res.StatusCode, out.Bytes()
}

func importFile(t *testing.T, ts *httptest.Server, token, source, path string) (int, map[string]any) {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	code, body := rawCall(t, ts, "POST", "/api/v1/conversations/import?source="+source, token, raw)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return code, out
}

func processImports(t *testing.T, database *sql.DB) {
	t.Helper()
	conversation.ProcessImports(t.Context(), database)
}

func importCounts(t *testing.T, ts *httptest.Server, token, id string) map[string]any {
	t.Helper()
	code, out := call(t, ts, "GET", "/api/v1/imports/"+id, token, nil)
	if code != 200 {
		t.Fatalf("import status %d: %v", code, out)
	}
	return out
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func TestConversationImportDedupBranchAndArchiveRoundTrip(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "desktop")
	at := a["token"].(string)

	// Second account for tenant isolation.
	if code, out := call(t, ts, "POST", "/api/v1/admin/users", at, map[string]string{"email": "m3@example.com", "password": "test-password-123"}); code != 201 {
		t.Fatalf("create user %d: %v", code, out)
	}
	_, b := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "m3@example.com", "password": "test-password-123", "device_name": "other", "device_kind": "mobile", "installation_id": "other-device-install-0001"})
	bt := b["token"].(string)

	// 1. Import the ChatGPT tree export; the async worker materializes it.
	code, out := importFile(t, ts, at, "chatgpt_export", "testdata/chatgpt_export.json")
	if code != 202 {
		t.Fatalf("import %d: %v", code, out)
	}
	importID := out["id"].(string)
	processImports(t, database)
	done := importCounts(t, ts, at, importID)
	if done["status"] != "completed" {
		t.Fatalf("import not completed: %v", done)
	}
	if num(done["conversations_created"]) != 2 || num(done["messages_imported"]) != 7 || num(done["branches_created"]) != 3 {
		t.Fatalf("counts wrong: %v", done)
	}

	// 2. The branch graph survives: main path plus the divergent assistant answer.
	_, list := call(t, ts, "GET", "/api/v1/conversations", at, nil)
	convs := list["conversations"].([]any)
	if len(convs) != 2 {
		t.Fatalf("expected 2 conversations: %v", list)
	}
	var branchDemo string
	for _, c := range convs {
		m := c.(map[string]any)
		if m["title"] == "Branch demo" {
			branchDemo = m["id"].(string)
		}
	}
	if branchDemo == "" {
		t.Fatal("branch demo missing")
	}
	code, detail := call(t, ts, "GET", "/api/v1/conversations/"+branchDemo, at, nil)
	if code != 200 {
		t.Fatalf("detail %d", code)
	}
	branches := detail["branches"].([]any)
	if len(branches) != 2 {
		t.Fatalf("expected main + branch-1: %v", detail)
	}
	main := branches[0].(map[string]any)
	if main["name"] != "main" || len(main["messages"].([]any)) != 4 {
		t.Fatalf("main branch wrong: %v", main)
	}
	msgs := main["messages"].([]any)
	first := msgs[0].(map[string]any)
	second := msgs[1].(map[string]any)
	if first["parent_id"] != nil || second["parent_id"] != first["id"] {
		t.Fatalf("parent chain broken: %v %v", first, second)
	}
	alt := branches[1].(map[string]any)
	if alt["name"] != "branch-1" || len(alt["messages"].([]any)) != 1 {
		t.Fatalf("alternate branch wrong: %v", alt)
	}

	// 3. Identical re-import dedups instead of duplicating.
	code, out = importFile(t, ts, at, "chatgpt_export", "testdata/chatgpt_export.json")
	if code != 202 {
		t.Fatalf("reimport %d", code)
	}
	processImports(t, database)
	done = importCounts(t, ts, at, out["id"].(string))
	if done["status"] != "completed" || num(done["conversations_deduplicated"]) != 2 || num(done["conversations_created"]) != 0 {
		t.Fatalf("dedup failed: %v", done)
	}

	// 4. Archive round-trip: export v1 archive, re-import, everything dedups.
	code, archive := rawCall(t, ts, "GET", "/api/v1/conversations/"+branchDemo+"/export?format=archive", at, nil)
	if code != 200 {
		t.Fatalf("export %d", code)
	}
	if !bytes.Contains(archive, []byte("aihub.conversation-archive")) {
		t.Fatal("archive envelope missing")
	}
	code, out2raw := rawCall(t, ts, "POST", "/api/v1/conversations/import?source=archive", at, archive)
	if code != 202 {
		t.Fatalf("archive reimport %d", code)
	}
	var out2 map[string]any
	_ = json.Unmarshal(out2raw, &out2)
	processImports(t, database)
	done = importCounts(t, ts, at, out2["id"].(string))
	if done["status"] != "completed" || num(done["conversations_deduplicated"]) != 1 || num(done["conversations_created"]) != 0 {
		t.Fatalf("archive round-trip duplicated: %v", done)
	}

	// 5. Markdown export is human-readable.
	code, md := rawCall(t, ts, "GET", "/api/v1/conversations/"+branchDemo+"/export?format=markdown", at, nil)
	if code != 200 || !bytes.Contains(md, []byte("# Branch demo")) || !bytes.Contains(md, []byte("### user")) {
		t.Fatalf("markdown export wrong: %d %s", code, md)
	}

	// 6. Tenant isolation: the other account sees nothing of user A.
	if code, _ := call(t, ts, "GET", "/api/v1/conversations/"+branchDemo, bt, nil); code != 404 {
		t.Fatalf("cross-tenant read %d", code)
	}
	_, blist := call(t, ts, "GET", "/api/v1/conversations", bt, nil)
	if len(blist["conversations"].([]any)) != 0 {
		t.Fatal("cross-tenant list leak")
	}

	// 7. Codex CLI JSONL import tolerates foreign lines and keeps the two items.
	code, out = importFile(t, ts, at, "codex_cli_jsonl", "testdata/codex_session.jsonl")
	if code != 202 {
		t.Fatalf("codex import %d: %v", code, out)
	}
	processImports(t, database)
	done = importCounts(t, ts, at, out["id"].(string))
	if done["status"] != "completed" || num(done["conversations_created"]) != 1 || num(done["messages_imported"]) != 2 {
		t.Fatalf("codex import wrong: %v", done)
	}
}

func TestConversationImportRejectsInvalidInput(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "desktop")
	at := a["token"].(string)

	if code, _ := rawCall(t, ts, "POST", "/api/v1/conversations/import?source=bogus", at, []byte("{}")); code != 400 {
		t.Fatalf("unknown source %d", code)
	}
	if code, body := rawCall(t, ts, "POST", "/api/v1/conversations/import?source=chatgpt_export", at, []byte("not-json")); code != 400 || !strings.Contains(string(body), "invalid_import") {
		t.Fatalf("garbage accepted: %d %s", code, body)
	}
	if code, _ := rawCall(t, ts, "POST", "/api/v1/conversations/import?source=archive", at, bytes.Repeat([]byte("x"), conversation.MaxFileBytes+10)); code != 400 {
		t.Fatalf("oversize accepted %d", code)
	}
	if code, _ := rawCall(t, ts, "POST", "/api/v1/conversations/import?source=archive", "", nil); code != 401 {
		t.Fatalf("unauthenticated accepted %d", code)
	}
}
