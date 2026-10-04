package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/httpx"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	DB              *sql.DB
	AfterQuotaWrite func(context.Context, string) error
}

func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name string `json:"name"`
	}
	if httpx.Decode(r, &q) != nil || len(strings.TrimSpace(q.Name)) == 0 || len(q.Name) > 120 {
		httpx.Error(w, 400, "invalid_input", "请输入电脑名称")
		return
	}
	id, token := httpx.NewID("bridge"), httpx.Token()
	uid, e := auth.DesktopDeviceUser(r.Context(), s.DB, auth.Who(r).DeviceID)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 403, "desktop_required", "请在已绑定的桌面设备创建连接")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "无法创建连接")
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "无法创建连接")
		return
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, uid); e != nil {
		httpx.Error(w, 503, "unavailable", "无法创建连接")
		return
	}
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO codex_bridges(id,user_id,name,token_hash,device_id) VALUES($1,$2,$3,$4,$5)`, id, uid, strings.TrimSpace(q.Name), auth.Hash(token), auth.Who(r).DeviceID); e != nil {
		httpx.Error(w, 503, "unavailable", "无法创建连接")
		return
	}
	if e = tx.Commit(); e != nil {
		httpx.Error(w, 503, "unavailable", "无法创建连接")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 201, map[string]string{"id": id, "token": token})
}
func (s *Service) Revoke(w http.ResponseWriter, r *http.Request) {
	result, e := s.DB.ExecContext(r.Context(), `UPDATE codex_bridges SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, r.PathValue("id"), auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "撤销失败")
		return
	}
	n, e := result.RowsAffected()
	if e != nil {
		httpx.Error(w, 503, "unavailable", "撤销失败")
		return
	}
	if n == 0 {
		httpx.Error(w, 404, "not_found", "连接不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Service) Upload(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.Error(w, 401, "unauthenticated", "连接未授权")
		return
	}
	var q codex.Snapshot
	if httpx.Decode(r, &q) != nil || q.Validate(time.Now()) != nil {
		httpx.Error(w, 400, "invalid_snapshot", "采集数据格式无效")
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	defer tx.Rollback()
	var bridgeID, deviceID, bridgeOwner string
	var old []byte
	e = tx.QueryRowContext(r.Context(), `SELECT b.id,b.device_id,b.user_id,b.snapshot FROM codex_bridges b WHERE b.token_hash=$1 AND b.revoked_at IS NULL FOR UPDATE`, auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&bridgeID, &deviceID, &bridgeOwner, &old)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 401, "bridge_revoked", "连接已撤销，请重新配对")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	if e := auth.EnsureActiveDesktopDevice(r.Context(), tx, deviceID); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			httpx.Error(w, 401, "bridge_revoked", "连接已撤销，请重新配对")
			return
		}
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	var previous codex.Snapshot
	if len(old) > 0 {
		if e = json.Unmarshal(old, &previous); e != nil {
			httpx.Error(w, 503, "unavailable", "数据暂不可用")
			return
		}
	}
	// A lost response can be retried. Older/equal samples never overwrite newer ones.
	applied := len(old) == 0 || q.ObservedAt.After(previous.ObservedAt)
	if applied {
		raw, _ := json.Marshal(q)
		_, e = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET snapshot=$1,received_at=now() WHERE id=$2`, raw, bridgeID)
		if e == nil {
			_, e = tx.ExecContext(r.Context(), `INSERT INTO codex_history(bridge_id,observed_at,snapshot) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, bridgeID, q.ObservedAt, raw)
		}
		if e == nil {
			_, e = tx.ExecContext(r.Context(), `DELETE FROM codex_history WHERE bridge_id=$1 AND observed_at<now()-interval '7 days'`, bridgeID)
		}
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	if applied && q.News != nil {
		s.promoteGlobalNews(r.Context(), bridgeOwner, q.News)
	}
	if applied {
		s.rollupUsage(r.Context(), bridgeID, len(old) > 0, &previous, &q)
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true, "applied": applied})
}

// windowKey identifies a window slot across snapshots.
func windowKey(b codex.Bucket, w *codex.Window) string {
	d := int64(0)
	if w != nil {
		d = w.DurationMinutes
	}
	return b.ID + ":" + strconv.FormatInt(d, 10)
}

