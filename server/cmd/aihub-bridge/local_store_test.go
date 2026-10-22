package main

import (
	"aihub.dev/server/internal/codex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalStorageRetainsSamplesAndExcludesResetAndOutage(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	reset := now.Add(time.Hour).Unix()
	save := func(at time.Time, used float64, r int64) {
		t.Helper()
		raw, _ := json.Marshal(codex.Snapshot{ObservedAt: at, Status: "ok", Source: "codex_app_server", Buckets: []codex.Bucket{{ID: "codex", Primary: &codex.Window{UsedPercent: used, DurationMinutes: 10080, ResetsAt: &r}}}})
		if e := saveLocalSample(dir, "codex", raw); e != nil {
			t.Fatal(e)
		}
	}
	save(now, 10, reset)
	save(now.Add(time.Minute), 20, reset)
	save(now.Add(2*time.Minute), 2, reset+60)
	save(now.Add(time.Hour), 50, reset+60)
	raw, e := os.ReadFile(filepath.Join(dir, "rollups.json"))
	if e != nil {
		t.Fatal(e)
	}
	var out localRollups
	json.Unmarshal(raw, &out)
	sum, resets := 0.0, 0
	for _, r := range out.Rows {
		sum += r.Consumed
		resets += r.Resets
	}
	if sum != 10 || resets != 1 {
		t.Fatal(sum, resets)
	}
	if _, e = os.Stat(filepath.Join(dir, "codex-report.json")); e != nil {
		t.Fatal(e)
	}
}
