package api

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"time"

	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/quota"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	out, err := s.loadDashboard(r.Context(), userID(r))
	if err != nil {
		logErr("dashboard", err, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取看板失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) loadDashboard(ctx context.Context, uid string) (map[string]any, error) {
	now := s.clock.Now()
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.display_name, a.external_account_hint, a.region, a.status, a.updated_at,
		       p.id, p.slug, p.display_name,
		       e.id, e.plan_name, e.plan_code, e.renews_at, e.expires_at,
		       b.id, b.scope_key, b.quota_type, b.unit, b.limit_value, b.reset_policy, b.reset_at, b.expires_at, b.source_type, b.confidence,
		       s.remaining_value, s.remaining_ratio, s.used_value, s.observed_at, s.note, s.source_type
		FROM provider_accounts a
		JOIN providers p ON p.id = a.provider_id
		LEFT JOIN entitlements e ON e.id = (
			SELECT id FROM entitlements WHERE provider_account_id = a.id ORDER BY created_at DESC LIMIT 1
		)
		LEFT JOIN quota_buckets b ON b.provider_account_id = a.id
		LEFT JOIN usage_snapshots s ON s.id = (
			SELECT id FROM usage_snapshots WHERE quota_bucket_id = b.id ORDER BY observed_at DESC LIMIT 1
		)
		WHERE a.user_id = ?
		ORDER BY a.created_at DESC, b.created_at ASC
	`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type acc struct {
		id     string
		data   map[string]any
		status string
	}
	order := make([]string, 0)
	byID := map[string]*acc{}
	counts := map[string]int{
		quota.StatusHealthy:          0,
		quota.StatusLow:              0,
		quota.StatusResetSoonUnused:  0,
		quota.StatusExpireSoonUnused: 0,
		quota.StatusStale:            0,
		quota.StatusUnknown:          0,
	}

	for rows.Next() {
		var (
			aid, aname, hint, region, astatus, aupd string
			pid, pslug, pname                       string
			eid, planName, planCode                 sql.NullString
			renews, eexp                            sql.NullString
			bid, scope, qtype, unit, policy         sql.NullString
			breset, bexp, bsrc, conf                sql.NullString
			limit                                   sql.NullFloat64
			remain, ratio, used                     sql.NullFloat64
			obs, note, ssrc                         sql.NullString
		)
		if err := rows.Scan(
			&aid, &aname, &hint, &region, &astatus, &aupd,
			&pid, &pslug, &pname,
			&eid, &planName, &planCode, &renews, &eexp,
			&bid, &scope, &qtype, &unit, &limit, &policy, &breset, &bexp, &bsrc, &conf,
			&remain, &ratio, &used, &obs, &note, &ssrc,
		); err != nil {
			return nil, err
		}
		item, ok := byID[aid]
		if !ok {
			item = &acc{
				id:     aid,
				status: quota.StatusUnknown,
				data: map[string]any{
					"id":                    aid,
					"display_name":          aname,
					"external_account_hint": hint,
					"region":                region,
					"status":                astatus,
					"updated_at":            aupd,
					"provider": map[string]any{
						"id": pid, "slug": pslug, "display_name": pname,
					},
					"plan_name":  nullStr(planName),
					"plan_code":  nullStr(planCode),
					"renews_at":  timeOut(nullTime(renews)),
					"expires_at": timeOut(nullTime(eexp)),
					"buckets":    []map[string]any{},
				},
			}
			byID[aid] = item
			order = append(order, aid)
		}
		if !bid.Valid {
			continue
		}
		view := quota.BucketView{
			LimitValue:     nullF64(limit),
			RemainingValue: nullF64(remain),
			RemainingRatio: nullF64(ratio),
			ResetAt:        nullTime(breset),
			ExpiresAt:      nullTime(bexp),
			ObservedAt:     nullTime(obs),
		}
		st := quota.ComputeStatus(view, now)
		if statusRank(st) < statusRank(item.status) || len(item.data["buckets"].([]map[string]any)) == 0 {
			item.status = st
		}
		src := nullStr(ssrc)
		if src == "" {
			src = nullStr(bsrc)
		}
		bucket := map[string]any{
			"id":               bid.String,
			"scope_key":        nullStr(scope),
			"quota_type":       nullStr(qtype),
			"unit":             nullStr(unit),
			"limit_value":      f64Out(nullF64(limit)),
			"remaining_value":  f64Out(nullF64(remain)),
			"remaining_ratio":  f64Out(nullF64(ratio)),
			"used_value":       f64Out(nullF64(used)),
			"reset_policy":     nullStr(policy),
			"reset_at":         timeOut(nullTime(breset)),
			"expires_at":       timeOut(nullTime(bexp)),
			"source_type":      src,
			"confidence":       nullStr(conf),
			"observed_at":      timeOut(nullTime(obs)),
			"note":             nullStr(note),
			"status":           st,
		}
		item.data["buckets"] = append(item.data["buckets"].([]map[string]any), bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	accounts := make([]map[string]any, 0, len(order))
	for _, id := range order {
		item := byID[id]
		if len(item.data["buckets"].([]map[string]any)) == 0 {
			item.status = quota.StatusUnknown
		}
		item.data["computed_status"] = item.status
		counts[item.status]++
		accounts = append(accounts, item.data)
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		si, _ := accounts[i]["computed_status"].(string)
		sj, _ := accounts[j]["computed_status"].(string)
		if statusRank(si) != statusRank(sj) {
			return statusRank(si) < statusRank(sj)
		}
		return false
	})
	return map[string]any{
		"generated_at": now.UTC().Format(time.RFC3339),
		"counts":       counts,
		"accounts":     accounts,
	}, nil
}

func (s *Server) manualSnapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RemainingValue *float64 `json:"remaining_value"`
		RemainingRatio *float64 `json:"remaining_ratio"`
		UsedValue      *float64 `json:"used_value"`
		LimitValue     *float64 `json:"limit_value"`
		ResetAt        string   `json:"reset_at"`
		ExpiresAt      string   `json:"expires_at"`
		Note           string   `json:"note"`
		ObservedAt     string   `json:"observed_at"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	bucketID := r.PathValue("id")
	uid := userID(r)
	var owner string
	err := s.db.QueryRowContext(r.Context(), `
		SELECT a.user_id FROM quota_buckets b
		JOIN provider_accounts a ON a.id = b.provider_account_id
		WHERE b.id = ?`, bucketID).Scan(&owner)
	if err == sql.ErrNoRows || owner != uid {
		httpx.Error(w, http.StatusNotFound, "not_found", "额度不存在")
		return
	}
	resetAt, err1 := parseTimePtr(req.ResetAt)
	expiresAt, err2 := parseTimePtr(req.ExpiresAt)
	obs, err3 := parseTimePtr(req.ObservedAt)
	if err1 != nil || err2 != nil || err3 != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "时间格式需要 RFC3339")
		return
	}
	now := s.clock.Now().UTC()
	if obs == nil {
		obs = &now
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "无法开始事务")
		return
	}
	defer func() { _ = tx.Rollback() }()
	if req.LimitValue != nil || resetAt != nil || expiresAt != nil {
		_, err = tx.ExecContext(ctx, `UPDATE quota_buckets SET
			limit_value = COALESCE(?, limit_value),
			reset_at = COALESCE(?, reset_at),
			expires_at = COALESCE(?, expires_at),
			source_type = 'user_manual',
			updated_at = ?
			WHERE id = ?`, argF64(req.LimitValue), argTime(resetAt), argTime(expiresAt), now.Format(time.RFC3339), bucketID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "更新额度失败")
			return
		}
	}
	var limit sql.NullFloat64
	_ = tx.QueryRowContext(ctx, `SELECT limit_value FROM quota_buckets WHERE id = ?`, bucketID).Scan(&limit)
	ratio := req.RemainingRatio
	if ratio == nil {
		ratio = quota.RemainingRatio(nullF64(limit), req.RemainingValue, nil)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_snapshots (id, quota_bucket_id, observed_at, used_value, remaining_value, remaining_ratio, note, raw_value_json, source_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '{}', 'user_manual', ?)`,
		httpx.NewID("snap"), bucketID, obs.Format(time.RFC3339), argF64(req.UsedValue), argF64(req.RemainingValue), argF64(ratio), req.Note, now.Format(time.RFC3339))
	if err != nil {
		logErr("insert snapshot", err, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "写入快照失败")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "写入快照失败")
		return
	}
	if err := s.evaluateUser(ctx, uid); err != nil {
		logErr("evaluate after snapshot", err, httpx.RequestID(r))
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "observed_at": obs.Format(time.RFC3339)})
}

func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	bucketID := r.PathValue("id")
	var owner string
	err := s.db.QueryRowContext(r.Context(), `
		SELECT a.user_id FROM quota_buckets b
		JOIN provider_accounts a ON a.id = b.provider_account_id
		WHERE b.id = ?`, bucketID).Scan(&owner)
	if err == sql.ErrNoRows || owner != userID(r) {
		httpx.Error(w, http.StatusNotFound, "not_found", "额度不存在")
		return
	}
	since := s.clock.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, observed_at, used_value, remaining_value, remaining_ratio, note, source_type
		FROM usage_snapshots WHERE quota_bucket_id = ? AND observed_at >= ? ORDER BY observed_at ASC`, bucketID, since)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取历史失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, obs, note, src string
		var used, remain, ratio sql.NullFloat64
		if err := rows.Scan(&id, &obs, &used, &remain, &ratio, &note, &src); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "读取历史失败")
			return
		}
		out = append(out, map[string]any{
			"id":              id,
			"observed_at":     obs,
			"used_value":      f64Out(nullF64(used)),
			"remaining_value": f64Out(nullF64(remain)),
			"remaining_ratio": f64Out(nullF64(ratio)),
			"note":            note,
			"source_type":     src,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"days": days, "snapshots": out})
}