// windowDelta folds the previous sample of the same window slot into the
// current one: consumed percentage points, plus a reset count when the window
// generation changed (or a Codex-side correction looked like one).
func windowDelta(prev *codex.Window, cur *codex.Window) (float64, int) {
	if prev == nil || cur == nil || prev.ResetsAt == nil || cur.ResetsAt == nil {
		return 0, 0
	}
	if *prev.ResetsAt == *cur.ResetsAt {
		if cur.UsedPercent < prev.UsedPercent-2 {
			return 0, 1 // drop too large for noise: counted as a reset
		}
		return math.Max(0, cur.UsedPercent-prev.UsedPercent), 0
	}
	return 0, 1 // generation change: the window was reset
}

// rollupUsage folds the delta between the previously stored snapshot and the
// current one into the hourly usage rollup. Best effort by design: analytics
// never break the quota ingest, and the consumption endpoint replays raw
// history when a rollup write was missed.
func (s *Service) rollupUsage(ctx context.Context, bridgeID string, hasPrev bool, prev *codex.Snapshot, cur *codex.Snapshot) {
	hour := cur.ObservedAt.Truncate(time.Hour)
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return
	}
	defer tx.Rollback()
	for _, b := range cur.Buckets {
		for slot, w := range []*codex.Window{b.Primary, b.Secondary} {
			if w == nil {
				continue
			}
			var prevW *codex.Window
			if hasPrev {
				for _, pb := range prev.Buckets {
					if pb.ID == b.ID {
						prevW = []*codex.Window{pb.Primary, pb.Secondary}[slot]
						break
					}
				}
			}
			delta, resets := windowDelta(prevW, w)
			if _, e = tx.ExecContext(ctx, `INSERT INTO codex_usage_rollup(bridge_id,hour_bucket,window_key,consumed_pp,samples,resets,level_last) VALUES($1,$2,$3,$4,1,$5,$6) ON CONFLICT (bridge_id,hour_bucket,window_key) DO UPDATE SET consumed_pp=codex_usage_rollup.consumed_pp+$4,samples=codex_usage_rollup.samples+1,resets=codex_usage_rollup.resets+$5,level_last=$6`, bridgeID, hour, windowKey(b, w), delta, resets, w.UsedPercent); e != nil {
				slog.Warn("usage rollup skipped", "error", e)
				return
			}
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO codex_usage_rollup_state(bridge_id,last_observed_at) VALUES($1,$2) ON CONFLICT (bridge_id) DO UPDATE SET last_observed_at=$2`, bridgeID, cur.ObservedAt); e != nil {
		slog.Warn("usage rollup state skipped", "error", e)
		return
	}
	if e = tx.Commit(); e != nil {
		slog.Warn("usage rollup skipped", "error", e)
	}
}

// promoteGlobalNews copies verified radar news from an administrator's bridge
// snapshot into the single global row, making the reset radar visible to every
// account. Best effort by design: a failed promotion never affects the quota
// ingest that already committed. Only newer checks replace the global row so a
// delayed retry cannot roll the radar back.
func (s *Service) promoteGlobalNews(ctx context.Context, ownerID string, news *codex.News) {
	var role string
	if e := s.DB.QueryRowContext(ctx, `SELECT role FROM users WHERE id=$1 AND account_status='active'`, ownerID).Scan(&role); e != nil || role != "admin" {
		return
	}
	if _, e := s.upsertGlobalNews(ctx, ownerID, news); e != nil {
		slog.Warn("global news promotion skipped", "error", e)
	}
}

// upsertGlobalNews stores the news only when its check is at least as recent
// as the stored one; reports whether the global row changed.
func (s *Service) upsertGlobalNews(ctx context.Context, ownerID string, news *codex.News) (bool, error) {
	var current []byte
	if e := s.DB.QueryRowContext(ctx, `SELECT payload FROM global_codex_news WHERE id=1`).Scan(&current); e == nil {
		var existing codex.News
		if json.Unmarshal(current, &existing) == nil && news.CheckedAt.Before(existing.CheckedAt) {
			return false, nil
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return false, e
	}
	payload, e := json.Marshal(news)
	if e != nil {
		return false, e
	}
	if _, e = s.DB.ExecContext(ctx, `INSERT INTO global_codex_news(id,payload,source_user_id,refreshed_at) VALUES(1,$1,$2,now()) ON CONFLICT (id) DO UPDATE SET payload=excluded.payload,source_user_id=excluded.source_user_id,refreshed_at=now()`, payload, ownerID); e != nil {
		return false, e
	}
	return true, nil
}

// RadarTokenCreate rotates the long-lived token that the reset-radar
// automation uses to push checked news straight to the server. The plaintext
// is returned once; older tokens are revoked on rotation.
func (s *Service) RadarTokenCreate(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, 403, "admin_only", "仅管理员可管理雷达令牌")
		return
	}
	var q struct {
		Note string `json:"note"`
	}
	httpx.Decode(r, &q)
	token := httpx.Token()
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(r.Context(), `UPDATE radar_tokens SET revoked_at=now() WHERE created_by=$1 AND revoked_at IS NULL`, auth.Who(r).UserID); e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO radar_tokens(id,token_hash,note,created_by,created_at) VALUES($1,$2,$3,$4,now())`, httpx.NewID("radar"), auth.Hash(token), strings.TrimSpace(q.Note), auth.Who(r).UserID); e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	if e = tx.Commit(); e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"token": token, "note": strings.TrimSpace(q.Note)})
}

