package auth

import (
	"aihub.dev/server/internal/audit"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"database/sql"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func adminOnly(w http.ResponseWriter, r *http.Request) bool {
	if Who(r).Role == "admin" {
		return true
	}
	httpx.Error(w, 403, "admin_required", "需要管理员权限")
	return false
}

func validEmail(email string) bool {
	a, e := mail.ParseAddress(email)
	return e == nil && a.Address == email && len(email) <= 254
}

func validPassword(password string) bool { return len(password) >= 8 && len(password) <= 72 }

func (s *Service) AdminUsers(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT u.id,u.email,u.role,u.account_status,u.created_at FROM users u ORDER BY u.created_at DESC`)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "账户列表暂不可用")
		return
	}
	defer rows.Close()
	users := []map[string]any{}
	for rows.Next() {
		var id, email, role, status, created string
		if e = rows.Scan(&id, &email, &role, &status, &created); e != nil {
			break
		}
		users = append(users, map[string]any{"id": id, "email": email, "role": role, "status": status, "created_at": created})
	}
	if e != nil || rows.Err() != nil {
		httpx.Error(w, 503, "unavailable", "账户列表暂不可用")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"users": users})
}

func (s *Service) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	var q struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.Email = strings.ToLower(strings.TrimSpace(q.Email))
	if !validEmail(q.Email) || !validPassword(q.Password) {
		httpx.Error(w, 400, "invalid_input", "检查邮箱和密码")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
	if e != nil {
		httpx.Error(w, 500, "internal", "密码处理失败")
		return
	}
	uid := httpx.NewID("usr")
	now := time.Now().UTC().Format(time.RFC3339)
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO users(id,email,password_hash,created_at,role,account_status) VALUES($1,$2,$3,$4,'member','active')`, uid, q.Email, string(hash), now); e != nil {
			return e
		}
		if s.OnRegister != nil {
			if e := s.OnRegister(r.Context(), tx, uid, now); e != nil {
				return e
			}
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "admin_user_created")
	})
	if e != nil {
		if strings.Contains(e.Error(), "23505") {
			httpx.Error(w, 409, "email_taken", "邮箱已存在")
		} else {
			httpx.Error(w, 503, "unavailable", "创建失败")
		}
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": uid, "email": q.Email, "role": "member", "status": "active"})
}

func (s *Service) AdminStatus(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	var q struct {
		Status string `json:"status"`
	}
	if httpx.Decode(r, &q) != nil || (q.Status != "active" && q.Status != "disabled") {
		httpx.Error(w, 400, "invalid_input", "状态只能为 active 或 disabled")
		return
	}
	uid := r.PathValue("id")
	if uid == Who(r).UserID && q.Status == "disabled" {
		httpx.Error(w, 409, "last_admin", "不能停用当前管理员")
		return
	}
	var found bool
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(r.Context(), `UPDATE users SET account_status=$1 WHERE id=$2`, q.Status, uid)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		found = n == 1
		if !found {
			return nil
		}
		if q.Status == "disabled" {
			if _, e = tx.ExecContext(r.Context(), `UPDATE devices SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, uid); e != nil {
				return e
			}
			if _, e = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, uid); e != nil {
				return e
			}
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "admin_user_status")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "操作失败")
		return
	}
	if !found {
		httpx.Error(w, 404, "not_found", "账户不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func (s *Service) AdminPassword(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	var q struct {
		Password string `json:"password"`
	}
	if httpx.Decode(r, &q) != nil || !validPassword(q.Password) {
		httpx.Error(w, 400, "invalid_input", "密码长度应为 8–72 字节")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
	if e != nil {
		httpx.Error(w, 500, "internal", "密码处理失败")
		return
	}
	uid := r.PathValue("id")
	var found bool
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=$1 WHERE id=$2`, string(hash), uid)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		found = n == 1
		if !found {
			return nil
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE sessions SET consumed=true WHERE user_id=$1`, uid); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "admin_password_reset")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "重置失败")
		return
	}
	if !found {
		httpx.Error(w, 404, "not_found", "账户不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func (s *Service) AdminUnbind(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	uid, did := r.PathValue("id"), r.PathValue("device")
	var found bool
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(r.Context(), `UPDATE devices SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, did, uid)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		found = n == 1
		if !found {
			return nil
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE codex_bridges SET revoked_at=now() WHERE device_id=$1 AND revoked_at IS NULL`, did); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "admin_device_unbound")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "解绑失败")
		return
	}
	if !found {
		httpx.Error(w, 404, "not_found", "设备不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func (s *Service) AdminDevices(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,name,kind,created_at,revoked_at FROM devices WHERE user_id=$1 ORDER BY created_at DESC`, r.PathValue("id"))
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	devices := []map[string]any{}
	for rows.Next() {
		var id, name, kind string
		var created time.Time
		var revoked sql.NullTime
		if e = rows.Scan(&id, &name, &kind, &created, &revoked); e != nil {
			break
		}
		var rv any
		if revoked.Valid {
			rv = revoked.Time
		}
		devices = append(devices, map[string]any{"id": id, "name": name, "kind": kind, "created_at": created, "revoked_at": rv})
	}
	if e != nil || rows.Err() != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"devices": devices})
}
