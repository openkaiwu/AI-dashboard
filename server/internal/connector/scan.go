package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/config"
	"aihub.dev/server/internal/httpx"
	"net/http"
	"strings"
)

// ConfigScan accepts sanitized config-discovery manifests from the desktop
// bridge (bridge token auth). Entries carry secret KEY NAMES only; values are
// rejected at the bridge and never transported.
func (s *Service) ConfigScan(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		httpx.Error(w, 401, "unauthenticated", "连接未授权")
		return
	}
	var q struct {
		Entries []config.ScanEntry `json:"entries"`
	}
	if httpx.Decode(r, &q) != nil || len(q.Entries) > 100 {
		httpx.Error(w, 400, "invalid_input", "扫描清单无效")
		return
	}
	var uid, device string
	e := s.DB.QueryRowContext(r.Context(), `SELECT b.user_id,b.device_id FROM codex_bridges b WHERE b.token_hash=$1 AND b.revoked_at IS NULL`,
		auth.Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&uid, &device)
	if e != nil {
		httpx.Error(w, 401, "bridge_revoked", "连接已撤销")
		return
	}
	if e := auth.EnsureActiveDesktopDevice(r.Context(), s.DB, device); e != nil {
		httpx.Error(w, 401, "bridge_revoked", "连接已撤销")
		return
	}
	if e := config.StoreScan(r.Context(), s.DB, uid, device, q.Entries); e != nil {
		httpx.Error(w, 503, "unavailable", "扫描结果保存失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "recorded": len(q.Entries)})
}
