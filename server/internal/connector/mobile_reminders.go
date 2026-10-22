package connector

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/httpx"
)

type MobileEvent struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Priority int    `json:"priority"`
	Group    string `json:"group"`
	LegacyID string `json:"legacy_id,omitempty"`
}

// Semantic identity excludes presentation generations and priority escalation.
func SemanticAdvice(key, reason string) (string, string) {
	if strings.HasPrefix(key, "credit:") {
		return strings.TrimSuffix(strings.TrimSuffix(key, ":urgent"), ":last-chance"), strings.TrimSuffix(strings.TrimSuffix(key, ":urgent"), ":last-chance")
	}
	parts := strings.Split(key, ":")
	if len(parts) < 3 {
		return key, key
	}
	kind := reason
	if strings.Contains(key, ":exhausted") {
		reason = "quota_exhausted"
	} else if strings.Contains(key, ":low") {
		reason = "quota_low"
	} else if strings.Contains(key, ":sustained") || strings.HasSuffix(key, ":rapid") {
		reason = "rapid_consumption"
	}
	switch reason {
	case "pace_ahead", "runway_short", "rapid_consumption":
		kind = "rapid"
	case "quota_low":
		kind = "low"
	case "quota_exhausted":
		kind = "exhausted"
	}
	return strings.Join(parts[:3], ":") + ":" + kind, strings.Join(parts[:2], ":")
}

