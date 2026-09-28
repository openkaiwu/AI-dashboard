package api

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strings"

	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/quota"
)

type quotaInput struct {
	ScopeKey             string   `json:"scope_key"`
	QuotaType            string   `json:"quota_type"`
	Unit                 string   `json:"unit"`
	LimitValue           *float64 `json:"limit_value"`
	RemainingValue       *float64 `json:"remaining_value"`
	RemainingRatio       *float64 `json:"remaining_ratio"`
	ResetPolicy          string   `json:"reset_policy"`
	ResetAt              string   `json:"reset_at"`
	ExpiresAt            string   `json:"expires_at"`
	RollingWindowSeconds *int     `json:"rolling_window_seconds"`
	Note                 string   `json:"note"`
}

type createAccountReq struct {
	ProviderID          string      `json:"provider_id"`
	DisplayName         string      `json:"display_name"`
	ExternalAccountHint string      `json:"external_account_hint"`
	Region              string      `json:"region"`
	PlanCode            string      `json:"plan_code"`
	PlanName            string      `json:"plan_name"`
	StartsAt            string      `json:"starts_at"`
	RenewsAt            string      `json:"renews_at"`
	ExpiresAt           string      `json:"expires_at"`
	Quota               *quotaInput `json:"quota"`
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, slug, display_name, category, homepage FROM providers ORDER BY display_name`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取平台失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, slug, name, cat, home string
		if err := rows.Scan(&id, &slug, &name, &cat, &home); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "读取平台失败")
			return
		}
		caps, _ := s.providerCaps(r, id)
		out = append(out, map[string]any{
			"id":           id,
			"slug":         slug,
			"display_name": name,
			"category":     cat,
			"homepage":     home,
			"capabilities": caps,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"providers": out})
}

func (s *Server) providerCaps(r *http.Request, providerID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT capability, support_level, acquisition_mode, connector_version FROM provider_capabilities WHERE provider_id = $1`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var cap, support, mode, ver string
		if err := rows.Scan(&cap, &support, &mode, &ver); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"capability":        cap,
			"support_level":     support,
			"acquisition_mode":  mode,
			"connector_version": ver,
		})
	}
	return out, rows.Err()
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	dash, err := s.loadDashboard(r.Context(), userID(r))
	if err != nil {
		logErr("list accounts", err, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取账户失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"accounts": dash["accounts"]})
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dash, err := s.loadDashboard(r.Context(), userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取账户失败")
		return
	}
	accounts, _ := dash["accounts"].([]map[string]any)
	for _, a := range accounts {
		if a["id"] == id {
			httpx.WriteJSON(w, http.StatusOK, a)
			return
		}
	}
	httpx.Error(w, http.StatusNotFound, "not_found", "账户不存在")
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.ProviderID == "" || req.DisplayName == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "平台和账户名称必填")
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM providers WHERE id = $1`, req.ProviderID).Scan(&exists); err != nil || exists == 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "未知平台")
		return
	}
	uid := userID(r)
	now := s.nowRFC()
	acctID := httpx.NewID("acct")
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "无法开始事务")
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_accounts (id, user_id, provider_id, display_name, external_account_hint, region, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $8)`, acctID, uid, req.ProviderID, req.DisplayName, req.ExternalAccountHint, req.Region, now, now)
	if err != nil {
		logErr("insert account", err, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "创建账户失败")
		return
	}
	entID := ""
	if strings.TrimSpace(req.PlanName) != "" || req.RenewsAt != "" || req.ExpiresAt != "" {
		entID = httpx.NewID("ent")
		starts, err1 := parseTimePtr(req.StartsAt)
		renews, err2 := parseTimePtr(req.RenewsAt)
		expires, err3 := parseTimePtr(req.ExpiresAt)
		if err1 != nil || err2 != nil || err3 != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "时间格式需要 RFC3339")
			return
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO entitlements (id, provider_account_id, plan_code, plan_name, starts_at, renews_at, expires_at, source_type, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'user_manual', $8)`,
			entID, acctID, req.PlanCode, nz(req.PlanName, "未命名套餐"), argTime(starts), argTime(renews), argTime(expires), now)
		if err != nil {
			logErr("insert entitlement", err, httpx.RequestID(r))
			httpx.Error(w, http.StatusInternalServerError, "internal", "创建套餐失败")
			return
		}
	}
	if req.Quota != nil {
		if err := insertBucketTx(ctx, tx, acctID, entID, *req.Quota, now); err != nil {
			if err == errBadQuota {
				httpx.Error(w, http.StatusBadRequest, "invalid_quota", "额度数值无效")
				return
			}
			if err == errBadTime {
				httpx.Error(w, http.StatusBadRequest, "invalid_input", "时间格式需要 RFC3339")
				return
			}
			logErr("insert bucket", err, httpx.RequestID(r))
			httpx.Error(w, http.StatusInternalServerError, "internal", "创建额度失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "创建账户失败")
		return
	}
	if err := s.evaluateUser(ctx, uid); err != nil {
		logErr("evaluate after create", err, httpx.RequestID(r))
	}
	s.getAccountByID(w, r, acctID)
}

func (s *Server) getAccountByID(w http.ResponseWriter, r *http.Request, id string) {
	dash, err := s.loadDashboard(r.Context(), userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取账户失败")
		return
	}
	accounts, _ := dash["accounts"].([]map[string]any)
	for _, a := range accounts {
		if a["id"] == id {
			httpx.WriteJSON(w, http.StatusCreated, a)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM provider_accounts WHERE id = $1 AND user_id = $2`, r.PathValue("id"), userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "删除失败")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "账户不存在")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) createBucket(w http.ResponseWriter, r *http.Request) {
	var req quotaInput
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	acctID := r.PathValue("id")
	var owner string
	err := s.db.QueryRowContext(r.Context(), `SELECT user_id FROM provider_accounts WHERE id = $1`, acctID).Scan(&owner)
	if err == sql.ErrNoRows || owner != userID(r) {
		httpx.Error(w, http.StatusNotFound, "not_found", "账户不存在")
		return
	}
	var entID sql.NullString
	_ = s.db.QueryRowContext(r.Context(), `SELECT id FROM entitlements WHERE provider_account_id = $1 ORDER BY created_at DESC LIMIT 1`, acctID).Scan(&entID)
	now := s.nowRFC()
	if err := insertBucketTx(r.Context(), s.db, acctID, nullStr(entID), req, now); err != nil {
		if err == errBadQuota {
			httpx.Error(w, http.StatusBadRequest, "invalid_quota", "额度数值无效")
			return
		}
		if err == errBadTime {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "时间格式需要 RFC3339")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal", "创建额度失败")
		return
	}
	_ = s.evaluateUser(r.Context(), userID(r))
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

var errBadTime = errors.New("bad time")
var errBadQuota = errors.New("bad quota")

func quotaNumber(value *float64, max float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= max)
}

