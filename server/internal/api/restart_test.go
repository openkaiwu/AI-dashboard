package api_test

import (
	"aihub.dev/server/internal/api"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/testdb"
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"testing"
)

func TestServerProcess(t *testing.T) {
	dsn := os.Getenv("AIHUB_PROCESS_TEST_DSN")
	if dsn == "" {
		t.Skip("subprocess helper")
	}
	database, e := db.Open(dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	if e = db.Migrate(t.Context(), database); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("http://" + listener.Addr().String())
	if e = http.Serve(listener, api.New(database, nil, "").Handler()); e != nil {
		t.Fatal(e)
	}
}
func TestActualProcessRestart(t *testing.T) {
	database := testdb.Open(t)
	if e := api.BootstrapAdmin(t.Context(), database, "m0@example.com", "test-password-123"); e != nil {
		t.Fatal(e)
	}
	var schema string
	if e := database.QueryRow("SHOW search_path").Scan(&schema); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(os.Getenv("AIHUB_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	start := func() (*httptest.Server, func()) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestServerProcess$")
		cmd.Env = append(os.Environ(), "AIHUB_PROCESS_TEST_DSN="+u.String())
		stdout, e := cmd.StdoutPipe()
		if e != nil {
			t.Fatal(e)
		}
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
		t.Cleanup(stop)
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			t.Fatal("child failed to start")
		}
		return &httptest.Server{URL: scanner.Text()}, stop
	}
	first, stop := start()
	a := login(t, first, true, "process A")
	token := a["token"].(string)
	command := op("retry-after-crash", "durable", 0, "survives process restart", "put")
	push(t, first, token, command)
	stop()
	second, _ := start()
	if v := push(t, second, token, command); v["status"] != "applied" {
		t.Fatal(v)
	}
	_, page := call(t, second, "GET", "/api/v1/sync/pull", token, nil)
	if len(page["events"].([]any)) != 1 {
		t.Fatal("restart duplicated/lost event")
	}
}