// RadarTokenStatus reports the active token without leaking the plaintext.
func (s *Service) RadarTokenStatus(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, 403, "admin_only", "仅管理员可管理雷达令牌")
		return
	}
	var created time.Time
	var note string
	e := s.DB.QueryRowContext(r.Context(), `SELECT created_at,note FROM radar_tokens WHERE created_by=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, auth.Who(r).UserID).Scan(&created, &note)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.WriteJSON(w, 200, map[string]any{"empty": true})
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"created_at": created, "note": note})
}

// RadarTokenRevoke immediately disables the automation token.
func (s *Service) RadarTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, 403, "admin_only", "仅管理员可管理雷达令牌")
		return
	}
	res, e := s.DB.ExecContext(r.Context(), `UPDATE radar_tokens SET revoked_at=now() WHERE created_by=$1 AND revoked_at IS NULL`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	rows, _ := res.RowsAffected()
	httpx.WriteJSON(w, 200, map[string]bool{"revoked": rows > 0})
}

// RadarNews accepts checked radar news from the reset-radar automation
// (Bearer radar token) and promotes it for every account.
func (s *Service) RadarNews(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.Error(w, 401, "unauthenticated", "雷达令牌缺失")
		return
	}
	var owner string
	e := s.DB.QueryRowContext(r.Context(), `SELECT created_by FROM radar_tokens WHERE token_hash=$1 AND revoked_at IS NULL`, auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&owner)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 401, "radar_token_invalid", "雷达令牌无效或已吊销")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	var q codex.News
	if httpx.Decode(r, &q) != nil || !q.Validate(time.Now()) {
		httpx.Error(w, 400, "invalid_news", "雷达消息格式无效")
		return
	}
	applied, e := s.upsertGlobalNews(r.Context(), owner, &q)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "雷达消息保存失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "applied": applied, "checked_at": q.CheckedAt})
}
func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	plan, _, pe := getPlan(r.Context(), s.DB, auth.Who(r).UserID)
	if pe != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,name,revoked_at,received_at,snapshot FROM codex_bridges WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var revoked, received sql.NullTime
		var raw []byte
		if e = rows.Scan(&id, &name, &revoked, &received, &raw); e != nil {
			break
		}
		var snapshot any
		if len(raw) > 0 {
			var parsed codex.Snapshot
			if e = json.Unmarshal(raw, &parsed); e != nil {
				break
			}
			snapshot = parsed.ForPlan(plan)
		}
		var rv, rx any
		if revoked.Valid {
			rv = revoked.Time
		}
		if received.Valid {
			rx = received.Time
		}
		out = append(out, map[string]any{"id": id, "name": name, "revoked_at": rv, "received_at": rx, "snapshot": snapshot})
	}
	if e != nil || rows.Err() != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 200, map[string]any{"bridges": out})
}
