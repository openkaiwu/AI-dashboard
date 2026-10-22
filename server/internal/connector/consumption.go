package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/httpx"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type consumptionPoint struct {
	T    string  `json:"t"`
	Used float64 `json:"used"`
}

type consumptionGeneration struct {
	ResetsAt *int64             `json:"resets_at"`
	Start    string             `json:"start"`
	End      string             `json:"end"`
	Points   []consumptionPoint `json:"points"`
}

type consumptionWindow struct {
	ID              string `json:"id"`
	DurationMinutes int64  `json:"duration_minutes"`
	Present         bool   `json:"present"`
}

// effectiveWindowFilter keeps different quota windows from being summed into
// one number (plan rule 7): an unspecified window means the primary weekly
// quota, never "all windows added together".
func effectiveWindowFilter(filter, plan string) string {
	if filter == "" {
		return "10080"
	}
	return filter
}

// Consumption serves the daily/weekly/monthly usage views backed by the
// hourly rollup, plus the live per-generation series for the current window.
// Aggregates uploaded by desktops (offline periods the server never saw as
// raw samples) fill the buckets the rollup lacks, without ever summing two
// sources over the same bucket (plan section 4.2).
func (s *Service) Consumption(w http.ResponseWriter, r *http.Request) {
	uid := auth.Who(r).UserID
	granularity := r.URL.Query().Get("granularity")
	switch granularity {
	case "", "window", "hourly", "daily", "weekly", "monthly":
	default:
		httpx.Error(w, 400, "invalid_granularity", "granularity 取值非法")
		return
	}
	backup, hasBackup := s.freshestBackup(r.Context(), uid)
	var backupAt time.Time
	if hasBackup {
		backupAt = backup.ObservedFreshness()
	}
	liveAt, hasLive := s.liveDataAsOf(r.Context(), uid)
	dataAsOf := later(liveAt, backupAt)

	tz := s.tzOffsetMinutes(r.Context(), uid)
	if v := r.URL.Query().Get("tz_offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= -720 && n <= 840 {
			tz = n
		}
	}
	loc := time.FixedZone("hub", tz*60)
	now := time.Now().UTC()
	plan, _, pe := getPlan(r.Context(), s.DB, uid)
	if pe != nil {
		plan = "unknown"
	}
	if granularity == "hourly" {
		out, e := s.consumptionHourly(r.Context(), uid, now, r.URL.Query(), plan, backup)
		if e != nil {
			httpx.Error(w, 503, "unavailable", "消耗数据暂不可用")
			return
		}
		out["plan_type"] = plan
		out["tz_offset_minutes"] = tz
		out["data_as_of"] = observationValue(dataAsOf)
		out["stale_reason"] = staleReason("hourly", dataAsOf, now)
		httpx.WriteJSON(w, 200, out)
		return
	}

	// Raw-history samples only survive 7 days, so the fine view reads
	// codex_history directly while coarser views read the hourly rollup.
	// Both paths first replay any raw samples the rollup does not cover yet
	// (initial backfill and self-healing after a missed rollup write).
	if granularity == "window" {
		out, e := s.consumptionWindow(r.Context(), uid, now, loc, r.URL.Query().Get("window"), plan, backup, liveAt)
		if e != nil {
			httpx.Error(w, 503, "unavailable", "消耗数据暂不可用")
			return
		}
		out["tz_offset_minutes"] = tz
		out["data_as_of"] = observationValue(dataAsOf)
		out["stale_reason"] = staleReason("window", dataAsOf, now)
		httpx.WriteJSON(w, 200, out)
		return
	}

	days := map[string]int{"daily": 30, "weekly": 84, "monthly": 365}[granularity]
	if v := r.URL.Query().Get("days"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 7 && n <= 365 && n < days {
			days = n
		}
	}
	if e := s.ensureRollups(r.Context(), uid); e != nil {
		httpx.Error(w, 503, "unavailable", "消耗数据暂不可用")
		return
	}
	// One effective source per hour bucket: when several computers observe
	// the same account, the most recently active collector wins and the
	// others only fill hours it never covered. Devices are never summed.
	rows, e := s.DB.QueryContext(r.Context(), `SELECT hour_bucket,window_key,consumed_pp,samples,resets,level_last FROM (SELECT r.hour_bucket,r.window_key,r.consumed_pp,r.samples,r.resets,r.level_last,row_number() OVER (PARTITION BY r.hour_bucket,r.window_key ORDER BY COALESCE(st.last_observed_at,to_timestamp(0)) DESC,r.samples DESC,r.bridge_id) AS rn FROM codex_usage_rollup r JOIN codex_bridges b ON b.id=r.bridge_id AND b.user_id=$1 AND b.revoked_at IS NULL LEFT JOIN codex_usage_rollup_state st ON st.bridge_id=r.bridge_id WHERE r.hour_bucket>now()-($2 * interval '1 day')) ranked WHERE rn=1`, uid, days+1)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "消耗数据暂不可用")
		return
	}
	defer rows.Close()

	type hourRow struct {
		hour     time.Time
		key      string
		consumed float64
		samples  int
		resets   int
		level    float64
	}
	var hours []hourRow
	for rows.Next() {
		var h hourRow
		var consumed, level sql.NullFloat64
		if e = rows.Scan(&h.hour, &h.key, &consumed, &h.samples, &h.resets, &level); e != nil {
			rows.Close()
			httpx.Error(w, 503, "unavailable", "消耗数据暂不可用")
			return
		}
		h.consumed, h.level = consumed.Float64, level.Float64
		hours = append(hours, h)
	}
	rows.Close()

	// Apply plan visibility before aggregating; raw rollups remain available for plan changes.
	windowFilter := effectiveWindowFilter(r.URL.Query().Get("window"), plan)
	{
		kept := hours[:0]
		for _, h := range hours {
			if visibleConsumptionKey(h.key, windowFilter, plan) {
				kept = append(kept, h)
			}
		}
		hours = kept
	}

	// Calendar buckets in the account timezone.
	bucketLabel := func(t time.Time) string {
		local := t.In(loc)
		switch granularity {
		case "weekly":
			off := (int(local.Weekday()) + 6) % 7 // Monday as the week start
			return local.AddDate(0, 0, -off).Format("2006-01-02")
		case "monthly":
			return local.Format("2006-01") + "-01"
		default:
			return local.Format("2006-01-02")
		}
	}
	type agg struct {
		consumed   float64
		level      float64
		hasLevel   bool
		peak       float64
		resets     int
		hoursCount int
	}
	byBucket := map[string]*agg{}
	for _, h := range hours {
		label := bucketLabel(h.hour)
		a := byBucket[label]
		if a == nil {
			a = &agg{}
			byBucket[label] = a
		}
		a.consumed += h.consumed
		a.resets += h.resets
		a.hoursCount++
		if h.samples > 0 {
			a.level = h.level // overwritten per row: ends up as the latest level in the bucket
			a.hasLevel = true
		}
		if h.consumed > a.peak {
			a.peak = h.consumed
		}
	}

	// Build the full calendar series so the front end can render gaps honestly.
	rangeStart := bucketStart(now.Add(-time.Duration(days)*24*time.Hour), loc, granularity)
	series := []bucketCoverage{}
	for cur := rangeStart; !cur.After(bucketStart(now, loc, granularity)); cur = nextBucket(cur, loc, granularity) {
		label := cur.Format("2006-01-02")
		if granularity == "weekly" {
			label = cur.Format("2006-01-02")
		}
		entry := bucketCoverage{BucketStart: label, CoverageHours: 0}
		if a := byBucket[label]; a != nil {
			entry.ConsumedPP = &a.consumed
			entry.PeakPPHour = &a.peak
			entry.Resets = &a.resets
			entry.CoverageHours = a.hoursCount
			if a.hasLevel {
				lv := a.level
				entry.LevelEnd = &lv
			}
		}
		series = append(series, entry)
	}

	// Desktop aggregates fill the calendar buckets the rollup lacks; the two
	// sources are never added over the same bucket.
	usedBackup := false
	if granularity == "daily" && hasBackup {
		if key := windowFilter + ":daily"; codex.WindowVisibleForPlan(plan, windowDuration(windowFilter)) {
			if backupRows := backupSeries(backup, key); len(backupRows) > 0 {
				series, usedBackup = mergeSeries(series, backupRows)
			}
		}
	}

	// Window capability from the newest snapshot among the account's bridges.
	var latestRaw []byte
	_ = s.DB.QueryRowContext(r.Context(), `SELECT snapshot FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL ORDER BY received_at DESC LIMIT 1`, uid).Scan(&latestRaw)
	windows := windowCapability(latestRaw, plan)

	daysWithData := 0
	firstDay := ""
	for _, e := range series {
		if e.ConsumedPP != nil {
			daysWithData++
			if firstDay == "" || e.BucketStart < firstDay {
				firstDay = e.BucketStart
			}
		}
	}
	reference := 0.0
	for _, w := range windows {
		if w.Present && w.DurationMinutes > 0 {
			perDay := 100 / (float64(w.DurationMinutes) / 1440)
			switch granularity {
			case "weekly":
				perDay *= 7
			case "monthly":
				perDay *= 30
			}
			reference += perDay
		}
	}
	compare := map[string]any(nil)
	if r.URL.Query().Get("compare") == "true" && (granularity == "weekly" || granularity == "monthly") {
		periodDays := map[string]int{"weekly": 7, "monthly": 30}[granularity]
		thisStart := bucketStart(now, loc, granularity)
		prevStart := thisStart.AddDate(0, 0, -periodDays)
		elapsed := now.Sub(thisStart)
		sumRange := func(from, to time.Time) float64 {
			sumv := 0.0
			for _, h := range hours {
				if !h.hour.Before(from) && h.hour.Before(to) {
					sumv += h.consumed
				}
			}
			return sumv
		}
		thisPP := sumRange(thisStart, now)
		prevPP := sumRange(prevStart, prevStart.Add(elapsed))
		compare = map[string]any{"this_pp": thisPP, "prev_pp": prevPP, "elapsed_hours": elapsed.Hours()}
		if prevPP > 0 {
			compare["ratio"] = thisPP / prevPP
		}
	}
	empty := len(hours) == 0 && !usedBackup
	httpx.WriteJSON(w, 200, map[string]any{
		"generated_at":       now,
		"tz_offset_minutes":  tz,
		"windows":            windows,
		"series":             series,
		"coverage":           map[string]any{"first_day": firstDay, "days_with_data": daysWithData, "granularity": granularity},
		"accumulating_since": firstDay,
		"reference_pp":       reference,
		"plan_type":          plan,
		"compare":            compare,
		"data_as_of":         observationValue(dataAsOf),
		"source":             sourceLabel(hasLive, usedBackup, empty),
		"stale_reason":       staleReason(granularity, dataAsOf, now),
	})
}

