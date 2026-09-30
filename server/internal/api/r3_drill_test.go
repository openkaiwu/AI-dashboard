package api_test

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aihub.dev/server/internal/testdb"
)

// TestR3BackupRestoreDrill automates the INH-538 recovery drill against a real
// PostgreSQL: dump the migrated schema with pg_dump, DESTROY it, restore from
// the dump and prove the data came back. Skipped when pg_dump is unavailable.
func TestR3BackupRestoreDrill(t *testing.T) {
	url := os.Getenv("AIHUB_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AIHUB_TEST_DATABASE_URL required")
	}
	if _, e := exec.LookPath("pg_dump"); e != nil {
		t.Skip("pg_dump not installed")
	}
	if _, e := exec.LookPath("psql"); e != nil {
		t.Skip("psql not installed")
	}
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	_ = admin

	// The admin account is the marker row the drill must recover.
	var schema string
	if e := database.QueryRow(`SELECT current_schema()`).Scan(&schema); e != nil || schema == "" {
		t.Fatalf("current schema: %v %q", e, schema)
	}

	dumpFile := filepath.Join(t.TempDir(), "aihub-backup-drill.sql")
	dump := exec.Command("pg_dump", "--no-owner", "--clean", "--if-exists", "--schema="+schema, "-f", dumpFile, url)
	if out, e := dump.CombinedOutput(); e != nil {
		t.Fatalf("pg_dump failed: %v\n%s", e, out)
	}
	raw, e := os.ReadFile(dumpFile)
	if e != nil || !strings.Contains(string(raw), "m0@example.com") {
		t.Fatalf("dump missing marker row: %v", e)
	}

	// DESTROY the schema (simulates catastrophic upgrade failure).
	if _, e := database.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); e != nil {
		t.Fatalf("drop schema: %v", e)
	}
	var gone int
	if e := database.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name='users'`, schema).Scan(&gone); e != nil || gone != 0 {
		t.Fatalf("schema not destroyed: %d %v", gone, e)
	}

	// RESTORE: recreate schema, replay the dump inside one transaction.
	restore := exec.Command("psql", url, "-c", "CREATE SCHEMA IF NOT EXISTS "+schema)
	if out, e := restore.CombinedOutput(); e != nil {
		t.Fatalf("create schema: %v\n%s", e, out)
	}
	replay := exec.Command("psql", "--single-transaction", "--set", "ON_ERROR_STOP=1", url, "-c",
		"SET search_path="+schema, "-f", dumpFile)
	if out, e := replay.CombinedOutput(); e != nil {
		t.Fatalf("restore failed: %v\n%s", e, out)
	}

	// PROVE the data came back.
	var emails int
	if e := database.QueryRow(`SELECT count(*) FROM `+schema+`.users WHERE email='m0@example.com'`).Scan(&emails); e != nil || emails != 1 {
		t.Fatalf("marker row not recovered: %d %v", emails, e)
	}
	var tables int
	if e := database.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema=$1`, schema).Scan(&tables); e != nil || tables < 40 {
		t.Fatalf("restored schema incomplete: %d tables", tables)
	}
	_ = time.Now()
	_ = ts
	var _ *sql.DB = database
}
