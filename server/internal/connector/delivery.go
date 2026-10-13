package connector

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/httpx"
)

// Delivery reserves one account event for one device. A lost client expires
// after two minutes; a successful delivery consumes only this notification
// generation. Read/dismiss/snooze still belong to the independent event state.
func (s *Service) Delivery(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Action  string    `json:"action"`
		Token   time.Time `json:"token"`
		ClaimID string    `json:"claim_id"`
	}
	if httpx.Decode(r, &q) != nil || (q.Action != "claim" && q.Action != "delivered" && q.Action != "release") || (q.Action == "claim" && q.Token.IsZero()) || (q.Action != "claim" && q.ClaimID == "") {
		httpx.Error(w, 400, "invalid_delivery", "无效的提醒送达请求")
		return
	}
	who, id := auth.Who(r), r.PathValue("id")
	if q.Action != "claim" {
		query := `UPDATE codex_alert_deliveries SET delivered_at=COALESCE(delivered_at,now()),lease_until=now() WHERE user_id=$1 AND alert_id=$2 AND claim_id=$3 AND device_id=$4`
		if q.Action == "release" {
			query = `DELETE FROM codex_alert_deliveries WHERE user_id=$1 AND alert_id=$2 AND claim_id=$3 AND device_id=$4`
		}
		result, err := s.DB.ExecContext(r.Context(), query, who.UserID, id, q.ClaimID, who.DeviceID)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "提醒送达状态暂时无法保存")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			httpx.Error(w, 409, "delivery_changed", "提醒送达请求已失效")
			return
		}
		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	now := time.Now().UTC()
	// Re-check fresh snapshots and current thresholds before reserving a popup.
	if _, err := s.Evaluate(r.Context(), who.UserID, now); err != nil {
		httpx.Error(w, 503, "unavailable", "暂时无法确认提醒")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "暂时无法确认提醒")
		return
	}
	defer tx.Rollback()
	fail := func() { httpx.Error(w, 503, "unavailable", "暂时无法确认提醒") }
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,42))`, who.UserID); err != nil {
		fail()
		return
	}
	p := codex.Defaults()
	var settings []byte
	err = tx.QueryRowContext(r.Context(), `SELECT settings FROM codex_preferences WHERE user_id=$1`, who.UserID).Scan(&settings)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail()
		return
	}
	if len(settings) > 0 && json.Unmarshal(settings, &p) != nil {
		fail()
		return
	}
	decline := func(reason string) { httpx.WriteJSON(w, 200, map[string]any{"claimed": false, "reason": reason}) }
	if !p.Enabled || p.Quiet(now) {
		decline("paused")
		return
	}
	var raw []byte
	var token, read, snooze sql.NullTime
	var active, dismissed bool
	err = tx.QueryRowContext(r.Context(), `SELECT payload,notified_at,read_at,snoozed_until,active,dismissed FROM codex_alerts WHERE user_id=$1 AND id=$2 FOR UPDATE`, who.UserID, id).Scan(&raw, &token, &read, &snooze, &active, &dismissed)
	if errors.Is(err, sql.ErrNoRows) {
		decline("inactive")
		return
	}
	if err != nil {
		fail()
		return
	}
	var advice codex.Advice
	if json.Unmarshal(raw, &advice) != nil {
		fail()
		return
	}
	if !active || dismissed || read.Valid || (snooze.Valid && snooze.Time.After(now)) || !advice.Interrupt || !token.Valid || !token.Time.Equal(q.Token) {
		decline("changed")
		return
	}
	var oldToken, lease sql.NullTime
	var delivered sql.NullTime
	err = tx.QueryRowContext(r.Context(), `SELECT token,lease_until,delivered_at FROM codex_alert_deliveries WHERE user_id=$1 AND alert_id=$2`, who.UserID, id).Scan(&oldToken, &lease, &delivered)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail()
		return
	}
	if oldToken.Valid && oldToken.Time.Equal(q.Token) && (delivered.Valid || (lease.Valid && lease.Time.After(now))) {
		decline("reserved")
		return
	}
	var previousPriority int
	err = tx.QueryRowContext(r.Context(), `SELECT priority FROM codex_alert_deliveries WHERE user_id=$1 AND COALESCE(delivered_at,claimed_at)>$2 AND (delivered_at IS NOT NULL OR lease_until>$3) ORDER BY COALESCE(delivered_at,claimed_at) DESC LIMIT 1`, who.UserID, now.Add(-10*time.Minute), now).Scan(&previousPriority)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail()
		return
	}
	if err == nil && advice.Priority <= previousPriority {
		decline("spacing")
		return
	}
	claim := httpx.NewID("delivery")
	_, err = tx.ExecContext(r.Context(), `INSERT INTO codex_alert_deliveries(user_id,alert_id,token,claim_id,device_id,priority,claimed_at,lease_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(user_id,alert_id) DO UPDATE SET token=excluded.token,claim_id=excluded.claim_id,device_id=excluded.device_id,priority=excluded.priority,claimed_at=excluded.claimed_at,lease_until=excluded.lease_until,delivered_at=NULL`, who.UserID, id, q.Token, claim, who.DeviceID, advice.Priority, now, now.Add(2*time.Minute))
	if err != nil {
		fail()
		return
	}
	if err = tx.Commit(); err != nil {
		fail()
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"claimed": true, "claim_id": claim})
}