func (s *Service) mobileEvents(ctx context.Context, uid string, now time.Time) ([]MobileEvent, bool, error) {
	overview, e := s.Evaluate(ctx, uid, now)
	if e != nil {
		return nil, false, e
	}
	quiet, _ := overview["quiet"].(bool)
	events := []MobileEvent{}
	// Track trusted reset boundaries separately from the reset timestamp text.
	cycles := map[string]string{}
	for _, report := range overview["devices"].([]map[string]any) {
		snapshot := report["snapshot"].(codex.Snapshot)
		if snapshot.Status != "ok" || now.Sub(snapshot.ObservedAt) > 11*time.Minute {
			continue
		}
		for _, b := range snapshot.Buckets {
			for slot, w := range []*codex.Window{b.Primary, b.Secondary} {
				if w == nil {
					continue
				}
				window := fmt.Sprintf("%s:%d", b.ID, slot)
				var cycle string
				var reset sql.NullInt64
				var observed time.Time
				err := s.DB.QueryRowContext(ctx, `SELECT cycle_id,reset_at,observed_at FROM mobile_reminder_cycles WHERE user_id=$1 AND window_id=$2`, uid, window).Scan(&cycle, &reset, &observed)
				if err != nil && err != sql.ErrNoRows {
					return nil, false, err
				}
				// A corrected future reset (including unknown -> known) retains identity.
				if err == sql.ErrNoRows || (reset.Valid && w.ResetsAt != nil && now.Unix() >= reset.Int64 && *w.ResetsAt > reset.Int64+900 && snapshot.ObservedAt.After(observed)) {
					cycle = "unknown"
					if w.ResetsAt != nil {
						cycle = strconv.FormatInt(*w.ResetsAt, 10)
					}
				}
				_, err = s.DB.ExecContext(ctx, `INSERT INTO mobile_reminder_cycles(user_id,window_id,cycle_id,reset_at,observed_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,window_id) DO UPDATE SET cycle_id=$3,reset_at=COALESCE($4,mobile_reminder_cycles.reset_at),observed_at=$5 WHERE mobile_reminder_cycles.observed_at<=$5`, uid, window, cycle, w.ResetsAt, snapshot.ObservedAt)
				if err != nil {
					return nil, false, err
				}
				cycles[window] = cycle
			}
		}
	}
	for _, row := range overview["alerts"].([]map[string]any) {
		a := row["advice"].(map[string]any)
		if a["interrupt"] != true || row["dismissed"] == true || row["read_at"] != nil || row["notified_at"] == nil {
			continue
		}
		if until, ok := row["snoozed_until"].(time.Time); ok && until.After(now) {
			continue
		}
		key, _ := a["key"].(string)
		reason, _ := a["reason"].(string)
		id, group := SemanticAdvice(key, reason)
		if cycle, ok := cycles[group]; ok {
			parts := strings.Split(id, ":")
			if len(parts) >= 4 {
				parts[2] = cycle
				id = strings.Join(parts, ":")
			}
		}
		var presented bool
		// Import all legacy card urgency levels and cycle variants without replay.
		rows, err := s.DB.QueryContext(ctx, `SELECT advice_key FROM mobile_legacy_presented WHERE user_id=$1`, uid)
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var old string
			if err = rows.Scan(&old); err != nil {
				rows.Close()
				return nil, false, err
			}
			legacy, _ := SemanticAdvice(old, reason)
			if legacy == id {
				presented = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false, err
		}
		if presented {
			continue
		}
		events = append(events, MobileEvent{"codex:" + id, a["title"].(string), a["body"].(string), int(a["priority"].(float64)), "codex:" + group, "codex:" + row["id"].(string)})
	}
	// Generic notifications share the same account-wide mobile delivery gate.
	rows, e := s.DB.QueryContext(ctx, `SELECT n.id,n.dedupe_key,n.title,n.body,n.severity,COALESCE(n.quota_bucket_id,''),COALESCE(p.slug,'') FROM notifications n LEFT JOIN provider_accounts a ON a.id=n.provider_account_id LEFT JOIN providers p ON p.id=a.provider_id WHERE n.user_id=$1 AND n.status='unread' AND (n.rule_id IS NULL OR EXISTS(SELECT 1 FROM alert_states st WHERE st.user_id=n.user_id AND st.notification_id=n.id)) AND n.severity IN ('warning','critical') ORDER BY n.created_at DESC LIMIT 100`, uid)
	if e != nil {
		return nil, false, e
	}
	for rows.Next() {
		var legacyID, key, title, body, severity, bucket, provider string
		if e = rows.Scan(&legacyID, &key, &title, &body, &severity, &bucket, &provider); e != nil {
			rows.Close()
			return nil, false, e
		}
		if strings.HasPrefix(key, "codex_alert:") || strings.HasPrefix(key, "quota.reset_") || strings.HasPrefix(key, "connector.") || (provider == "codex" && strings.HasPrefix(key, "quota.")) {
			continue
		}
		if strings.HasPrefix(key, "quota.low:") && bucket != "" {
			var fresh bool
			if e = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM usage_snapshots WHERE quota_bucket_id=$1 AND observed_at::timestamptz>now()-interval '11 minutes')`, bucket).Scan(&fresh); e != nil {
				rows.Close()
				return nil, false, e
			}
			if !fresh {
				continue
			}
			raw := strings.TrimPrefix(key, "quota.low:"+bucket+":")
			resetAt, parseErr := time.Parse(time.RFC3339, raw)
			var resetValue any
			cycle := "unknown"
			if parseErr == nil {
				cycle = strconv.FormatInt(resetAt.Unix(), 10)
				resetValue = resetAt.Unix()
			}
			var old string
			var oldReset sql.NullInt64
			err := s.DB.QueryRowContext(ctx, `SELECT cycle_id,reset_at FROM mobile_reminder_cycles WHERE user_id=$1 AND window_id=$2`, uid, "generic:"+bucket).Scan(&old, &oldReset)
			if err != nil && err != sql.ErrNoRows {
				rows.Close()
				return nil, false, err
			}
			if err == nil && !(oldReset.Valid && parseErr == nil && now.Unix() >= oldReset.Int64 && resetAt.Unix() > oldReset.Int64+900) {
				cycle = old
			}
			if _, e = s.DB.ExecContext(ctx, `INSERT INTO mobile_reminder_cycles(user_id,window_id,cycle_id,reset_at,observed_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,window_id) DO UPDATE SET cycle_id=$3,reset_at=COALESCE($4,mobile_reminder_cycles.reset_at),observed_at=$5`, uid, "generic:"+bucket, cycle, resetValue, now); e != nil {
				rows.Close()
				return nil, false, e
			}
			key = "quota.low:" + bucket + ":" + cycle
		}
		priority := 2
		if severity == "critical" {
			priority = 4
		}
		events = append(events, MobileEvent{"generic:" + key, title, body, priority, "generic:" + bucket, legacyID})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, false, e
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Priority > events[j].Priority })
	unique := map[string]bool{}
	groups := map[string]int{}
	out := []MobileEvent{}
	for _, ev := range events {
		if unique[ev.ID] {
			continue
		}
		unique[ev.ID] = true
		// An exhausted window subsumes weaker reasons in the same batch.
		if p := groups[ev.Group]; p >= 4 && ev.Priority < 4 {
			continue
		}
		groups[ev.Group] = ev.Priority
		var seen bool
		if e = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mobile_reminder_receipts WHERE user_id=$1 AND event_id=$2)`, uid, ev.ID).Scan(&seen); e != nil {
			return nil, false, e
		}
		if !seen {
			out = append(out, ev)
		}
	}
	return out, quiet, nil
}

