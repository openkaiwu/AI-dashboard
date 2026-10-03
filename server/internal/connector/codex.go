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
	var bridgeID, deviceID string
	var old []byte
	e = tx.QueryRowContext(r.Context(), `SELECT b.id,b.device_id,b.snapshot FROM codex_bridges b WHERE b.token_hash=$1 AND b.revoked_at IS NULL FOR UPDATE`, auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&bridgeID, &deviceID, &old)
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
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true, "applied": applied})
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
