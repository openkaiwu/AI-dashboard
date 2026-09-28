package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"aihub.dev/server/internal/connector"
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/notification"
	"aihub.dev/server/internal/quota"
)

func snapshotCollectionStatus(note, raw string) string {
	note = strings.TrimSpace(note)
	if note == "stale" || strings.Contains(strings.ToLower(note), "unavailable") {
		return "stale"
	}
	if raw == "" {
		return ""
	}
	var body struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(raw), &body) == nil {
		return strings.TrimSpace(body.Status)
	}
	return ""
}

func collectionBlocksQuotaAlert(status string) bool {
	switch status {
	case "stale", "unavailable", "unknown":
		return true
	default:
		return false
	}
}

func (s *Server) ownsProviderAccount(ctx context.Context, uid, accountID string) bool {
	var exists bool
	return s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_accounts WHERE id=$1 AND user_id=$2)`, accountID, uid).Scan(&exists) == nil && exists
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, rule_type, enabled, params_json, provider_account_id, created_at, updated_at
		FROM notification_rules WHERE user_id = $1 ORDER BY created_at`, userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取规则失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		item, err := scanRule(rows)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "读取规则失败")
			return
		}
		out = append(out, item)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"rules": out})
}

func scanRule(rows *sql.Rows) (map[string]any, error) {
	var id, name, typ, params, created, updated string
	var enabled int
	var acct sql.NullString
	if err := rows.Scan(&id, &name, &typ, &enabled, &params, &acct, &created, &updated); err != nil {
		return nil, err
	}
	var p any
	_ = json.Unmarshal([]byte(params), &p)
	return map[string]any{
		"id":                  id,
		"name":                name,
		"rule_type":           typ,
		"enabled":             enabled == 1,
		"params":              p,
		"provider_account_id": nullStr(acct),
		"created_at":          created,
		"updated_at":          updated,
	}, nil
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              string              `json:"name"`
		RuleType          string              `json:"rule_type"`
		Enabled           *bool               `json:"enabled"`
		Params            notification.Params `json:"params"`
		ProviderAccountID string              `json:"provider_account_id"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.RuleType) == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "规则名称和类型必填")
		return
	}
	if req.ProviderAccountID != "" && !s.ownsProviderAccount(r.Context(), userID(r), req.ProviderAccountID) {
		httpx.Error(w, http.StatusNotFound, "not_found", "账户不存在")
		return
	}
	raw, _ := json.Marshal(req.Params)
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	var acct any
	if req.ProviderAccountID != "" {
		acct = req.ProviderAccountID
	}
	id := httpx.NewID("rule")
	now := s.nowRFC()
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO notification_rules (id, user_id, name, rule_type, enabled, params_json, provider_account_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, id, userID(r), req.Name, req.RuleType, enabled, string(raw), acct, now, now)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "创建规则失败")
		return
	}
	_ = s.evaluateUser(r.Context(), userID(r))
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              *string              `json:"name"`
		Enabled           *bool                `json:"enabled"`
		Params            *notification.Params `json:"params"`
		ProviderAccountID *string              `json:"provider_account_id"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	id := r.PathValue("id")
	var curName, curType, curParams string
	var enabled int
	var curAcct sql.NullString
	err := s.db.QueryRowContext(r.Context(), `SELECT name, rule_type, enabled, params_json, provider_account_id FROM notification_rules WHERE id = $1 AND user_id = $2`, id, userID(r)).
		Scan(&curName, &curType, &enabled, &curParams, &curAcct)
	if err == sql.ErrNoRows {
		httpx.Error(w, http.StatusNotFound, "not_found", "规则不存在")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取规则失败")
		return
	}
	if req.Name != nil {
		curName = *req.Name
	}
	if req.Enabled != nil {
		enabled = 0
		if *req.Enabled {
			enabled = 1
		}
	}
	if req.Params != nil {
		raw, _ := json.Marshal(req.Params)
		curParams = string(raw)
	}
	if req.ProviderAccountID != nil {
		if *req.ProviderAccountID == "" {
			curAcct = sql.NullString{}
		} else {
			curAcct = sql.NullString{String: *req.ProviderAccountID, Valid: true}
		}
	}
	if curAcct.Valid && !s.ownsProviderAccount(r.Context(), userID(r), curAcct.String) {
		httpx.Error(w, http.StatusNotFound, "not_found", "账户不存在")
		return
	}
	var acct any
	if curAcct.Valid {
		acct = curAcct.String
	}
	_, err = s.db.ExecContext(r.Context(), `UPDATE notification_rules SET name = $1, enabled = $2, params_json = $3, provider_account_id = $4, updated_at = $5 WHERE id = $6`,
		curName, enabled, curParams, acct, s.nowRFC(), id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "更新规则失败")
		return
	}
	_ = s.evaluateUser(r.Context(), userID(r))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM notification_rules WHERE id = $1 AND user_id = $2`, r.PathValue("id"), userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "删除失败")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "规则不存在")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) previewRule(w http.ResponseWriter, r *http.Request) {
	rule, err := s.loadRule(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "规则不存在")
		return
	}
	s.writePreview(w, r, rule)
}

func (s *Server) previewRuleBody(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RuleType          string              `json:"rule_type"`
		Params            notification.Params `json:"params"`
		ProviderAccountID string              `json:"provider_account_id"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	var acct *string
	if req.ProviderAccountID != "" {
		if !s.ownsProviderAccount(r.Context(), userID(r), req.ProviderAccountID) {
			httpx.Error(w, http.StatusNotFound, "not_found", "账户不存在")
			return
		}
		acct = &req.ProviderAccountID
	}
	s.writePreview(w, r, notification.Rule{Type: req.RuleType, Params: req.Params, ProviderAccountID: acct, Enabled: true})
}

