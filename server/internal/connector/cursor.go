package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/cursor"
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/quota"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const cursorProviderID = "prov_cursor"

func (s *Service) UploadCursor(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.Error(w, 401, "unauthenticated", "连接未授权")
		return
	}
	var snap cursor.Snapshot
	if httpx.Decode(r, &snap) != nil || snap.Validate(time.Now()) != nil {
		httpx.Error(w, 400, "invalid_snapshot", "采集数据格式无效")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	defer tx.Rollback()
	var bridgeID, userID string
	err = tx.QueryRowContext(r.Context(), `SELECT id,user_id FROM codex_bridges WHERE token_hash=$1 AND revoked_at IS NULL FOR UPDATE`, auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&bridgeID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Error(w, 401, "bridge_revoked", "连接已撤销，请重新配对")
		return
	}
	if err != nil {
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	if snap.Status == "unavailable" || snap.Status == "unknown" {
		_, err = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET received_at=now() WHERE id=$1`, bridgeID)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			httpx.Error(w, 503, "unavailable", "保存失败")
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"ok": true, "applied": false, "status": snap.Status})
		return
	}
	acctID, err := ensureCursorAccount(r.Context(), tx, userID, bridgeID, snap.PlanName)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "无法关联 Cursor 账户")
		return
	}
	now := time.Now().UTC()
	source := snap.Source
	if snap.Status == "stale" {
		source = "cursor_api2"
	}
	resetAt := argTime(snap.ResetAt)
	note := snap.CollectionStatus
	if snap.Status == "stale" && note == "" {
		note = "stale"
	}
	targets := cursorBucketTargets(snap)
	for _, target := range targets {
		bucketID, err := ensureCursorBucket(r.Context(), tx, acctID, target.ScopeKey)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "无法关联 Cursor 额度桶")
			return
		}
		limit := snap.LimitUSD
		remain := snap.RemainingUSD
		ratio := cursor.RemainingRatioFromUsedPercent(target.UsedPercent)
		if ratio == nil {
			ratio = quota.RemainingRatio(limit, remain, cursor.RemainingRatioFromUsedPercent(snap.UsedPercent))
		}
		var used *float64
		if target.UsedPercent != nil {
			v := *target.UsedPercent
			used = &v
		} else if limit != nil && remain != nil {
			v := *limit - *remain
			if v < 0 {
				v = 0
			}
			used = &v
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE quota_buckets SET limit_value=COALESCE($1,limit_value), reset_at=COALESCE($2,reset_at), reset_policy='billing_cycle', source_type=$3, confidence='high', updated_at=$4 WHERE id=$5`,
			argF64(limit), resetAt, source, now.Format(time.RFC3339), bucketID)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "更新额度失败")
			return
		}
		raw, _ := json.Marshal(map[string]any{
			"status":       snap.Status,
			"plan_name":    snap.PlanName,
			"scope_key":    target.ScopeKey,
			"label":        target.Label,
			"used_percent": target.UsedPercent,
		})
		_, err = tx.ExecContext(r.Context(), `INSERT INTO usage_snapshots (id, quota_bucket_id, observed_at, used_value, remaining_value, remaining_ratio, note, raw_value_json, source_type, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			httpx.NewID("snap"), bucketID, snap.ObservedAt.UTC().Format(time.RFC3339), argF64(used), argF64(remain), argF64(ratio), note, string(raw), source, now.Format(time.RFC3339))
		if err != nil {
			httpx.Error(w, 503, "unavailable", "写入快照失败")
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE provider_accounts SET updated_at=$1 WHERE id=$2`, now.Format(time.RFC3339), acctID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET received_at=now() WHERE id=$1`, bridgeID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	if snap.Status != "stale" && s.AfterQuotaWrite != nil {
		_ = s.AfterQuotaWrite(r.Context(), userID)
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "applied": true, "provider_account_id": acctID, "status": snap.Status, "buckets": len(targets)})
}

type cursorBucketTarget struct {
	ScopeKey    string
	Label       string
	UsedPercent *float64
}

func cursorBucketTargets(snap cursor.Snapshot) []cursorBucketTarget {
	if len(snap.Buckets) > 0 {
		out := make([]cursorBucketTarget, 0, len(snap.Buckets))
		for _, b := range snap.Buckets {
			out = append(out, cursorBucketTarget{ScopeKey: b.ScopeKey, Label: b.Label, UsedPercent: b.UsedPercent})
		}
		return out
	}
	return []cursorBucketTarget{{ScopeKey: "default", Label: "Total", UsedPercent: snap.UsedPercent}}
}

func ensureCursorAccount(ctx context.Context, tx *sql.Tx, userID, bridgeID, planName string) (acctID string, err error) {
	hint := "bridge:" + bridgeID
	err = tx.QueryRowContext(ctx, `SELECT id FROM provider_accounts
		WHERE user_id=$1 AND provider_id=$2 AND external_account_hint=$3 LIMIT 1`, userID, cursorProviderID, hint).Scan(&acctID)
	if err == nil {
		return acctID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	acctID = httpx.NewID("acct")
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_accounts (id,user_id,provider_id,display_name,external_account_hint,status,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,'active',$6,$7)`, acctID, userID, cursorProviderID, "Cursor 本机", hint, now, now)
	if err != nil {
		return "", err
	}
	entID := httpx.NewID("ent")
	plan := strings.TrimSpace(planName)
	if plan == "" {
		plan = "Cursor"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO entitlements (id,provider_account_id,plan_code,plan_name,source_type,created_at)
		VALUES ($1,$2,$3,$4,'official_api',$5)`, entID, acctID, plan, plan, now)
	return acctID, err
}

func ensureCursorBucket(ctx context.Context, tx *sql.Tx, acctID, scopeKey string) (bucketID string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id FROM quota_buckets WHERE provider_account_id=$1 AND scope_key=$2 LIMIT 1`, acctID, scopeKey).Scan(&bucketID)
	if err == nil {
		return bucketID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var entID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM entitlements WHERE provider_account_id=$1 ORDER BY created_at DESC LIMIT 1`, acctID).Scan(&entID)
	if err != nil {
		return "", err
	}
	bucketID = httpx.NewID("bkt")
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, `INSERT INTO quota_buckets (id,provider_account_id,entitlement_id,scope_key,quota_type,unit,reset_policy,source_type,confidence,created_at,updated_at)
		VALUES ($1,$2,$3,$4,'usage','percent','billing_cycle','cursor_api2','high',$5,$5)`, bucketID, acctID, entID, scopeKey, now)
	return bucketID, err
}

func argF64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func argTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
