package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/httpx"
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// SampleUpload records a fixed-page parser probe without changing real quotas.
func (s *Service) SampleUpload(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.Error(w, 401, "unauthenticated", "连接未授权")
		return
	}
	var q struct {
		ProviderSlug string    `json:"provider_slug"`
		SampleKind   string    `json:"sample_kind"`
		UsedPercent  float64   `json:"used_percent"`
		ObservedAt   time.Time `json:"observed_at"`
	}
	if httpx.Decode(r, &q) != nil || q.ProviderSlug != "cursor" || q.SampleKind != "fixed_page_poc" || math.IsNaN(q.UsedPercent) || math.IsInf(q.UsedPercent, 0) || q.UsedPercent < 0 || q.UsedPercent > 100 || q.ObservedAt.IsZero() || q.ObservedAt.After(time.Now().Add(2*time.Minute)) || q.ObservedAt.Before(time.Now().Add(-24*time.Hour)) {
		httpx.Error(w, 400, "invalid_sample", "只接受固定页面的脱敏数值样本")
		return
	}
	var uid, device string
	err := s.DB.QueryRowContext(r.Context(), `SELECT b.user_id,b.device_id FROM codex_bridges b WHERE b.token_hash=$1 AND b.revoked_at IS NULL`, auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&uid, &device)
	if err != nil {
		httpx.Error(w, 401, "bridge_revoked", "连接已撤销")
		return
	}
	if err := auth.EnsureActiveDesktopDevice(r.Context(), s.DB, device); err != nil {
		httpx.Error(w, 401, "bridge_revoked", "连接已撤销")
		return
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%.4f|%s", q.ProviderSlug, q.SampleKind, q.UsedPercent, q.ObservedAt.UTC().Format(time.RFC3339)))))
	result, err := s.DB.ExecContext(r.Context(), `INSERT INTO connector_samples(id,user_id,device_id,provider_slug,sample_kind,used_percent,content_hash,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (device_id,provider_slug,content_hash) DO NOTHING`, httpx.NewID("sample"), uid, device, q.ProviderSlug, q.SampleKind, q.UsedPercent, hash, q.ObservedAt)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "样本保存失败")
		return
	}
	n, _ := result.RowsAffected()
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "sample_only": true, "applied": n == 1})
}
