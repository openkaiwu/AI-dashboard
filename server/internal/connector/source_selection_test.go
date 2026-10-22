package connector

import (
	"aihub.dev/server/internal/codex"
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func fp(v float64) *float64 { return &v }

// Overlapping buckets are never summed: the source with greater coverage
// wins and the other source's value is dropped for that bucket (plan 4.2).
func TestMergeSeriesFillsGapsWithoutSumming(t *testing.T) {
	live := []bucketCoverage{{BucketStart: "2026-10-21", ConsumedPP: fp(5), CoverageHours: 3}}
	backup := []bucketCoverage{
		{BucketStart: "2026-10-20", ConsumedPP: fp(8), CoverageHours: 6}, // offline day, rollup lacks it
		{BucketStart: "2026-10-21", ConsumedPP: fp(8), CoverageHours: 1}, // overlapping, weaker coverage
	}
	merged, usedBackup := mergeSeries(live, backup)
	if len(merged) != 2 || !usedBackup {
		t.Fatal(merged, usedBackup)
	}
	byLabel := map[string]bucketCoverage{}
	for _, e := range merged {
		byLabel[e.BucketStart] = e
	}
	if got := *byLabel["2026-10-21"].ConsumedPP; got != 5 {
		t.Fatalf("overlap was overridden or summed: %v", got)
	}
	if got := *byLabel["2026-10-20"].ConsumedPP; got != 8 {
		t.Fatalf("offline bucket missing: %v", got)
	}
}

func TestMergeSeriesLiveWinsTies(t *testing.T) {
	live := []bucketCoverage{{BucketStart: "2026-10-21", ConsumedPP: fp(5), CoverageHours: 2}}
	backup := []bucketCoverage{{BucketStart: "2026-10-21", ConsumedPP: fp(9), CoverageHours: 2}}
	merged, usedBackup := mergeSeries(live, backup)
	if len(merged) != 1 || usedBackup {
		t.Fatal(merged, usedBackup)
	}
	if *merged[0].ConsumedPP != 5 {
		t.Fatal("tie did not stay with the live rollup")
	}
}

func TestMergeHourlyFillsGapsWithoutSumming(t *testing.T) {
	live := []hourlyEntry{{Hour: "2026-10-21T10:00:00Z", Consumed: 4, Samples: 2}}
	backup := []hourlyEntry{
		{Hour: "2026-10-21T09:00:00Z", Consumed: 7, Samples: 3},
		{Hour: "2026-10-21T10:00:00Z", Consumed: 9, Samples: 1},
	}
	merged, usedBackup := mergeHourly(live, backup)
	if len(merged) != 2 || !usedBackup {
		t.Fatal(merged, usedBackup)
	}
	byHour := map[string]hourlyEntry{}
	for _, e := range merged {
		byHour[e.Hour] = e
	}
	if byHour["2026-10-21T10:00:00Z"].Consumed != 4 || byHour["2026-10-21T09:00:00Z"].Consumed != 7 {
		t.Fatalf("hourly merge summed or lost buckets: %v", byHour)
	}
}

func TestStaleReasonThresholds(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if got := staleReason("window", time.Time{}, now); got != "no_source" {
		t.Fatal(got)
	}
	fresh := now.Add(-5 * time.Minute)
	if got := staleReason("window", fresh, now); got != "" {
		t.Fatal(got)
	}
	if got := staleReason("hourly", now.Add(-12*time.Minute), now); got != "stale_source" {
		t.Fatal(got)
	}
	// Daily views tolerate a full collection day of gap.
	if got := staleReason("daily", now.Add(-30*time.Hour), now); got != "" {
		t.Fatal(got)
	}
	if got := staleReason("daily", now.Add(-40*time.Hour), now); got != "stale_source" {
		t.Fatal(got)
	}
}

func TestBackupFreshnessUsesNewestObservation(t *testing.T) {
	older := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 10, 21, 11, 30, 0, 0, time.UTC)
	generated := time.Date(2026, 10, 21, 11, 31, 0, 0, time.UTC)
	b := LocalBackup{GeneratedAt: generated, RecentSnapshots: []codex.Snapshot{
		{ObservedAt: older}, {ObservedAt: newer},
	}}
	if !b.Freshness().Equal(newer) {
		t.Fatalf("freshness must be the newest observation, got %v", b.Freshness())
	}
	empty := LocalBackup{GeneratedAt: generated}
	if !empty.ObservedFreshness().IsZero() { t.Fatal("generation time is not collection time") }
	if !empty.Freshness().Equal(generated) {
		t.Fatal("no samples: freshness falls back to generation time")
	}
}

// Legacy rows written before migration 029 keep their freshness readable from
// the payload itself (stored column NULL fallback).
func TestBackupDataAsOfLegacyFallback(t *testing.T) {
	at := time.Date(2026, 10, 21, 11, 30, 0, 0, time.UTC)
	raw, _ := json.Marshal(LocalBackup{GeneratedAt: at, RecentSnapshots: []codex.Snapshot{{ObservedAt: at}}})
	got, ok := backupDataAsOf(raw, sql.NullTime{})
	if !ok || !got.Equal(at) {
		t.Fatal(got, ok)
	}
}

func TestEffectiveWindowFilterDefaultsToPrimary(t *testing.T) {
	if got := effectiveWindowFilter("", "unknown"); got != "10080" {
		t.Fatal("empty filter must mean the primary weekly window, never a sum of windows:", got)
	}
	if got := effectiveWindowFilter("300", "plus"); got != "300" {
		t.Fatal(got)
	}
}

func TestSourceLabel(t *testing.T) {
	cases := []struct {
		hasLive, usedBackup, empty bool
		want                       string
	}{
		{true, false, false, "server_rollup"},
		{false, true, false, "local_backup"},
		{true, true, false, "merged"},
		{false, false, true, "none"},
	}
	for _, c := range cases {
		if got := sourceLabel(c.hasLive, c.usedBackup, c.empty); got != c.want {
			t.Fatalf("sourceLabel(%v,%v,%v)=%s want %s", c.hasLive, c.usedBackup, c.empty, got, c.want)
		}
	}
}
