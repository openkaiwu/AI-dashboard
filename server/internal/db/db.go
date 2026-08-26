package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." && filepath.Dir(path) != "" {
		return nil, err
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if _, err := database.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func Migrate(ctx context.Context, database *sql.DB, sqlText string) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return tx.Commit()
}
