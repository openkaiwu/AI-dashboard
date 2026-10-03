// Package auth owns user, device and refresh-session persistence.
package auth

import (
	"aihub.dev/server/internal/audit"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type identityKey struct{}
type Identity struct {
	UserID   string
	DeviceID string
	Role     string
	Kind     string
}

func Who(r *http.Request) Identity { v, _ := r.Context().Value(identityKey{}).(Identity); return v }
func Hash(s string) string         { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

type Service struct {
	DB         *sql.DB
	OnRegister func(context.Context, *sql.Tx, string, string) error
}
type credentials struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	DeviceName     string `json:"device_name"`
	DeviceKind     string `json:"device_kind"`
	InstallationID string `json:"installation_id"`
}
type Session struct {
	Token        string            `json:"token"`
	RefreshToken string            `json:"refresh_token"`
	ExpiresAt    time.Time         `json:"expires_at"`
	DeviceID     string            `json:"device_id"`
	User         map[string]string `json:"user"`
}

func session(ctx context.Context, tx *sql.Tx, user, device, email string) (Session, error) {
	now := time.Now().UTC()
	out := Session{Token: httpx.Token(), RefreshToken: httpx.Token(), ExpiresAt: now.Add(15 * time.Minute), DeviceID: device, User: map[string]string{"id": user, "email": email}}
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions(access_hash,refresh_hash,user_id,device_id,access_expires,refresh_expires) VALUES($1,$2,$3,$4,$5,$6)`, Hash(out.Token), Hash(out.RefreshToken), user, device, out.ExpiresAt, now.Add(30*24*time.Hour))
	return out, err
}
func (s *Service) Login(w http.ResponseWriter, r *http.Request) { s.signIn(w, r) }
func (s *Service) signIn(w http.ResponseWriter, r *http.Request) {
	var q credentials
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.Email = strings.ToLower(strings.TrimSpace(q.Email))
	q.DeviceName = strings.TrimSpace(q.DeviceName)
	if q.DeviceName == "" {
		q.DeviceName = "新设备"
	}
	addr, err := mail.ParseAddress(q.Email)
	if err != nil || addr.Address != q.Email || len(q.Email) > 254 || len(q.Password) < 8 || len(q.Password) > 72 || len(q.DeviceName) > 120 || len(q.InstallationID) < 16 || len(q.InstallationID) > 128 || (q.DeviceKind != "desktop" && q.DeviceKind != "mobile" && q.DeviceKind != "web_admin") {
		httpx.Error(w, 400, "invalid_input", "检查邮箱、密码（8–72 字节）及设备名称")
		return
	}
	var uid, hash, role, status string
	e := s.DB.QueryRowContext(r.Context(), `SELECT id,password_hash,role,account_status FROM users WHERE email=$1`, q.Email).Scan(&uid, &hash, &role, &status)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 503, "auth_unavailable", "认证服务暂不可用，请稍后重试")
		return
	}
	if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(q.Password)) != nil {
		httpx.Error(w, 401, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	if status != "active" || (q.DeviceKind == "web_admin" && role != "admin") {
		if status == "pending" {
			httpx.Error(w, 403, "account_pending", "账户正在等待管理员审核")
		} else {
			httpx.Error(w, 403, "account_not_authorized", "账户尚未获授权或已停用")
		}
		return
	}
	var out Session
	occupied := errors.New("device slot occupied")
	unauthorized := errors.New("account not active")
	pendingReview := errors.New("account pending review")
	err = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		var currentStatus string
		if e := tx.QueryRowContext(r.Context(), `SELECT account_status FROM users WHERE id=$1 FOR UPDATE`, uid).Scan(&currentStatus); e != nil {
			return e
		}
		if currentStatus != "active" {
			if currentStatus == "pending" {
				return pendingReview
			}
			return unauthorized
		}
		var did, existingHash string
		installHash := Hash(q.InstallationID)
		if q.DeviceKind != "web_admin" {
			e := tx.QueryRowContext(r.Context(), `SELECT id,installation_hash FROM devices WHERE user_id=$1 AND kind=$2 AND revoked_at IS NULL FOR UPDATE`, uid, q.DeviceKind).Scan(&did, &existingHash)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if e == nil && existingHash != installHash {
				return occupied
			}
		}
		if did == "" {
			did = httpx.NewID("dev")
			if _, e := tx.ExecContext(r.Context(), `INSERT INTO devices(id,user_id,name,kind,installation_hash) VALUES($1,$2,$3,$4,$5)`, did, uid, q.DeviceName, q.DeviceKind, installHash); e != nil {
				return e
			}
		} else {
			if _, e := tx.ExecContext(r.Context(), `UPDATE devices SET name=$1 WHERE id=$2`, q.DeviceName, did); e != nil {
				return e
			}
		}
		var e error
		out, e = session(r.Context(), tx, uid, did, q.Email)
		if e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, uid, did, "login")
	})
	if err != nil {
		if errors.Is(err, occupied) {
			httpx.Error(w, 409, "device_slot_occupied", "此类设备已绑定，请联系管理员解绑")
		} else if errors.Is(err, pendingReview) {
			httpx.Error(w, 403, "account_pending", "账户正在等待管理员审核")
		} else if errors.Is(err, unauthorized) {
			httpx.Error(w, 403, "account_not_authorized", "账户尚未获授权或已停用")
		} else {
			httpx.Error(w, 500, "internal", "登录失败")
		}
		return
	}
	out.User["role"] = role
	httpx.WriteJSON(w, 200, out)
}
func (s *Service) Middleware(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, 401, "unauthenticated", "需要登录")
			return
		}
		var who Identity
		err := s.DB.QueryRowContext(r.Context(), `SELECT s.user_id,s.device_id,u.role,d.kind FROM sessions s JOIN devices d ON d.id=s.device_id JOIN users u ON u.id=s.user_id WHERE s.access_hash=$1 AND NOT s.consumed AND s.access_expires>now() AND d.revoked_at IS NULL AND u.account_status='active'`, Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&who.UserID, &who.DeviceID, &who.Role, &who.Kind)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.Error(w, 401, "unauthenticated", "登录过期或设备已撤销")
			} else {
				httpx.Error(w, 503, "auth_unavailable", "认证服务暂不可用，请稍后重试")
			}
			return
		}
		if who.Kind == "web_admin" && !strings.HasPrefix(r.URL.Path, "/api/v1/admin/") && r.URL.Path != "/api/v1/auth/logout" && r.URL.Path != "/api/v1/me" {
			httpx.Error(w, 403, "admin_console_only", "管理浏览器只能访问管理台")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, who)))
	})
}
func (s *Service) Refresh(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Refresh string `json:"refresh_token"`
	}
	if httpx.Decode(r, &q) != nil || q.Refresh == "" {
		httpx.Error(w, 400, "invalid_json", "缺少刷新凭据")
		return
	}
	var out Session
	invalid := false
	err := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		var uid, did, email, status, role string
		var consumed, revoked bool
		var expiry time.Time
		// Lock device before session everywhere, including revoke, to avoid deadlocks.
		e := tx.QueryRowContext(r.Context(), `SELECT s.user_id,s.device_id,u.email,u.account_status,u.role,s.consumed,s.refresh_expires,d.revoked_at IS NOT NULL FROM sessions s JOIN devices d ON d.id=s.device_id JOIN users u ON u.id=s.user_id WHERE s.refresh_hash=$1 FOR UPDATE OF d`, Hash(q.Refresh)).Scan(&uid, &did, &email, &status, &role, &consumed, &expiry, &revoked)
		if errors.Is(e, sql.ErrNoRows) {
			invalid = true
			return nil
		}
		if e != nil {
			return e
		}
		// Refresh row again after device lock acquisition under READ COMMITTED.
		if e = tx.QueryRowContext(r.Context(), `SELECT consumed FROM sessions WHERE refresh_hash=$1 FOR UPDATE`, Hash(q.Refresh)).Scan(&consumed); e != nil {
			return e
		}
		if revoked || status != "active" || expiry.Before(time.Now()) {
			invalid = true
			return nil
		}
		if consumed {
			invalid = true
			if _, e = tx.ExecContext(r.Context(), `UPDATE devices SET revoked_at=now() WHERE id=$1`, did); e != nil {
				return e
			}
			if _, e = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET revoked_at=now() WHERE device_id=$1 AND revoked_at IS NULL`, did); e != nil {
				return e
			}
			return audit.Record(r.Context(), tx, uid, did, "refresh_reuse_revoked")
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE sessions SET consumed=true WHERE refresh_hash=$1`, Hash(q.Refresh)); e != nil {
			return e
		}
		out, e = session(r.Context(), tx, uid, did, email)
		if e != nil {
			return e
		}
		out.User["role"] = role
		return audit.Record(r.Context(), tx, uid, did, "refresh")
	})
	if err != nil {
		httpx.Error(w, 500, "internal", "刷新失败")
		return
	}
	if invalid {
		httpx.Error(w, 401, "session_revoked", "凭据过期、重用或设备已撤销，请重新登录")
		return
	}
	httpx.WriteJSON(w, 200, out)
}
func (s *Service) Devices(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,name,kind,created_at,revoked_at FROM devices WHERE user_id=$1 AND revoked_at IS NULL AND kind IN ('desktop','mobile') ORDER BY kind`, Who(r).UserID)
	if e != nil {
		httpx.Error(w, 500, "internal", "无法读取设备")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, kind string
		var created time.Time
		var revoked sql.NullTime
		if e = rows.Scan(&id, &name, &kind, &created, &revoked); e != nil {
			httpx.Error(w, 500, "internal", "无法读取设备")
			return
		}
		var rv any
		if revoked.Valid {
			rv = revoked.Time
		}
		out = append(out, map[string]any{"id": id, "name": name, "kind": kind, "created_at": created, "revoked_at": rv, "current": id == Who(r).DeviceID})
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "internal", "无法读取设备")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"devices": out})
}
func (s *Service) Revoke(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, 403, "admin_required", "请联系管理员解绑设备")
}
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	_, err := s.DB.ExecContext(r.Context(), `UPDATE sessions SET consumed=true WHERE access_hash=$1 AND device_id=$2`, Hash(header), Who(r).DeviceID)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "退出失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Service) revoke(w http.ResponseWriter, r *http.Request, did string) {
	found := false
	err := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(r.Context(), `UPDATE devices SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, did, Who(r).UserID)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return e
		}
		found = n > 0
		if !found {
			return nil
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET revoked_at=now() WHERE device_id=$1 AND revoked_at IS NULL`, did); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, did, "device_revoked")
	})
	if err != nil {
		httpx.Error(w, 500, "internal", "撤销失败")
		return
	}
	if !found {
		httpx.Error(w, 404, "not_found", "设备不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// CheckDevice holds a shared lock until the domain transaction ends; revocation is ordered against writes.
func CheckDevice(ctx context.Context, tx *sql.Tx, user, device string) error {
	var id string
	return tx.QueryRowContext(ctx, `SELECT d.id FROM devices d JOIN users u ON u.id=d.user_id WHERE d.id=$1 AND d.user_id=$2 AND d.revoked_at IS NULL AND u.account_status='active' FOR SHARE OF d`, device, user).Scan(&id)
}
