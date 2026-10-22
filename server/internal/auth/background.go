package auth

import (
	"aihub.dev/server/internal/httpx"
	"context"
	"net/http"
	"strings"
	"time"
)

// A separate, revocable credential avoids racing the interactive refresh token.
// It is accepted only by the explicitly registered read/receipt routes.
func (s *Service) BackgroundToken(w http.ResponseWriter, r *http.Request) {
	who := Who(r)
	if who.Kind != "mobile" {
		httpx.Error(w, 403, "mobile_required", "仅手机可启用后台同步")
		return
	}
	if r.Method == "DELETE" {
		if _, e := s.DB.ExecContext(r.Context(), `DELETE FROM background_tokens WHERE device_id=$1`, who.DeviceID); e != nil {
			httpx.Error(w, 503, "unavailable", "保存失败")
			return
		}
		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	token := httpx.Token()
	expiry := time.Now().Add(30 * 24 * time.Hour)
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO background_tokens(device_id,token_hash,expires_at) VALUES($1,$2,$3) ON CONFLICT(device_id) DO UPDATE SET token_hash=$2,expires_at=$3`, who.DeviceID, Hash(token), expiry); e != nil {
		httpx.Error(w, 503, "unavailable", "保存失败")
		return
	}
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO mobile_reminder_settings(user_id,device_id) VALUES($1,$2) ON CONFLICT(user_id) DO NOTHING`, who.UserID, who.DeviceID); e != nil {
		httpx.Error(w, 503, "unavailable", "主提醒手机保存失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 200, map[string]any{"token": token, "expires_at": expiry})
}

func (s *Service) Background(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, 401, "unauthenticated", "后台同步未授权")
			return
		}
		var who Identity
		e := s.DB.QueryRowContext(r.Context(), `SELECT d.user_id,d.id,u.role,d.kind FROM background_tokens b JOIN devices d ON d.id=b.device_id JOIN users u ON u.id=d.user_id WHERE b.token_hash=$1 AND b.expires_at>now() AND d.revoked_at IS NULL AND u.account_status='active' AND d.kind='mobile' AND EXISTS(SELECT 1 FROM sessions s WHERE s.device_id=d.id AND NOT s.consumed AND s.refresh_expires>now())`, Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&who.UserID, &who.DeviceID, &who.Role, &who.Kind)
		if e != nil {
			httpx.Error(w, 401, "background_expired", "请打开应用重新启用后台同步")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, who)))
	})
}
