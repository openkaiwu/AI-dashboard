package api

import (
	"aihub.dev/server/internal/httpx"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

func (s *Server) patchAccount(w http.ResponseWriter, r *http.Request) {
	var q struct {
		DisplayName string `json:"display_name"`
		Region      string `json:"region"`
		PlanName    string `json:"plan_name"`
		RenewsAt    string `json:"renews_at"`
		ExpiresAt   string `json:"expires_at"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.DisplayName = strings.TrimSpace(q.DisplayName)
	q.PlanName = strings.TrimSpace(q.PlanName)
	q.Region = strings.TrimSpace(q.Region)
	if q.DisplayName == "" || len(q.DisplayName) > 120 || len(q.PlanName) > 120 || len(q.Region) > 40 {
		httpx.Error(w, 400, "invalid_input", "账户字段无效")
		return
	}
	renew, err1 := parseTimePtr(q.RenewsAt)
	expiry, err2 := parseTimePtr(q.ExpiresAt)
	if err1 != nil || err2 != nil {
		httpx.Error(w, 400, "invalid_input", "时间格式需要 RFC3339")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(r.Context(), `SELECT id FROM provider_accounts WHERE id=$1 AND user_id=$2 FOR UPDATE`, r.PathValue("id"), userID(r)).Scan(&id); err == sql.ErrNoRows {
		httpx.Error(w, 404, "not_found", "账户不存在")
		return
	} else if err != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(r.Context(), `UPDATE provider_accounts SET display_name=$1,region=$2,updated_at=$3 WHERE id=$4`, q.DisplayName, q.Region, now, id)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	if q.PlanName != "" || renew != nil || expiry != nil {
		var entID, source string
		err = tx.QueryRowContext(r.Context(), `SELECT id,source_type FROM entitlements WHERE provider_account_id=$1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, id).Scan(&entID, &source)
		if err == sql.ErrNoRows {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO entitlements(id,provider_account_id,plan_name,renews_at,expires_at,source_type,created_at) VALUES($1,$2,$3,$4,$5,'user_manual',$6)`, httpx.NewID("ent"), id, nz(q.PlanName, "未命名套餐"), argTime(renew), argTime(expiry), now)
		} else if err == nil && source != "user_manual" {
			httpx.Error(w, 409, "automatic_entitlement", "自动采集的套餐不能手工覆盖")
			return
		} else if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE entitlements SET plan_name=CASE WHEN $1='' THEN plan_name ELSE $1 END,renews_at=COALESCE($2,renews_at),expires_at=COALESCE($3,expires_at),source_type='user_manual' WHERE id=$4`, q.PlanName, argTime(renew), argTime(expiry), entID)
		}
		if err != nil {
			httpx.Error(w, 503, "unavailable", "保存失败")
			return
		}
	}
	if tx.Commit() != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	_ = s.evaluateUser(r.Context(), userID(r))
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}
