package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Source selection rules (plan section 4.2): business data time decides which
// data is served; the upload timestamp never does. Overlapping aggregates from
// multiple collectors are never summed - one effective source is selected per
// bucket and the rest stays visible through the source metadata.

// bucketCoverage is the shared shape of a daily/weekly/monthly series entry.
type bucketCoverage struct {
	BucketStart   string   `json:"bucket_start"`
	ConsumedPP    *float64 `json:"consumed_pp"`
	LevelEnd      *float64 `json:"level_end"`
	PeakPPHour    *float64 `json:"peak_pp_hour"`
	Resets        *int     `json:"resets"`
	CoverageHours int      `json:"coverage_hours"`
}

func (b bucketCoverage) cover() int {
	if b.CoverageHours > 0 {
		return b.CoverageHours
	}
	if b.ConsumedPP != nil || b.LevelEnd != nil {
		return 1
	}
	return 0
}

type hourlyEntry struct {
	Hour     string  `json:"hour"`
	Consumed float64 `json:"consumed_pp"`
	Samples  int     `json:"samples"`
	Resets   int     `json:"resets"`
}

// mergeSeries picks, per calendar bucket, the source that actually covers it.
// The live rollup wins ties; the local backup fills buckets the server never
// received (offline periods). Values are never added across sources.
func mergeSeries(live, backup []bucketCoverage) ([]bucketCoverage, bool) {
	if len(backup) == 0 {
		return live, false
	}
	if len(live) == 0 {
		return backup, true
	}
	byLabel := map[string]bucketCoverage{}
	for _, entry := range live {
		byLabel[entry.BucketStart] = entry
	}
	usedBackup := false
	for _, entry := range backup {
		current, exists := byLabel[entry.BucketStart]
		if !exists || entry.cover() > current.cover() {
			byLabel[entry.BucketStart] = entry
			usedBackup = true
		}
	}
	merged := make([]bucketCoverage, 0, len(byLabel))
	for _, entry := range byLabel {
		merged = append(merged, entry)
	}
	sortBuckets(merged)
	return merged, usedBackup
}

// mergeHourly applies the same one-source-per-bucket rule to the hourly view.
func mergeHourly(live, backup []hourlyEntry) ([]hourlyEntry, bool) {
	if len(backup) == 0 {
		return live, false
	}
	if len(live) == 0 {
		return backup, true
	}
	byHour := map[string]hourlyEntry{}
	for _, row := range live {
		byHour[row.Hour] = row
	}
	usedBackup := false
	for _, row := range backup {
		current, exists := byHour[row.Hour]
		if !exists || row.Samples > current.Samples {
			byHour[row.Hour] = row
			usedBackup = true
		}
	}
	merged := make([]hourlyEntry, 0, len(byHour))
	for _, row := range byHour {
		merged = append(merged, row)
	}
	sortHourly(merged)
	return merged, usedBackup
}

func sortBuckets(rows []bucketCoverage) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].BucketStart < rows[j-1].BucketStart; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func sortHourly(rows []hourlyEntry) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Hour < rows[j-1].Hour; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// sourceLabel describes which source produced the served numbers.
func sourceLabel(hasLive, usedBackup bool, empty bool) string {
	if empty {
		return "none"
	}
	if usedBackup && hasLive {
		return "merged"
	}
	if usedBackup {
		return "local_backup"
	}
	return "server_rollup"
}

// staleness thresholds: fine views follow the reminder freshness window; day
// and coarser views allow a full collection day of gap before "stale".
const (
	fineStaleAfter   = 11 * time.Minute
	coarseStaleAfter = 36 * time.Hour
)

func staleReason(granularity string, dataAsOf time.Time, now time.Time) string {
	if dataAsOf.IsZero() {
		return "no_source"
	}
	limit := coarseStaleAfter
	if granularity == "window" || granularity == "hourly" {
		limit = fineStaleAfter
	}
	if now.Sub(dataAsOf) > limit {
		return "stale_source"
	}
	return ""
}

// later returns the newer of two observation times.
func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// liveDataAsOf is the newest raw sample the server has actually received.
func (s *Service) liveDataAsOf(ctx context.Context, uid string) (time.Time, bool) {
	var at sql.NullTime
	if e := s.DB.QueryRowContext(ctx, `SELECT max(observed_at) FROM codex_history h JOIN codex_bridges b ON b.id=h.bridge_id WHERE b.user_id=$1 AND b.revoked_at IS NULL`, uid).Scan(&at); e != nil || !at.Valid {
		return time.Time{}, false
	}
	return at.Time.UTC(), true
}

// freshestBackup returns the stored backup with the greatest business data
// time, regardless of which device uploaded it last.
func (s *Service) freshestBackup(ctx context.Context, uid string) (LocalBackup, bool) {
	var raw []byte
	var stored sql.NullTime
	e := s.DB.QueryRowContext(ctx, `SELECT payload,data_as_of FROM local_data_backups WHERE user_id=$1 ORDER BY data_as_of DESC NULLS LAST, updated_at DESC LIMIT 1`, uid).Scan(&raw, &stored)
	if e != nil {
		return LocalBackup{}, false
	}
	var backup LocalBackup
	if json.Unmarshal(raw, &backup) != nil {
		return LocalBackup{}, false
	}
	if _, ok := backupDataAsOf(raw, stored); !ok {
		return LocalBackup{}, false
	}
	return backup, true
}

// backupSeries parses a stored consumption aggregate section.
func backupSeries(backup LocalBackup, key string) []bucketCoverage {
	raw := backup.Consumption[key]
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Series []bucketCoverage `json:"series"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	return payload.Series
}

func backupHourly(backup LocalBackup, key string) []hourlyEntry {
	raw := backup.Consumption[key]
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Hourly []hourlyEntry `json:"hourly"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	return payload.Hourly
}
