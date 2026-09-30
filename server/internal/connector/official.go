package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/official"
	"aihub.dev/server/internal/quota"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

const officialProviderIDPrefix = "prov_"

// UploadOfficial records an official-api quota snapshot (INH-399): the bridge
// fetches the provider's official API and posts the normalized reading here.
func (s *Service) UploadOfficial(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.Error(w, 401, "unauthenticated", "连接未授权")
		return
	}
	var snap official.Snapshot
	if httpx.Decode(r, &snap) != nil || snap.Validate(time.Now()) != nil {
		httpx.Error(w, 400, "invalid_snapshot", "采集数据格式无效")
		return
	}
	bridgeID, userID, err := s.officialBridgeIdentity(r, strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, 401, "bridge_revoked", "连接已撤销，请重新配对")
			return
		}
		httpx.Error(w, 503, "unavailable", "服务暂不可用")
		return
	}
	_ = bridgeID
	providerID, err := s.ensureOfficialProvider(r, strings.ToLower(strings.TrimSpace(snap.ProviderSlug)))
	if err != nil {
		httpx.Error(w, 503, "unavailable", "无法登记 Provider")
		return
	}
	acctID, err := s.ensureOfficialAccount(r, userID, providerID, bridgeID, snap.PlanName)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "无法关联官方 API 账户")
		return
	}
	bucketID, err := s.ensureOfficialBucket(r, acctID)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "无法关联额度桶")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ratio := quota.RemainingRatio(snap.LimitUSD, snap.RemainingUSD, nil)
	if _, err := s.DB.ExecContext(r.Context(), `INSERT INTO usage_snapshots (id, quota_bucket_id, observed_at, used_value, remaining_value, remaining_ratio, note, raw_value_json, source_type, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'official_api',$9)`,
		httpx.NewID("snap"), bucketID, snap.ObservedAt.UTC().Format(time.RFC3339), argF64(snap.UsedPercent), argF64(snap.RemainingUSD), argF64(ratio),
		snap.PlanName, "{}", now); err != nil {
		httpx.Error(w, 503, "unavailable", "写入快照失败")
		return
	}
	if _, err := s.DB.ExecContext(r.Context(), `UPDATE codex_bridges SET received_at=now() WHERE id=$1`, bridgeID); err != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	if s.AfterQuotaWrite != nil {
		_ = s.AfterQuotaWrite(r.Context(), userID)
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "applied": true, "provider_slug": strings.ToLower(strings.TrimSpace(snap.ProviderSlug))})
}

// officialBridgeIdentity resolves a bridge token to (bridgeID, userID, deviceID)
// and validates the bound desktop device is still active.
func (s *Service) officialBridgeIdentity(r *http.Request, token string) (string, string, error) {
	var bridgeID, userID, deviceID string
	e := s.DB.QueryRowContext(r.Context(), `SELECT b.id,b.user_id,b.device_id FROM codex_bridges b WHERE b.token_hash=$1 AND b.revoked_at IS NULL`,
		auth.Hash(token)).Scan(&bridgeID, &userID, &deviceID)
	if e != nil {
		return "", "", e
	}
	if e := auth.EnsureActiveDesktopDevice(r.Context(), s.DB, deviceID); e != nil {
		return "", "", sql.ErrNoRows
	}
	return bridgeID, userID, nil
}

func (s *Service) ensureOfficialProvider(r *http.Request, slug string) (string, error) {
	id := officialProviderIDPrefix + slug
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO providers(id,slug,display_name,category,homepage) VALUES($1,$2,$3,'ai','')
		ON CONFLICT DO NOTHING`, id, slug, slug); e != nil {
		return "", e
	}
	var resolved string
	if e := s.DB.QueryRowContext(r.Context(), `SELECT id FROM providers WHERE id=$1 OR slug=$2 LIMIT 1`, id, slug).Scan(&resolved); e != nil {
		return "", e
	}
	return resolved, nil
}

func (s *Service) ensureOfficialAccount(r *http.Request, userID, providerID, bridgeID, planName string) (string, error) {
	hint := "official:" + bridgeID
	var acctID string
	e := s.DB.QueryRowContext(r.Context(), `SELECT id FROM provider_accounts WHERE user_id=$1 AND provider_id=$2 AND external_account_hint=$3 LIMIT 1`,
		userID, providerID, hint).Scan(&acctID)
	if e == nil {
		return acctID, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	acctID = httpx.NewID("acct")
	now := time.Now().UTC().Format(time.RFC3339)
	plan := strings.TrimSpace(planName)
	if plan == "" {
		plan = "Official"
	}
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO provider_accounts (id,user_id,provider_id,display_name,external_account_hint,status,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,'active',$6,$7)`, acctID, userID, providerID, "官方 API", hint, now, now); e != nil {
		return "", e
	}
	entID := httpx.NewID("ent")
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO entitlements (id,provider_account_id,plan_code,plan_name,source_type,created_at)
		VALUES ($1,$2,$3,$4,'official_api',$5)`, entID, acctID, plan, plan, now); e != nil {
		return "", e
	}
	return acctID, nil
}

func (s *Service) ensureOfficialBucket(r *http.Request, acctID string) (string, error) {
	var bucketID string
	e := s.DB.QueryRowContext(r.Context(), `SELECT id FROM quota_buckets WHERE provider_account_id=$1 AND scope_key='default' LIMIT 1`, acctID).Scan(&bucketID)
	if e == nil {
		return bucketID, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	var entID string
	if e := s.DB.QueryRowContext(r.Context(), `SELECT id FROM entitlements WHERE provider_account_id=$1 ORDER BY created_at DESC LIMIT 1`, acctID).Scan(&entID); e != nil {
		return "", e
	}
	bucketID = httpx.NewID("bkt")
	now := time.Now().UTC().Format(time.RFC3339)
	_, e = s.DB.ExecContext(r.Context(), `INSERT INTO quota_buckets (id,provider_account_id,entitlement_id,scope_key,quota_type,unit,reset_policy,source_type,confidence,created_at,updated_at)
		VALUES ($1,$2,$3,'default','usage','percent','billing_cycle','official_api','high',$4,$4)`, bucketID, acctID, entID, now)
	return bucketID, e
}
