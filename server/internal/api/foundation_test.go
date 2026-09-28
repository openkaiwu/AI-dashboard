package api_test

import (
	"aihub.dev/server/internal/api"
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/testdb"
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func call(t *testing.T, ts *httptest.Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	out := map[string]any{}
	if e = json.NewDecoder(res.Body).Decode(&out); e != nil {
		t.Fatal(e)
	}
	return res.StatusCode, out
}
func login(t *testing.T, ts *httptest.Server, register bool, name string) map[string]any {
	path := "login"
	if register {
		path = "register"
	}
	code, v := call(t, ts, "POST", "/api/v1/auth/"+path, "", map[string]string{"email": "m0@example.com", "password": "test-password-123", "device_name": name})
	if code >= 300 {
		t.Fatalf("sign in %d: %v", code, v)
	}
	return v
}
func op(id, entity string, base int, title, action string) map[string]any {
	return map[string]any{"protocol": 1, "operation_id": id, "entity": "note", "entity_id": entity, "base_version": base, "op": action, "payload": map[string]string{"title": title, "body": "offline draft"}}
}
func push(t *testing.T, ts *httptest.Server, token string, body any) map[string]any {
	code, v := call(t, ts, "POST", "/api/v1/sync/push", token, body)
	if code != 200 {
		t.Fatalf("push %d: %v", code, v)
	}
	return v
}
func server(t *testing.T, database *sql.DB) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(api.New(database, nil, "").Handler())
	t.Cleanup(ts.Close)
	return ts
}
func TestM0TwoDeviceRecovery(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "desktop")
	b := login(t, ts, false, "mobile")
	at := a["token"].(string)
	bt := b["token"].(string)
	// Both devices edit the same version while offline; commands remain immutable across retries.
	first := op("a1", "note-1", 0, "Desktop", "put")
	applied := push(t, ts, at, first)
	if applied["status"] != "applied" {
		t.Fatal(applied)
	}
	lostResponseRetry := push(t, ts, at, first)
	x, _ := json.Marshal(applied)
	y, _ := json.Marshal(lostResponseRetry)
	if !bytes.Equal(x, y) {
		t.Fatal("replay changed result")
	}
	conflict := push(t, ts, bt, op("b1", "note-1", 0, "Mobile", "put"))
	if conflict["status"] != "conflict" {
		t.Fatal(conflict)
	}
	if push(t, ts, bt, op("b1", "note-1", 0, "Mobile", "put"))["status"] != "conflict" {
		t.Fatal("conflict replay")
	}
	if code, _ := call(t, ts, "POST", "/api/v1/sync/push", at, op("a1", "note-1", 1, "changed", "put")); code != 409 {
		t.Fatalf("changed id accepted %d", code)
	}
	if push(t, ts, bt, op("b2", "note-1", 1, "Mobile resolved", "put"))["status"] != "applied" {
		t.Fatal("resolution")
	}
	_, page := call(t, ts, "GET", "/api/v1/sync/pull?limit=1", bt, nil)
	if len(page["events"].([]any)) != 1 || page["has_more"] != true {
		t.Fatal(page)
	}
	cursor := page["cursor"].(string)
	_, page = call(t, ts, "GET", "/api/v1/sync/pull?cursor="+cursor, bt, nil)
	if len(page["events"].([]any)) != 1 {
		t.Fatal(page)
	}
	cursor = page["cursor"].(string)
	// Recreate API process against the same DB: persisted sessions, dedupe and cursor survive.
	ts2 := server(t, database)
	if push(t, ts2, at, first)["status"] != "applied" {
		t.Fatal("restart dedupe")
	}
	push(t, ts2, at, op("a2", "note-1", 2, "", "delete"))
	_, page = call(t, ts2, "GET", "/api/v1/sync/pull?cursor="+cursor, bt, nil)
	events := page["events"].([]any)
	if len(events) != 1 || events[0].(map[string]any)["op"] != "delete" {
		t.Fatal(page)
	}
	if code, v := call(t, ts2, "GET", "/api/v1/sync/pull?cursor=old:100", bt, nil); code != 409 || v["error"] != "cursor_reset" {
		t.Fatal(v)
	}
	_, page = call(t, ts2, "GET", "/api/v1/sync/pull", bt, nil)
	if len(page["events"].([]any)) != 3 {
		t.Fatal("rebuild lost events")
	}
	// Tenant isolation: a different account receives no events or device access.
	code, c := call(t, ts2, "POST", "/api/v1/auth/register", "", map[string]string{"email": "other@example.com", "password": "test-password-456", "device_name": "other"})
	if code != 201 {
		t.Fatal(c)
	}
	ct := c["token"].(string)
	_, page = call(t, ts2, "GET", "/api/v1/sync/pull", ct, nil)
	if len(page["events"].([]any)) != 0 {
		t.Fatal("tenant data leak")
	}
	if code, _ = call(t, ts2, "DELETE", "/api/v1/devices/"+b["device_id"].(string), ct, nil); code != 404 {
		t.Fatal("cross-user revoke")
	}
	if code, _ = call(t, ts2, "DELETE", "/api/v1/devices/"+b["device_id"].(string), at, nil); code != 200 {
		t.Fatal("revoke")
	}
	for _, path := range []string{"/api/v1/me", "/api/v1/sync/pull"} {
		if code, _ = call(t, ts2, "GET", path, bt, nil); code != 401 {
			t.Fatal("revoked device accepted")
		}
	}
}
func TestConcurrentWritersAndCursorOrdering(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "A")
	token := a["token"].(string)
	var wg sync.WaitGroup
	results := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, _ := json.Marshal(op(string(rune('a'+i)), "shared", 0, "write", "put"))
			r, _ := http.NewRequest("POST", ts.URL+"/api/v1/sync/push", bytes.NewReader(raw))
			r.Header.Set("Authorization", "Bearer "+token)
			res, e := http.DefaultClient.Do(r)
			if e != nil {
				results <- "error"
				return
			}
			defer res.Body.Close()
			var v map[string]any
			json.NewDecoder(res.Body).Decode(&v)
			status, _ := v["status"].(string)
			results <- status
		}(i)
	}
	wg.Wait()
	close(results)
	applied, conflicts := 0, 0
	for r := range results {
		switch r {
		case "applied":
			applied++
		case "conflict":
			conflicts++
		default:
			t.Fatal(r)
		}
	}
	if applied != 1 || conflicts != 11 {
		t.Fatalf("applied=%d conflicts=%d", applied, conflicts)
	}
	_, page := call(t, ts, "GET", "/api/v1/sync/pull", token, nil)
	if len(page["events"].([]any)) != 1 {
		t.Fatal("duplicate event")
	}
	var events, ops int
	database.QueryRow("SELECT count(*) FROM sync_events").Scan(&events)
	database.QueryRow("SELECT count(*) FROM applied_operations").Scan(&ops)
	if events != 1 || ops != 12 {
		t.Fatalf("%d/%d", events, ops)
	}
}
func TestRefreshRotationReuseAndExpiry(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "A")
	if code, _ := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "m0@example.com", "password": "badpassword"}); code != 401 {
		t.Fatal("wrong password")
	}
	refresh := map[string]string{"refresh_token": a["refresh_token"].(string)}
	code, b := call(t, ts, "POST", "/api/v1/auth/refresh", "", refresh)
	if code != 200 {
		t.Fatal(b)
	}
	if code, _ = call(t, ts, "GET", "/api/v1/me", a["token"].(string), nil); code != 401 {
		t.Fatal("old access accepted")
	}
	if code, _ = call(t, ts, "GET", "/api/v1/me", b["token"].(string), nil); code != 200 {
		t.Fatal("new access rejected")
	}
	if code, _ = call(t, ts, "POST", "/api/v1/auth/refresh", "", refresh); code != 401 {
		t.Fatal("refresh reuse accepted")
	}
	if code, _ = call(t, ts, "GET", "/api/v1/sync/pull", b["token"].(string), nil); code != 401 {
		t.Fatal("reused family not revoked")
	}
	c := login(t, ts, false, "C")
	database.Exec("UPDATE sessions SET access_expires=now()-interval '1 second' WHERE access_hash=$1", auth.Hash(c["token"].(string)))
	if code, _ = call(t, ts, "GET", "/api/v1/me", c["token"].(string), nil); code != 401 {
		t.Fatal("expired access")
	}
	if code, _ = call(t, ts, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": c["refresh_token"].(string)}); code != 200 {
		t.Fatal("unrelated device revoked")
	}
}
func TestValidationAndOrigins(t *testing.T) {
	ts := server(t, testdb.Open(t))
	a := login(t, ts, true, "A")
	token := a["token"].(string)
	invalid := op("x", "a", 0, "x", "put")
	invalid["payload"] = map[string]string{"title": "x", "body": "body", "cookie": "secret"}
	if code, _ := call(t, ts, "POST", "/api/v1/sync/push", token, invalid); code != 400 {
		t.Fatal("unknown fields accepted")
	}
	invalid = op("y", "a", 0, "x", "put")
	invalid["protocol"] = 2
	if code, _ := call(t, ts, "POST", "/api/v1/sync/push", token, invalid); code != 426 {
		t.Fatal("unknown protocol accepted")
	}
	r, _ := http.NewRequest("GET", ts.URL+"/api/v1/me", nil)
	r.Header.Set("Origin", "https://untrusted.invalid")
	r.Header.Set("Authorization", "Bearer "+token)
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("untrusted origin")
	}
}

func TestDatabaseOutageDoesNotRevokeSession(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "A")
	database.Close()
	if code, _ := call(t, ts, "GET", "/api/v1/me", a["token"].(string), nil); code != 503 {
		t.Fatalf("DB outage masqueraded as expired credential: %d", code)
	}
	if code, _ := call(t, ts, "GET", "/health", "", nil); code != 200 {
		t.Fatal("liveness")
	}
	if code, _ := call(t, ts, "GET", "/ready", "", nil); code != 503 {
		t.Fatal("readiness")
	}
}