func (s *Service) MobileReminders(w http.ResponseWriter, r *http.Request) {
	who := auth.Who(r)
	if who.Kind != "mobile" {
		httpx.Error(w, 403, "mobile_required", "仅主提醒手机可接收")
		return
	}
	var device string
	err := s.DB.QueryRowContext(r.Context(), `SELECT device_id FROM mobile_reminder_settings WHERE user_id=$1`, who.UserID).Scan(&device)
	if err != nil && err != sql.ErrNoRows {
		httpx.Error(w, 503, "unavailable", "提醒设置暂不可读")
		return
	}
	if r.Method == "PUT" {
		_, err = s.DB.ExecContext(r.Context(), `INSERT INTO mobile_reminder_settings(user_id,device_id) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET device_id=$2,updated_at=now()`, who.UserID, who.DeviceID)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "主手机保存失败")
			return
		}
		httpx.WriteJSON(w, 200, map[string]bool{"primary": true})
		return
	}
	if device != who.DeviceID {
		httpx.WriteJSON(w, 200, map[string]any{"primary": false, "events": []MobileEvent{}, "reason": "not_primary"})
		return
	}
	events, quiet, err := s.mobileEvents(r.Context(), who.UserID, time.Now().UTC())
	if err != nil {
		httpx.Error(w, 503, "unavailable", "提醒检查暂不可用")
		return
	}
	if r.Method == "GET" {
		var source sql.NullTime
		err = s.DB.QueryRowContext(r.Context(), `SELECT max(t) FROM (SELECT (snapshot->>'observed_at')::timestamptz t FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL AND snapshot IS NOT NULL UNION ALL SELECT us.observed_at::timestamptz FROM usage_snapshots us JOIN quota_buckets qb ON qb.id=us.quota_bucket_id JOIN provider_accounts pa ON pa.id=qb.provider_account_id WHERE pa.user_id=$1) samples`, who.UserID).Scan(&source)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "采集状态不可读")
			return
		}
		reason := "no_new_events"
		var sourceAt any
		if source.Valid {
			sourceAt = source.Time
			if time.Since(source.Time) > 11*time.Minute {
				reason = "stale_source"
			}
		} else {
			reason = "no_source"
		}
		httpx.WriteJSON(w, 200, map[string]any{"primary": true, "events": events, "quiet": quiet, "generated_at": time.Now().UTC(), "source_observed_at": sourceAt, "reason": reason})
		return
	}

	var q struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
		Batch  string   `json:"batch_id"`
	}
	if httpx.Decode(r, &q) != nil || len(q.IDs) > 100 || (q.Action != "reserve" && q.Action != "submitted" && q.Action != "release" && q.Action != "migrate") {
		httpx.Error(w, 400, "invalid_delivery", "无效送达请求")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "送达暂不可用")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,116))`, who.UserID); err != nil {
		httpx.Error(w, 503, "unavailable", "送达暂不可用")
		return
	}
	if q.Action != "reserve" && q.Action != "migrate" {
		if q.Batch == "" {
			httpx.Error(w, 400, "invalid_delivery", "缺少送达编号")
			return
		}
		query := `UPDATE mobile_reminder_receipts SET submitted_at=COALESCE(submitted_at,now()) WHERE user_id=$1 AND device_id=$2 AND batch_id=$3`
		if q.Action == "release" {
			query = `DELETE FROM mobile_reminder_receipts WHERE user_id=$1 AND device_id=$2 AND batch_id=$3 AND submitted_at IS NULL`
		}
		_, err = tx.ExecContext(r.Context(), query, who.UserID, who.DeviceID, q.Batch)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			httpx.Error(w, 503, "unavailable", "回执保存失败")
			return
		}
		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	decline := func(reason string) { httpx.WriteJSON(w, 200, map[string]any{"claimed": false, "reason": reason}) }
	if quiet && q.Action != "migrate" {
		decline("quiet_hours")
		return
	}
	// A primary-device change cannot race a reservation.
	if err = tx.QueryRowContext(r.Context(), `SELECT device_id FROM mobile_reminder_settings WHERE user_id=$1 FOR UPDATE`, who.UserID).Scan(&device); err != nil || device != who.DeviceID {
		decline("not_primary")
		return
	}
	var recent bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM mobile_reminder_receipts WHERE user_id=$1 AND reserved_at>now()-interval '30 minutes')`, who.UserID).Scan(&recent)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "限频状态不可读")
		return
	}
	if recent && q.Action != "migrate" {
		decline("rate_limited")
		return
	}
	selected := []MobileEvent{}
	wanted := map[string]bool{}
	for _, id := range q.IDs {
		wanted[id] = true
	}
	batch := httpx.NewID("mobile")
	for _, ev := range events {
		if !wanted[ev.ID] {
			continue
		}
		result, e := tx.ExecContext(r.Context(), `INSERT INTO mobile_reminder_receipts(user_id,event_id,batch_id,device_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, who.UserID, ev.ID, batch, who.DeviceID)
		if e != nil {
			httpx.Error(w, 503, "unavailable", "送达保存失败")
			return
		}
		if q.Action == "migrate" {
			_, e = tx.ExecContext(r.Context(), `UPDATE mobile_reminder_receipts SET reserved_at=to_timestamp(0),submitted_at=now() WHERE user_id=$1 AND event_id=$2`, who.UserID, ev.ID)
			if e != nil {
				httpx.Error(w, 503, "unavailable", "迁移保存失败")
				return
			}
		}
		n, _ := result.RowsAffected()
		if n > 0 {
			if strings.HasPrefix(ev.ID, "codex:") && strings.HasSuffix(ev.ID, ":exhausted") {
				for _, kind := range []string{"low", "rapid"} {
					alias := strings.TrimSuffix(ev.ID, ":exhausted") + ":" + kind
					if _, e = tx.ExecContext(r.Context(), `INSERT INTO mobile_reminder_receipts(user_id,event_id,batch_id,device_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, who.UserID, alias, batch, who.DeviceID); e != nil {
						httpx.Error(w, 503, "unavailable", "合并状态保存失败")
						return
					}
				}
			}
			selected = append(selected, ev)
		}
	}
	if len(selected) == 0 {
		decline("duplicate_or_inactive")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Error(w, 503, "unavailable", "送达保存失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"claimed": true, "batch_id": batch, "events": selected})
}