func (s *Server) writePreview(w http.ResponseWriter, r *http.Request, rule notification.Rule) {
	subs, err := s.listSubjects(r.Context(), userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取额度失败")
		return
	}
	now := s.clock.Now()
	matches := make([]map[string]any, 0)
	would := false
	for _, sub := range subs {
		if rule.ProviderAccountID != nil && *rule.ProviderAccountID != sub.AccountID {
			continue
		}
		if skipSubject(rule, sub) {
			continue
		}
		res := notification.Evaluate(rule, sub, now)
		if res.WouldFire {
			would = true
		}
		matches = append(matches, map[string]any{
			"provider":             sub.ProviderName,
			"account":              sub.AccountName,
			"would_fire":           res.WouldFire,
			"reason":               res.Reason,
			"estimated_trigger_at": timeOut(res.EstimatedTriggerAt),
			"dedupe_key":           res.DedupeKey,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"would_fire":   would,
		"matches":      matches,
		"evaluated_at": now.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT n.id, n.rule_id, n.provider_account_id, n.quota_bucket_id, n.title, n.body, n.severity, n.dedupe_key,
		CASE WHEN n.status='snoozed' AND n.snoozed_until<=now() THEN 'unread' ELSE n.status END, n.created_at,
		COALESCE(p.slug, ''), COALESCE(p.display_name, '')
		FROM notifications n
		LEFT JOIN provider_accounts a ON a.id = n.provider_account_id
		LEFT JOIN providers p ON p.id = a.provider_id
		WHERE n.user_id = $1 ORDER BY n.created_at DESC LIMIT 100`, userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取通知失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, title, body, sev, key, status, created, providerSlug, providerName string
		var ruleID, acct, bucket sql.NullString
		if err := rows.Scan(&id, &ruleID, &acct, &bucket, &title, &body, &sev, &key, &status, &created, &providerSlug, &providerName); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "读取通知失败")
			return
		}
		out = append(out, map[string]any{
			"id":                  id,
			"rule_id":             nullStr(ruleID),
			"provider_account_id": nullStr(acct),
			"quota_bucket_id":     nullStr(bucket),
			"provider_slug":       providerSlug,
			"provider_name":       providerName,
			"title":               title,
			"body":                body,
			"severity":            sev,
			"dedupe_key":          key,
			"status":              status,
			"created_at":          created,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"notifications": out})
}

func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) {
	_, err := s.db.ExecContext(r.Context(), `UPDATE notifications SET status = 'read' WHERE id = $1 AND user_id = $2`, r.PathValue("id"), userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "更新失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) notificationAction(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Action string `json:"action"`
	}
	if httpx.Decode(r, &q) != nil || (q.Action != "read" && q.Action != "snooze" && q.Action != "dismiss") {
		httpx.Error(w, 400, "invalid_action", "请选择已读、稍后提醒或忽略")
		return
	}
	var result sql.Result
	var err error
	switch q.Action {
	case "snooze":
		result, err = s.db.ExecContext(r.Context(), `UPDATE notifications SET status='snoozed',snoozed_until=now()+interval '1 hour' WHERE id=$1 AND user_id=$2 AND status IN ('unread','snoozed')`, r.PathValue("id"), userID(r))
	case "dismiss":
		result, err = s.db.ExecContext(r.Context(), `UPDATE notifications SET status='dismissed',snoozed_until=NULL WHERE id=$1 AND user_id=$2`, r.PathValue("id"), userID(r))
	default:
		result, err = s.db.ExecContext(r.Context(), `UPDATE notifications SET status='read',snoozed_until=NULL WHERE id=$1 AND user_id=$2`, r.PathValue("id"), userID(r))
	}
	if err != nil {
		httpx.Error(w, 503, "unavailable", "更新失败")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.Error(w, 404, "not_found", "提醒不存在或已处理")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "action": q.Action})
}

func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	_, err := s.db.ExecContext(r.Context(), `UPDATE notifications SET status = 'read' WHERE user_id = $1`, userID(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "更新失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) loadRule(ctx context.Context, uid, id string) (notification.Rule, error) {
	var name, typ, params string
	var enabled int
	var acct sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT name, rule_type, enabled, params_json, provider_account_id FROM notification_rules WHERE id = $1 AND user_id = $2`, id, uid).
		Scan(&name, &typ, &enabled, &params, &acct)
	if err != nil {
		return notification.Rule{}, err
	}
	p, _ := notification.ParseParams(params)
	rule := notification.Rule{ID: id, Name: name, Type: typ, Enabled: enabled == 1, Params: p}
	if acct.Valid {
		v := acct.String
		rule.ProviderAccountID = &v
	}
	return rule, nil
}

func skipSubject(rule notification.Rule, sub notification.Subject) bool {
	switch rule.Type {
	case notification.TypeLowQuota, notification.TypeResetSoonUnused, notification.TypeExpireSoonUnused:
		if sub.BucketID == "" {
			return true
		}
		if collectionBlocksQuotaAlert(sub.CollectionStatus) {
			return true
		}
		ratio := quota.RemainingRatio(sub.LimitValue, sub.RemainingValue, sub.RemainingRatio)
		return ratio == nil && sub.RemainingValue == nil
	case notification.TypeStale:
		return sub.BucketID == ""
	case notification.TypeRenewalSoon:
		return sub.EntitlementID == ""
	default:
		return true
	}
}

func (s *Server) listSubjects(ctx context.Context, uid string) ([]notification.Subject, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.display_name, p.display_name,
		       COALESCE(b.id, ''), COALESCE(b.scope_key, ''),
		       COALESCE(e.id, ''), COALESCE(e.plan_name, ''),
		       b.limit_value, s.remaining_value, s.remaining_ratio,
		       b.reset_at, b.expires_at, e.renews_at, s.observed_at,
		       COALESCE(s.note, ''), COALESCE(s.raw_value_json, '{}')
		FROM provider_accounts a
		JOIN providers p ON p.id = a.provider_id
		LEFT JOIN entitlements e ON e.id = (
			SELECT id FROM entitlements WHERE provider_account_id = a.id ORDER BY created_at DESC LIMIT 1
		)
		LEFT JOIN quota_buckets b ON b.provider_account_id = a.id
		LEFT JOIN usage_snapshots s ON s.id = (
			SELECT id FROM usage_snapshots WHERE quota_bucket_id = b.id ORDER BY observed_at DESC LIMIT 1
		)
		WHERE a.user_id = $1`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]notification.Subject, 0)
	for rows.Next() {
		var sub notification.Subject
		var limit, remain, ratio sql.NullFloat64
		var reset, exp, renew, obs, note, raw sql.NullString
		if err := rows.Scan(&sub.AccountID, &sub.AccountName, &sub.ProviderName, &sub.BucketID, &sub.ScopeKey,
			&sub.EntitlementID, &sub.PlanName, &limit, &remain, &ratio, &reset, &exp, &renew, &obs, &note, &raw); err != nil {
			return nil, err
		}
		sub.LimitValue = nullF64(limit)
		sub.RemainingValue = nullF64(remain)
		sub.RemainingRatio = nullF64(ratio)
		sub.ResetAt = nullTime(reset)
		sub.ExpiresAt = nullTime(exp)
		sub.RenewsAt = nullTime(renew)
		sub.ObservedAt = nullTime(obs)
		sub.CollectionStatus = snapshotCollectionStatus(nullStr(note), nullStr(raw))
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Server) evaluateUser(ctx context.Context, uid string) error {
	rulesRows, err := s.db.QueryContext(ctx, `SELECT id, name, rule_type, enabled, params_json, provider_account_id FROM notification_rules WHERE user_id = $1`, uid)
	if err != nil {
		return err
	}
	defer rulesRows.Close()
	var rules []notification.Rule
	for rulesRows.Next() {
		var id, name, typ, params string
		var enabled int
		var acct sql.NullString
		if err := rulesRows.Scan(&id, &name, &typ, &enabled, &params, &acct); err != nil {
			return err
		}
		p, _ := notification.ParseParams(params)
		rule := notification.Rule{ID: id, Name: name, Type: typ, Enabled: enabled == 1, Params: p}
		if acct.Valid {
			v := acct.String
			rule.ProviderAccountID = &v
		}
		rules = append(rules, rule)
	}
	if err := rulesRows.Err(); err != nil {
		return err
	}

	subs, err := s.listSubjects(ctx, uid)
	if err != nil {
		return err
	}
	active := map[string]struct{}{}
	aRows, err := s.db.QueryContext(ctx, `SELECT dedupe_key FROM alert_states WHERE user_id = $1`, uid)
	if err != nil {
		return err
	}
	for aRows.Next() {
		var k string
		if err := aRows.Scan(&k); err != nil {
			_ = aRows.Close()
			return err
		}
		active[k] = struct{}{}
	}
	_ = aRows.Close()

	now := s.clock.Now()
	wanted := map[string]struct{}{}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, sub := range subs {
			if rule.ProviderAccountID != nil && *rule.ProviderAccountID != sub.AccountID {
				continue
			}
			if skipSubject(rule, sub) {
				continue
			}
			res := notification.Evaluate(rule, sub, now)
			if !res.WouldFire {
				continue
			}
			wanted[res.DedupeKey] = struct{}{}
			if _, ok := active[res.DedupeKey]; ok {
				continue
			}
			nid := httpx.NewID("ntf")
			ts := now.UTC().Format("2006-01-02T15:04:05Z07:00")
			var bucket any
			if res.BucketID != "" {
				bucket = res.BucketID
			}
			var acct any
			if res.AccountID != "" {
				acct = res.AccountID
			}
			_, err = s.db.ExecContext(ctx, `INSERT INTO notifications (id, user_id, rule_id, provider_account_id, quota_bucket_id, title, body, severity, dedupe_key, status, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'unread', $10)`, nid, uid, rule.ID, acct, bucket, res.Title, res.Body, res.Severity, res.DedupeKey, ts)
			if err != nil {
				return err
			}
			_, err = s.db.ExecContext(ctx, `INSERT INTO alert_states (user_id, dedupe_key, notification_id, created_at) VALUES ($1, $2, $3, $4)`, uid, res.DedupeKey, nid, ts)
			if err != nil {
				return err
			}
			active[res.DedupeKey] = struct{}{}
		}
	}
	for key := range active {
		if _, ok := wanted[key]; ok {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM alert_states WHERE user_id = $1 AND dedupe_key = $2`, uid, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) forUsers(ctx context.Context, evaluate func(string) error) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM users`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if err := evaluate(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) EvaluateCodexAll(ctx context.Context) error {
	return s.forUsers(ctx, func(id string) error {
		_, err := (&connector.Service{DB: s.db}).Evaluate(ctx, id, s.clock.Now())
		return err
	})
}

func (s *Server) EvaluateNotificationsAll(ctx context.Context) error {
	return s.forUsers(ctx, func(id string) error { return s.evaluateUser(ctx, id) })
}
