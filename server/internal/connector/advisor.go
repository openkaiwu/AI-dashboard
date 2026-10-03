package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/httpx"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (s *Service) Preferences(w http.ResponseWriter, r *http.Request) {
	var p codex.Preferences
	if httpx.Decode(r, &p) != nil || !p.Valid() {
		httpx.Error(w, 400, "invalid_preferences", "规则参数不在允许范围内")
		return
	}
	raw, _ := json.Marshal(p)
	_, e := s.DB.ExecContext(r.Context(), `INSERT INTO codex_preferences(user_id,settings) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET settings=excluded.settings`, auth.Who(r).UserID, raw)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	httpx.WriteJSON(w, 200, p)
}
func (s *Service) AlertAction(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Action string `json:"action"`
	}
	if httpx.Decode(r, &q) != nil || (q.Action != "dismiss" && q.Action != "snooze" && q.Action != "restore") {
		httpx.Error(w, 400, "invalid_action", "无效操作")
		return
	}
	result, e := s.DB.ExecContext(r.Context(), `UPDATE codex_alerts SET notified_at=CASE WHEN $3='snooze' THEN now()-interval '2 days' ELSE notified_at END,dismissed=CASE WHEN $3='dismiss' THEN true WHEN $3='restore' THEN false ELSE dismissed END,snoozed_until=CASE WHEN $3='snooze' THEN now()+interval '1 hour' WHEN $3='restore' THEN NULL ELSE snoozed_until END WHERE user_id=$1 AND id=$2`, auth.Who(r).UserID, r.PathValue("id"), q.Action)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "操作失败")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.Error(w, 404, "not_found", "提醒不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Service) Overview(w http.ResponseWriter, r *http.Request) {
	out, e := s.Evaluate(r.Context(), auth.Who(r).UserID, time.Now())
	if e != nil {
		httpx.Error(w, 503, "unavailable", "暂时无法生成分析")
		return
	}
	httpx.WriteJSON(w, 200, out)
}

