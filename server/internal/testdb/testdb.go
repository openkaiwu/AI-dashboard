// Package testdb isolates every integration test in a temporary PostgreSQL schema.
package testdb

import (
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"database/sql"
	"net/url"
	"os"
	"testing"
)

func Open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AIHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHUB_TEST_DATABASE_URL required for PostgreSQL integration tests")
	}
	admin, e := db.Open(dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := httpx.NewID("test")
	if _, e = admin.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	database, e := db.Open(u.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { database.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	if e = db.Migrate(t.Context(), database); e != nil {
		t.Fatal(e)
	}
	return database
}
