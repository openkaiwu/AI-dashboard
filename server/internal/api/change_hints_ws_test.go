package api_test

import (
	"aihub.dev/server/internal/testdb"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type hintEvent struct {
	Seq  uint64            `json:"seq"`
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

func respStatus(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func dialHints(t *testing.T, ts *httptest.Server, token, lastEventID string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u, e := url.Parse(ts.URL)
	if e != nil {
		t.Fatal(e)
	}
	u.Scheme = "ws"
	u.Path = "/api/v1/change-hints"
	q := url.Values{}
	if token != "" {
		q.Set("access_token", token)
	}
	if lastEventID != "" {
		q.Set("last_event_id", lastEventID)
	}
	u.RawQuery = q.Encode()
	header := http.Header{}
	if token != "" && lastEventID != "" {
		header.Set("Last-Event-ID", lastEventID)
	}
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	return dialer.Dial(u.String(), header)
}

func readHint(t *testing.T, conn *websocket.Conn) hintEvent {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ev hintEvent
	if e := conn.ReadJSON(&ev); e != nil {
		t.Fatalf("read hint: %v", e)
	}
	return ev
}

// TestChangeHintsWebSocket covers the INH-513 delivery swap: WebSocket on the
// change-hints route with the same hello, live push and Last-Event-ID replay
// semantics as the SSE channel, which stays available for fallback.
func TestChangeHintsWebSocket(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	ot := login(t, ts, true, "owner")
	token := ot["token"].(string)

	// Unauthenticated dials never reach the stream.
	if _, _, e := dialHints(t, ts, "", ""); e == nil {
		t.Fatal("unauthenticated dial accepted")
	}

	if code, _ := importFile(t, ts, token, "codex_cli_jsonl", "testdata/codex_session.jsonl"); code != 202 {
		t.Fatalf("import %d", code)
	}
	processImports(t, database)
	_, convs := call(t, ts, "GET", "/api/v1/conversations", token, nil)
	convID := convs["conversations"].([]any)[0].(map[string]any)["id"].(string)
	code, ws := call(t, ts, "POST", "/api/v1/workspaces", token, map[string]string{"name": "WS 空间"})
	if code != 201 {
		t.Fatalf("workspace %d", code)
	}
	if code, _ := call(t, ts, "PATCH", "/api/v1/conversations/"+convID, token, map[string]any{"workspace_id": ws["id"].(string)}); code != 200 {
		t.Fatal("share conversation")
	}

	// Live connection receives hello and then the comment hint.
	conn, resp, e := dialHints(t, ts, token, "")
	if e != nil {
		t.Fatalf("dial: %v status=%d", e, respStatus(resp))
	}
	hello := readHint(t, conn)
	if hello.Type != "hello" || hello.Seq == 0 {
		t.Fatalf("hello %v", hello)
	}
	code, _ = call(t, ts, "POST", "/api/v1/workspace-comments", token, map[string]string{"target_type": "conversation", "target_id": convID, "body": "ws 提醒"})
	if code != 201 {
		t.Fatalf("comment %d", code)
	}
	live := readHint(t, conn)
	if live.Type != "comment" || live.Seq != hello.Seq+1 {
		t.Fatalf("live hint %v (hello %d)", live, hello.Seq)
	}

	// A missed event replays on the next connection via Last-Event-ID.
	lastSeq := live.Seq
	_ = conn.Close()
	code, _ = call(t, ts, "POST", "/api/v1/workspace-comments", token, map[string]string{"target_type": "conversation", "target_id": convID, "body": "重放提醒"})
	if code != 201 {
		t.Fatalf("comment2 %d", code)
	}
	conn2, _, e := dialHints(t, ts, token, strconv.FormatUint(lastSeq, 10))
	if e != nil {
		t.Fatalf("dial2: %v", e)
	}
	defer conn2.Close()
	replayed := readHint(t, conn2)
	if replayed.Type != "comment" || replayed.Seq != lastSeq+1 || replayed.Data["target_id"] != convID {
		t.Fatalf("replayed hint %v (last %d)", replayed, lastSeq)
	}
	hello2 := readHint(t, conn2)
	if hello2.Type != "hello" {
		t.Fatalf("expected hello after replay, got %v", hello2)
	}
	_ = conn2.Close()

	// The SSE channel stays available as the fallback delivery.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/v1/change-hints", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatalf("sse: %v", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("sse response %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 1024)
	for {
		n, err := res.Body.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	body := string(buf)
	if !strings.Contains(body, "retry: 3000") || !strings.Contains(body, "event: hello") || !strings.Contains(body, `"type":"hello"`) {
		t.Fatalf("sse stream incomplete: %q", body)
	}
}
