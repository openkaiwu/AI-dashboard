package main

import (
	"aihub.dev/server/internal/codex"
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func writeLocalJSON(file string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return e
	}
	tmp := file + ".tmp"
	if e = os.WriteFile(tmp, raw, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, file)
}

func saveLocalSample(dir, slug string, payload []byte) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	archive := filepath.Join(dir, "samples")
	if e := os.MkdirAll(archive, 0700); e != nil {
		return e
	}
	file, e := os.OpenFile(filepath.Join(archive, slug+"-"+time.Now().UTC().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = file.Write(append(payload, '\n'))
	if e == nil {
		e = file.Sync()
	}
	closeErr := file.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	var value any
	if e = json.Unmarshal(payload, &value); e != nil {
		return e
	}
	if slug == "codex" {
		if e = rollupLocal(dir, payload); e != nil {
			return e
		}
	}
	if e = writeLocalJSON(filepath.Join(dir, slug+"-latest.json"), value); e != nil {
		return e
	}
	if slug != "codex" {
		return nil
	}
	var current codex.Snapshot
	if e = json.Unmarshal(payload, &current); e != nil {
		return e
	}
	prefs := struct {
		Preferences codex.Preferences `json:"preferences"`
		Plan        string            `json:"plan"`
	}{Preferences: codex.Defaults(), Plan: "unknown"}
	if raw, e := os.ReadFile(filepath.Join(dir, "preferences.json")); e == nil {
		_ = json.Unmarshal(raw, &prefs)
	}
	if !prefs.Preferences.Valid() {
		prefs.Preferences = codex.Defaults()
	}
	history := []codex.Snapshot{}
	for _, day := range []time.Time{time.Now().UTC().Add(-24 * time.Hour), time.Now().UTC()} {
		f, e := os.Open(filepath.Join(archive, "codex-"+day.Format("2006-01-02")+".jsonl"))
		if e != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 65536), 1024*1024)
		for scanner.Scan() {
			var s codex.Snapshot
			if json.Unmarshal(scanner.Bytes(), &s) == nil && s.ObservedAt.After(time.Now().Add(-6*time.Hour)) {
				history = append(history, s.ForPlan(prefs.Plan))
			}
		}
		if e = scanner.Err(); e != nil {
			f.Close()
			return e
		}
		f.Close()
	}
	sort.Slice(history, func(i, j int) bool { return history[i].ObservedAt.Before(history[j].ObservedAt) })
	current = current.ForPlan(prefs.Plan)
	return writeLocalJSON(filepath.Join(dir, "codex-report.json"), map[string]any{"snapshot": current, "history": history, "analysis": codex.Analyze(current, history, prefs.Preferences, time.Now()), "preferences": prefs.Preferences, "plan": map[string]string{"plan_type": prefs.Plan, "source_type": "manual"}})
}

type localRollup struct {
	Hour     string  `json:"hour"`
	Duration int64   `json:"duration"`
	Consumed float64 `json:"consumed_pp"`
	Samples  int     `json:"samples"`
	Resets   int     `json:"resets"`
}
type localRollups struct {
	Last time.Time              `json:"last"`
	Rows map[string]localRollup `json:"rows"`
}

func rollupLocal(dir string, payload []byte) error {
	var cur, prev codex.Snapshot
	if e := json.Unmarshal(payload, &cur); e != nil {
		return e
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "codex-latest.json"))
	_ = json.Unmarshal(raw, &prev)
	state := localRollups{Rows: map[string]localRollup{}}
	raw, _ = os.ReadFile(filepath.Join(dir, "rollups.json"))
	_ = json.Unmarshal(raw, &state)
	if !cur.ObservedAt.After(state.Last) {
		return nil
	}
	if state.Rows == nil {
		state.Rows = map[string]localRollup{}
	}
	if cur.Status == "ok" {
		for _, b := range cur.Buckets {
			for slot, w := range []*codex.Window{b.Primary, b.Secondary} {
				if w == nil {
					continue
				}
				hour := cur.ObservedAt.UTC().Truncate(time.Hour).Format(time.RFC3339)
				key := hour + ":" + b.ID + ":" + strconv.FormatInt(w.DurationMinutes, 10)
				row := state.Rows[key]
				row.Hour = hour
				row.Duration = w.DurationMinutes
				row.Samples++
				if prev.Status == "ok" && cur.ObservedAt.Sub(prev.ObservedAt) <= 15*time.Minute {
					for _, pb := range prev.Buckets {
						if pb.ID != b.ID {
							continue
						}
						old := []*codex.Window{pb.Primary, pb.Secondary}[slot]
						if old == nil || old.DurationMinutes != w.DurationMinutes || old.ResetsAt == nil || w.ResetsAt == nil {
							continue
						}
						if *old.ResetsAt != *w.ResetsAt || w.UsedPercent < old.UsedPercent-2 {
							row.Resets++
						} else {
							row.Consumed += math.Max(0, w.UsedPercent-old.UsedPercent)
						}
					}
				}
				state.Rows[key] = row
			}
		}
	}
	state.Last = cur.ObservedAt
	return writeLocalJSON(filepath.Join(dir, "rollups.json"), state)
}
