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
}

func Who(r *http.Request) Identity { v, _ := r.Context().Value(identityKey{}).(Identity); return v }
func Hash(s string) string         { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

type Service struct {
	DB         *sql.DB
	OnRegister func(context.Context, *sql.Tx, string, string) error
}
type credentials struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceName string `json:"device_name"`
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
func (s *Service) Register(w http.ResponseWriter, r *http.Request) { s.signIn(w, r, true) }
func (s *Service) Login(w http.ResponseWriter, r *http.Request)    { s.signIn(w, r, false) }
func (s *Service) signIn(w http.ResponseWriter, r *http.Request, register bool) {
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
	if err != nil || addr.Address != q.Email || len(q.Email) > 254 || len(q.Password) < 8 || len(q.Password) > 72 || len(q.DeviceName) > 120 {
		httpx.Error(w, 400, "invalid_input", "检查邮箱、密码（8–72 字节）及设备名称")
		return
	}
	var uid, hash string
	if register {
		uid = httpx.NewID("usr")
		raw, e := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
		if e != nil {
			httpx.Error(w, 500, "internal", "密码处理失败")
			return
		}
		hash = string(raw)
	} else {
		e := s.DB.QueryRowContext(r.Context(), `SELECT id,password_hash FROM users WHERE email=$1`, q.Email).Scan(&uid, &hash)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			httpx.Error(w, 503, "auth_unavailable", "认证服务暂不可用，请稍后重试")
			return
		}
		if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(q.Password)) != nil {
			httpx.Error(w, 401, "invalid_credentials", "邮箱或密码不正确")
			return
		}
	}
	var out Session
	err = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if register {
			if _, e := tx.ExecContext(r.Context(), `INSERT INTO users(id,email,password_hash,created_at) VALUES($1,$2,$3,$4)`, uid, q.Email, hash, time.Now().UTC().Format(time.RFC3339)); e != nil {
				return e
			}
			if s.OnRegister != nil {
				if e := s.OnRegister(r.Context(), tx, uid, time.Now().UTC().Format(time.RFC3339)); e != nil {
					return e
				}
			}
		}
		did := httpx.NewID("dev")
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO devices(id,user_id,name) VALUES($1,$2,$3)`, did, uid, q.DeviceName); e != nil {
			return e
		}
		var e error
		out, e = session(r.Context(), tx, uid, did, q.Email)
		if e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, uid, did, "login")
	})
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			httpx.Error(w, 409, "email_taken", "该邮箱已注册")
		} else {
			httpx.Error(w, 500, "internal", "登录失败")
		}
		return
	}
	code := 200
	if register {
		code = 201
	}
	httpx.WriteJSON(w, code, out)
}
func (s *Service) Middleware(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, 401, "unauthenticated", "需要登录")
			return
		}
		var who Identity
		err := s.DB.QueryRowContext(r.Context(), `SELECT s.user_id,s.device_id FROM sessions s JOIN devices d ON d.id=s.device_id WHERE s.access_hash=$1 AND NOT s.consumed AND s.access_expires>now() AND d.revoked_at IS NULL`, Hash(strings.TrimPrefix(header, "Bearer "))).Scan(&who.UserID, &who.DeviceID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.Error(w, 401, "unauthenticated", "登录过期或设备已撤销")
			} else {
				httpx.Error(w, 503, "auth_unavailable", "认证服务暂不可用，请稍后重试")
			}
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
		var uid, did, email string
		var consumed, revoked bool
		var expiry time.Time
		// Lock device before session everywhere, including revoke, to avoid deadlocks.
		e := tx.QueryRowContext(r.Context(), `SELECT s.user_id,s.device_id,u.email,s.consumed,s.refresh_expires,d.revoked_at IS NOT NULL FROM sessions s JOIN devices d ON d.id=s.device_id JOIN users u ON u.id=s.user_id WHERE s.refresh_hash=$1 FOR UPDATE OF d`, Hash(q.Refresh)).Scan(&uid, &did, &email, &consumed, &expiry, &revoked)
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
		if revoked || expiry.Before(time.Now()) {
			invalid = true
			return nil
		}
		if consumed {
			invalid = true
			if _, e = tx.ExecContext(r.Context(), `UPDATE devices SET revoked_at=now() WHERE id=$1`, did); e != nil {
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
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,name,created_at,revoked_at FROM devices WHERE user_id=$1 ORDER BY created_at DESC`, Who(r).UserID)
	if e != nil {
		httpx.Error(w, 500, "internal", "无法读取设备")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var created time.Time
		var revoked sql.NullTime
		if e = rows.Scan(&id, &name, &created, &revoked); e != nil {
			httpx.Error(w, 500, "internal", "无法读取设备")
			return
		}
		var rv any
		if revoked.Valid {
			rv = revoked.Time
		}
		out = append(out, map[string]any{"id": id, "name": name, "created_at": created, "revoked_at": rv, "current": id == Who(r).DeviceID})
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "internal", "无法读取设备")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"devices": out})
}
func (s *Service) Revoke(w http.ResponseWriter, r *http.Request) { s.revoke(w, r, r.PathValue("id")) }
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) { s.revoke(w, r, Who(r).DeviceID) }
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
	return tx.QueryRowContext(ctx, `SELECT id FROM devices WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL FOR SHARE`, device, user).Scan(&id)
}
