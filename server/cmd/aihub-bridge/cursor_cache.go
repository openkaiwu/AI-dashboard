package main

import (
	"aihub.dev/server/internal/cursor"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func cursorCachePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "cursor-last-good.json")
}

func loadCursorCache(configPath string) (cursor.Snapshot, bool) {
	raw, err := os.ReadFile(cursorCachePath(configPath))
	if err != nil {
		return cursor.Snapshot{}, false
	}
	var snap cursor.Snapshot
	if json.Unmarshal(raw, &snap) != nil {
		return cursor.Snapshot{}, false
	}
	return snap, true
}

func saveCursorCache(configPath string, snap cursor.Snapshot) {
	if snap.Status != "ok" {
		return
	}
	raw, _ := json.Marshal(snap)
	_ = os.WriteFile(cursorCachePath(configPath), raw, 0o600)
}

func collectCursor(configPath string) cursor.Snapshot {
	now := time.Now().UTC()
	token, err := cursorAuthToken()
	if err != nil {
		if prev, ok := loadCursorCache(configPath); ok {
			return cursor.StaleSnapshot(prev, now, "local auth unavailable")
		}
		return cursor.UnknownSnapshot(now, "cursor not signed in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	snap, err := cursor.Collect(ctx, token, cursorPlanHint())
	if err == nil {
		saveCursorCache(configPath, snap)
		return snap
	}
	if prev, ok := loadCursorCache(configPath); ok {
		return cursor.StaleSnapshot(prev, now, err.Error())
	}
	return cursor.UnknownSnapshot(now, err.Error())
}