func (s *Service) Evaluate(ctx context.Context, uid string, now time.Time) (map[string]any, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,42))`, uid); e != nil {
		return nil, e
	}
	plan, planSource, e := getPlan(ctx, tx, uid)
	if e != nil {
		return nil, e
	}
	p := codex.Defaults()
	var raw []byte
	e = tx.QueryRowContext(ctx, `SELECT settings FROM codex_preferences WHERE user_id=$1`, uid).Scan(&raw)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if len(raw) > 0 {
		if e = json.Unmarshal(raw, &p); e != nil {
			return nil, e
		}
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,name,snapshot FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at`, uid)
	if e != nil {
		return nil, e
	}
	type device struct {
		ID, Name string
		Snapshot codex.Snapshot
	}
	devices := []device{}
	for rows.Next() {
		var d device
		d.Snapshot.Buckets = []codex.Bucket{}
		var data []byte
		if e = rows.Scan(&d.ID, &d.Name, &data); e != nil {
			rows.Close()
			return nil, e
		}
		if len(data) > 0 {
			if e = json.Unmarshal(data, &d.Snapshot); e != nil {
				rows.Close()
				return nil, e
			}
		}
		d.Snapshot = d.Snapshot.ForPlan(plan)
		devices = append(devices, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE codex_alerts SET active=false WHERE user_id=$1`, uid); e != nil {
		return nil, e
	}
	global := globalNews(ctx, tx)
	reports := []map[string]any{}
	for _, d := range devices {
		history := []codex.Snapshot{}
		h, e := tx.QueryContext(ctx, `SELECT snapshot FROM (SELECT observed_at,snapshot FROM codex_history WHERE bridge_id=$1 AND observed_at>$2 ORDER BY observed_at DESC LIMIT 500) t ORDER BY observed_at`, d.ID, now.Add(-6*time.Hour))
		if e != nil {
			return nil, e
		}
		for h.Next() {
			var data []byte
			var v codex.Snapshot
			if e = h.Scan(&data); e != nil {
				h.Close()
				return nil, e
			}
			if e = json.Unmarshal(data, &v); e != nil {
				h.Close()
				return nil, e
			}
			history = append(history, v.ForPlan(plan))
		}
		e = h.Err()
		h.Close()
		if e != nil {
			return nil, e
		}
		d.Snapshot = fillNews(d.Snapshot, global)
		a := codex.Analyze(d.Snapshot, history, p, now)
		reports = append(reports, map[string]any{"id": d.ID, "name": d.Name, "snapshot": d.Snapshot, "analysis": a, "history": history})
		if p.Enabled {
			for _, v := range a.Advice {
				key := d.ID + ":" + v.Key
				if v.Kind == "credit" || v.Kind == "news" {
					key = v.Key
				}
				payload, _ := json.Marshal(v)
				_, e = tx.ExecContext(ctx, `INSERT INTO codex_alerts(user_id,id,payload,first_seen,last_seen,notified_at) VALUES($1,$2,$3,$4,$4,$4) ON CONFLICT(user_id,id) DO UPDATE SET payload=excluded.payload,active=true,last_seen=excluded.last_seen,notified_at=CASE WHEN NOT codex_alerts.dismissed AND (codex_alerts.snoozed_until IS NULL OR codex_alerts.snoozed_until<=$4) AND codex_alerts.notified_at<=$5 AND NOT $6 AND $7 THEN $4 ELSE codex_alerts.notified_at END`, uid, key, payload, now, now.Add(-time.Duration(p.CooldownHours)*time.Hour), p.Quiet(now), v.Kind != "news")
				if e != nil {
					return nil, e
				}
			}
		}
	}
	alerts := []map[string]any{}
	rows, e = tx.QueryContext(ctx, `SELECT id,payload,first_seen,notified_at,dismissed,snoozed_until FROM codex_alerts WHERE user_id=$1 AND active ORDER BY first_seen DESC LIMIT 100`, uid)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id string
		var raw []byte
		var first, notified time.Time
		var dismissed bool
		var snooze sql.NullTime
		var payload any
		if e = rows.Scan(&id, &raw, &first, &notified, &dismissed, &snooze); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(raw, &payload); e != nil {
			rows.Close()
			return nil, e
		}
		var until any
		if snooze.Valid {
			until = snooze.Time
		}
		alerts = append(alerts, map[string]any{"id": id, "advice": payload, "first_seen": first, "notified_at": notified, "dismissed": dismissed, "snoozed_until": until})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	// Bound inactive history while preserving active dismissal/snooze state.
	if _, e = tx.ExecContext(ctx, `DELETE FROM codex_alerts WHERE user_id=$1 AND NOT active AND last_seen<$2`, uid, now.Add(-30*24*time.Hour)); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return map[string]any{"generated_at": now, "preferences": p, "quiet": p.Quiet(now), "devices": reports, "alerts": alerts, "news": global, "plan": map[string]string{"plan_type": plan, "source_type": planSource}}, nil
}

// globalNews reads the promoted reset-radar news (admin bridge snapshots);
// nil when nothing has been promoted yet or the row is unreadable.
func globalNews(ctx context.Context, tx *sql.Tx) *codex.News {
	var raw []byte
	if e := tx.QueryRowContext(ctx, `SELECT payload FROM global_codex_news WHERE id=1`).Scan(&raw); e != nil || len(raw) == 0 {
		return nil
	}
	var n codex.News
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	return &n
}

// fillNews lets the promoted global radar back a device that has no news of
// its own (or only an older check); a device's own newer check always wins.
func fillNews(s codex.Snapshot, global *codex.News) codex.Snapshot {
	if global == nil {
		return s
	}
	if s.News == nil || s.News.CheckedAt.Before(global.CheckedAt) {
		s.News = global
	}
	return s
}
