//go:build windows

package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var errCursorUnavailable = errors.New("cursor local auth unavailable")

func cursorStateDBPath() string {
	if base := os.Getenv("APPDATA"); base != "" {
		return filepath.Join(base, "Cursor", "User", "globalStorage", "state.vscdb")
	}
	return ""
}

func readCursorStateKey(key string) (string, error) {
	path := cursorStateDBPath()
	if path == "" {
		return "", errCursorUnavailable
	}
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var value string
	err = db.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func cursorAuthToken() (string, error) {
	return readCursorStateKey("cursorAuth/accessToken")
}

func cursorPlanHint() string {
	plan, err := readCursorStateKey("cursorAuth/stripeMembershipType")
	if err != nil {
		return ""
	}
	return plan
}

func cursorAvailable() bool {
	path := cursorStateDBPath()
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	token, err := cursorAuthToken()
	return err == nil && token != ""
}
