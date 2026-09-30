package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestPreflightVersionRejection verifies the INH-540 version rejection path in
// an isolated schema: a database stamped by a newer binary must be reported
// incompatible, so an old process refuses to touch newer state.
func TestPreflightVersionRejection(t *testing.T) {
	dsn := os.Getenv("AIHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHUB_TEST_DATABASE_URL required")
	}
	root, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx := context.Background()

	schema := fmt.Sprintf("preflight_%d", time.Now().UnixNano())
	if _, e := root.ExecContext(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = root.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })

	isolated := dsn + "&search_path=" + schema
	database, e := sql.Open("pgx", isolated)
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()

	// Fresh schema: compatible by definition.
	report, e := Preflight(ctx, database)
	if e != nil {
		t.Fatalf("fresh preflight: %v", e)
	}
	if !report.Compatible {
		t.Fatalf("fresh database rejected: %+v", report)
	}

	// A database stamped by a NEWER binary must be refused.
	fake := "9999_fake_newer_migration.sql"
	if _, e := database.ExecContext(ctx, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY)`); e != nil {
		t.Fatal(e)
	}
	if _, e := database.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, fake); e != nil {
		t.Fatal(e)
	}
	report, e = Preflight(ctx, database)
	if e != nil {
		t.Fatalf("preflight: %v", e)
	}
	if report.Compatible {
		t.Fatalf("newer database accepted: %+v", report)
	}
	if len(report.UnknownVersions) == 0 || report.UnknownVersions[0] != fake {
		t.Fatalf("unknown version not reported: %+v", report)
	}
}