func insertBucketTx(ctx context.Context, tx execer, acctID, entID string, q quotaInput, now string) error {
	if !quotaNumber(q.LimitValue, 1e15) || !quotaNumber(q.RemainingValue, 1e15) || !quotaNumber(q.RemainingRatio, 1) || len(q.Note) > 300 || (q.RollingWindowSeconds != nil && (*q.RollingWindowSeconds <= 0 || *q.RollingWindowSeconds > 31536000)) {
		return errBadQuota
	}
	resetAt, err := parseTimePtr(q.ResetAt)
	if err != nil {
		return errBadTime
	}
	expiresAt, err := parseTimePtr(q.ExpiresAt)
	if err != nil {
		return errBadTime
	}
	bucketID := httpx.NewID("bkt")
	qtype := nz(q.QuotaType, "unknown")
	unit := nz(q.Unit, qtype)
	policy := nz(q.ResetPolicy, "unknown")
	scope := nz(q.ScopeKey, "default")
	var ent any
	if entID != "" {
		ent = entID
	}
	var rolling any
	if q.RollingWindowSeconds != nil {
		rolling = *q.RollingWindowSeconds
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO quota_buckets (id, provider_account_id, entitlement_id, scope_key, quota_type, unit, limit_value, reset_policy, reset_at, expires_at, rolling_window_seconds, source_type, confidence, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'user_manual', 'high', $12, $13)`,
		bucketID, acctID, ent, scope, qtype, unit, argF64(q.LimitValue), policy, argTime(resetAt), argTime(expiresAt), rolling, now, now)
	if err != nil {
		return err
	}
	if q.RemainingValue == nil && q.RemainingRatio == nil {
		return nil
	}
	used := (*float64)(nil)
	if q.LimitValue != nil && q.RemainingValue != nil {
		v := *q.LimitValue - *q.RemainingValue
		if v < 0 {
			v = 0
		}
		used = &v
	}
	ratio := q.RemainingRatio
	if ratio == nil {
		ratio = quota.RemainingRatio(q.LimitValue, q.RemainingValue, nil)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_snapshots (id, quota_bucket_id, observed_at, used_value, remaining_value, remaining_ratio, note, raw_value_json, source_type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '{}', 'user_manual', $8)`,
		httpx.NewID("snap"), bucketID, now, argF64(used), argF64(q.RemainingValue), argF64(ratio), q.Note, now)
	return err
}

func nz(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func statusRank(s string) int {
	switch s {
	case quota.StatusLow:
		return 0
	case quota.StatusExpireSoonUnused:
		return 1
	case quota.StatusResetSoonUnused:
		return 2
	case quota.StatusStale:
		return 3
	case quota.StatusUnknown:
		return 4
	default:
		return 5
	}
}