// windowDuration maps a window filter value to its duration in minutes.
func windowDuration(filter string) int64 {
	if filter == "300" {
		return 300
	}
	return 10080
}

// consumptionWindow returns the raw per-generation series for the live
// window view; every generation (reset) breaks the line.
func (s *Service) consumptionWindow(ctx context.Context, uid string, now time.Time, loc *time.Location, windowFilter string, plan string, backup LocalBackup, liveAt time.Time) (map[string]any, error) {
	var latestRaw []byte
	_ = s.DB.QueryRowContext(ctx, `SELECT snapshot FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL ORDER BY received_at DESC LIMIT 1`, uid).Scan(&latestRaw)
	windows := windowCapability(latestRaw, plan)
	// The desktop has observed samples the server never received as raw
	// history: serve its stored series instead of the stale live one.
	if windowFilter != "" && len(backup.Consumption) > 0 && codex.WindowVisibleForPlan(plan, windowDuration(windowFilter)) {
		if backupAt := backup.Freshness(); liveAt.IsZero() || backupAt.After(liveAt) {
			if value := backup.Consumption[windowFilter+":window"]; len(value) > 0 {
				var out map[string]any
				if json.Unmarshal(value, &out) == nil {
					if _, ok := out["windows"]; !ok {
						out["windows"] = windows
					}
					out["source"] = "local_backup"
					return out, nil
				}
			}
		}
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT observed_at,snapshot FROM codex_history WHERE bridge_id IN (SELECT id FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL) AND observed_at>now()-interval '7 days' ORDER BY observed_at`, uid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	type gen struct {
		ResetsAt *int64
		Start    time.Time
		End      time.Time
		Points   []consumptionPoint
	}
	gens := []*gen{}
	for rows.Next() {
		var observed time.Time
		var raw []byte
		if e = rows.Scan(&observed, &raw); e != nil {
			return nil, e
		}
		var snap codex.Snapshot
		if json.Unmarshal(raw, &snap) != nil {
			continue
		}
		for _, b := range snap.Buckets {
			for _, w := range []*codex.Window{b.Primary, b.Secondary} {
				if w == nil {
					continue
				}
				if !visibleConsumptionKey(windowKey(b, w), windowFilter, plan) {
					continue
				}
				var resetsAt *int64
				if w.ResetsAt != nil {
					r := *w.ResetsAt
					resetsAt = &r
				}
				var g *gen
				if len(gens) > 0 {
					g = gens[len(gens)-1]
				}
				sameGeneration := g != nil && ((g.ResetsAt == nil && resetsAt == nil) || (g.ResetsAt != nil && resetsAt != nil && *g.ResetsAt == *resetsAt))
				if !sameGeneration {
					g = &gen{ResetsAt: resetsAt, Start: observed}
					gens = append(gens, g)
				}
				g.End = observed
				g.Points = append(g.Points, consumptionPoint{T: observed.Format(time.RFC3339), Used: w.UsedPercent})
			}
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, e
	}
	outGens := []consumptionGeneration{}
	for _, g := range gens {
		if len(g.Points) == 0 {
			continue
		}
		outGens = append(outGens, consumptionGeneration{ResetsAt: g.ResetsAt, Start: g.Start.Format(time.RFC3339), End: g.End.Format(time.RFC3339), Points: g.Points})
	}
	return map[string]any{"generated_at": now, "windows": windows, "generations": outGens, "plan_type": plan, "source": "server_rollup"}, nil
}

// windowCapability reports which window durations the newest snapshot carried.
func windowCapability(latest []byte, plan string) []consumptionWindow {
	present := map[int64]bool{}
	if len(latest) > 0 {
		var snap codex.Snapshot
		if json.Unmarshal(latest, &snap) == nil {
			for _, b := range snap.Buckets {
				for _, w := range []*codex.Window{b.Primary, b.Secondary} {
					if w != nil {
						present[w.DurationMinutes] = true
					}
				}
			}
		}
	}
	out := []consumptionWindow{}
	for _, d := range []int64{300, 10080} {
		if !codex.WindowVisibleForPlan(plan, d) {
			continue
		}
		out = append(out, consumptionWindow{ID: "codex", DurationMinutes: d, Present: present[d]})
	}
	return out
}

// ensureRollup replays raw history into the rollup when the rollup lags the
// raw samples: the initial backfill and self-healing after a missed write.
// The replay recomputes every hour bucket from scratch and replaces the
// bridge's rollup rows atomically.
func (s *Service) ensureRollups(ctx context.Context, uid string) error {
	bridges := []string{}
	rows, e := s.DB.QueryContext(ctx, `SELECT id FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL`, uid)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		bridges = append(bridges, id)
	}
	rows.Close()
	for _, bridgeID := range bridges {
		if e := s.ensureRollup(ctx, bridgeID); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) ensureRollup(ctx context.Context, bridgeID string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7))`, bridgeID); e != nil {
		return e
	}
	var stale bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM codex_history h WHERE h.bridge_id=$1 AND h.observed_at>COALESCE((SELECT last_observed_at FROM codex_usage_rollup_state s WHERE s.bridge_id=$1),to_timestamp(0)))`, bridgeID).Scan(&stale)
	if e != nil || !stale {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT observed_at,snapshot FROM codex_history WHERE bridge_id=$1 ORDER BY observed_at`, bridgeID)
	if e != nil {
		return e
	}
	type acc struct {
		consumed float64
		samples  int
		resets   int
		level    float64
	}
	agg := map[string]*acc{}
	prevWindows := map[string]*codex.Window{}
	var lastObserved time.Time
	for rows.Next() {
		var observed time.Time
		var raw []byte
		if e = rows.Scan(&observed, &raw); e != nil {
			rows.Close()
			return e
		}
		lastObserved = observed
		var snap codex.Snapshot
		if json.Unmarshal(raw, &snap) != nil {
			continue
		}
		hour := observed.Truncate(time.Hour)
		for _, b := range snap.Buckets {
			for _, w := range []*codex.Window{b.Primary, b.Secondary} {
				if w == nil {
					continue
				}
				key := windowKey(b, w)
				delta, resets := windowDelta(prevWindows[key], w)
				k := hour.Format(time.RFC3339) + "|" + key
				a := agg[k]
				if a == nil {
					a = &acc{}
					agg[k] = a
				}
				a.consumed += delta
				a.samples++
				a.resets += resets
				a.level = w.UsedPercent
			}
		}
		next := map[string]*codex.Window{}
		for _, b := range snap.Buckets {
			for slot, w := range []*codex.Window{b.Primary, b.Secondary} {
				if w != nil {
					next[windowKey(b, w)] = []*codex.Window{b.Primary, b.Secondary}[slot]
				}
			}
		}
		prevWindows = next
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM codex_usage_rollup WHERE bridge_id=$1`, bridgeID); e != nil {
		return e
	}
	for k, a := range agg {
		parts := strings.SplitN(k, "|", 2)
		hour, e := time.Parse(time.RFC3339, parts[0])
		if e != nil {
			continue
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO codex_usage_rollup(bridge_id,hour_bucket,window_key,consumed_pp,samples,resets,level_last) VALUES($1,$2,$3,$4,$5,$6,$7)`, bridgeID, hour, parts[1], a.consumed, a.samples, a.resets, a.level); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO codex_usage_rollup_state(bridge_id,last_observed_at) VALUES($1,$2) ON CONFLICT (bridge_id) DO UPDATE SET last_observed_at=$2`, bridgeID, lastObserved); e != nil {
		return e
	}
	return tx.Commit()
}

// tzOffsetMinutes reads the account's preferred UTC offset (minutes).
func (s *Service) tzOffsetMinutes(ctx context.Context, uid string) int {
	var raw []byte
	if e := s.DB.QueryRowContext(ctx, `SELECT settings FROM codex_preferences WHERE user_id=$1`, uid).Scan(&raw); e == nil {
		var p struct {
			UTCOffset int `json:"utc_offset_minutes"`
		}
		if json.Unmarshal(raw, &p) == nil && p.UTCOffset >= -720 && p.UTCOffset <= 840 {
			return p.UTCOffset
		}
	}
	return codex.Defaults().UTCOffset
}

// bucketStart / nextBucket walk the calendar grid for the given granularity.
func bucketStart(t time.Time, loc *time.Location, granularity string) time.Time {
	local := t.In(loc)
	switch granularity {
	case "weekly":
		off := (int(local.Weekday()) + 6) % 7
		d := local.AddDate(0, 0, -off)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	case "monthly":
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	}
}

func nextBucket(t time.Time, loc *time.Location, granularity string) time.Time {
	switch granularity {
	case "weekly":
		return t.AddDate(0, 0, 7)
	case "monthly":
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}

// visibleConsumptionKey applies the same policy to raw and rolled-up windows.
func visibleConsumptionKey(key, filter, plan string) bool {
	return (codex.WindowVisibleForPlan(plan, 300) || !strings.HasSuffix(key, ":300")) &&
		(filter == "" || strings.HasSuffix(key, ":"+filter))
}

// consumptionHourly returns the per-hour consumed series from the rollup,
// optionally filtered to one window duration.
func (s *Service) consumptionHourly(ctx context.Context, uid string, now time.Time, qp url.Values, plan string, backup LocalBackup) (map[string]any, error) {
	hours := 24
	if v := qp.Get("hours"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 1 && n <= 168 {
			hours = n
		}
	}
	if e := s.ensureRollups(ctx, uid); e != nil {
		return nil, e
	}
	windowFilter := effectiveWindowFilter(qp.Get("window"), plan)
	// One effective source per hour bucket across all observing devices.
	rows, e := s.DB.QueryContext(ctx, `SELECT hour_bucket,window_key,consumed_pp,samples,resets FROM (SELECT r.hour_bucket,r.window_key,r.consumed_pp,r.samples,r.resets,row_number() OVER (PARTITION BY r.hour_bucket,r.window_key ORDER BY COALESCE(st.last_observed_at,to_timestamp(0)) DESC,r.samples DESC,r.bridge_id) AS rn FROM codex_usage_rollup r JOIN codex_bridges b ON b.id=r.bridge_id AND b.user_id=$1 AND b.revoked_at IS NULL LEFT JOIN codex_usage_rollup_state st ON st.bridge_id=r.bridge_id WHERE r.hour_bucket>now()-($2 * interval '1 hour')) ranked WHERE rn=1 ORDER BY hour_bucket`, uid, hours)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	live := []hourlyEntry{}
	for rows.Next() {
		var hour time.Time
		var key string
		var consumed float64
		var samples, resets int
		if e = rows.Scan(&hour, &key, &consumed, &samples, &resets); e != nil {
			return nil, e
		}
		if !visibleConsumptionKey(key, windowFilter, plan) {
			continue
		}
		live = append(live, hourlyEntry{Hour: hour.Format(time.RFC3339), Consumed: consumed, Samples: samples, Resets: resets})
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	// Desktop hourly aggregates fill hours the rollup lacks.
	var backupRows []hourlyEntry
	if codex.WindowVisibleForPlan(plan, windowDuration(windowFilter)) {
		backupRows = backupHourly(backup, windowFilter+":hourly")
	}
	merged, usedBackup := mergeHourly(live, backupRows)
	out := []map[string]any{}
	for _, row := range merged {
		out = append(out, map[string]any{"hour": row.Hour, "consumed_pp": row.Consumed, "samples": row.Samples, "resets": row.Resets})
	}
	return map[string]any{"hours": hours, "hourly": out, "source": sourceLabel(len(live) > 0, usedBackup, len(merged) == 0)}, nil
}

// Unknown observation time stays null, never response time or year 0001.
func observationValue(at time.Time) any {
	if at.IsZero() {
		return nil
	}
	return at
}
