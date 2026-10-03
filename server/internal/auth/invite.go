package auth

import (
	"aihub.dev/server/internal/audit"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Codes exclude I/L/O/0/1 look-alikes; 16 chars over 31 symbols ≈ 79 bits,
// so online guessing is infeasible under the register rate limit.
const inviteAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

var inviteCodePattern = regexp.MustCompile(`^[0-9A-Z]{10,64}$`)

// registerAttempts caps registration work per remote address; fixed-window
// in-process limiting matches the single-instance deployment target.
var registerAttempts = newIPLimiter(20, time.Hour)

var (
	errEmailPending  = errors.New("email pending review")
	errEmailTaken    = errors.New("email taken")
	errEmailRejected = errors.New("email previously rejected")
	errInviteUnusable = errors.New("invite code unusable")
	errAlreadyReviewed = errors.New("application already reviewed")
	errNotFound       = errors.New("application not found")
)

func normalizeInviteCode(code string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), "-", ""), " ", "")
}

func generateInviteCode() (string, error) {
	raw := make([]byte, 16)
	if _, e := rand.Read(raw); e != nil {
		return "", e
	}
	buf := make([]byte, 16)
	for i, v := range raw {
		buf[i] = inviteAlphabet[int(v)%len(inviteAlphabet)]
	}
	groups := make([]string, 4)
	for g := 0; g < 4; g++ {
		groups[g] = string(buf[g*4 : g*4+4])
	}
	return strings.Join(groups, "-"), nil
}

type registerRequest struct {
	InviteCode string `json:"invite_code"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Note       string `json:"note"`
}

// Register creates a pending user from an invite code; an administrator must
// approve the account before the first sign-in.
func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	if !registerAttempts.Allow(r) {
		httpx.Error(w, 429, "too_many_attempts", "注册尝试过于频繁，请稍后再试")
		return
	}
	var q registerRequest
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.Email = strings.ToLower(strings.TrimSpace(q.Email))
	q.Note = strings.TrimSpace(q.Note)
	code := normalizeInviteCode(q.InviteCode)
	if !inviteCodePattern.MatchString(code) || !validEmail(q.Email) || !validPassword(q.Password) || len(q.Note) > 200 {
		time.Sleep(300 * time.Millisecond)
		httpx.Error(w, 400, "invalid_input", "检查邀请码、邮箱和密码（8–72 字节）")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
	if e != nil {
		httpx.Error(w, 500, "internal", "密码处理失败")
		return
	}
	codeHash := Hash(code)
	uid := httpx.NewID("usr")
	now := time.Now().UTC().Format(time.RFC3339)
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		var existing, status string
		e := tx.QueryRowContext(r.Context(), `SELECT id,account_status FROM users WHERE email=$1`, q.Email).Scan(&existing, &status)
		if e == nil {
			switch status {
			case "pending":
				return errEmailPending
			case "active":
				return errEmailTaken
			default:
				return errEmailRejected
			}
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var inviteID string
		e = tx.QueryRowContext(r.Context(), `UPDATE invite_codes SET use_count=use_count+1 WHERE code_hash=$1 AND status='active' AND use_count<max_uses AND (expires_at IS NULL OR expires_at>now()) RETURNING id`, codeHash).Scan(&inviteID)
		if errors.Is(e, sql.ErrNoRows) {
			return errInviteUnusable
		}
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO users(id,email,password_hash,created_at,role,account_status) VALUES($1,$2,$3,$4,'member','pending')`, uid, q.Email, string(hash), now); e != nil {
			return e
		}
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO registration_applications(id,user_id,email,invite_code_id,note,created_at) VALUES($1,$2,$3,$4,$5,$6)`, httpx.NewID("reg"), uid, q.Email, inviteID, q.Note, now); e != nil {
			return e
		}
		if s.OnRegister != nil {
			if e = s.OnRegister(r.Context(), tx, uid, now); e != nil {
				return e
			}
		}
		return audit.Record(r.Context(), tx, uid, "", "registration_submitted")
	})
	if e != nil {
		switch {
		case errors.Is(e, errEmailPending):
			httpx.Error(w, 409, "email_pending_review", "该邮箱已提交注册，正在等待管理员审核")
		case errors.Is(e, errEmailTaken):
			httpx.Error(w, 409, "email_taken", "邮箱已被使用")
		case errors.Is(e, errEmailRejected):
			httpx.Error(w, 409, "email_rejected", "该邮箱的注册申请未通过审核")
		case errors.Is(e, errInviteUnusable):
			s.rejectInvite(w, r, codeHash)
		default:
			if strings.Contains(e.Error(), "23505") {
				httpx.Error(w, 409, "email_taken", "邮箱已被使用")
				return
			}
			httpx.Error(w, 503, "unavailable", "注册暂不可用，请稍后重试")
		}
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"status": "pending", "message": "注册已提交，等待管理员审核"})
}

// rejectInvite reports why a code could not be consumed; the extra lookup
// keeps the reasons distinguishable for the applicant.
func (s *Service) rejectInvite(w http.ResponseWriter, r *http.Request, codeHash string) {
	time.Sleep(300 * time.Millisecond)
	var status string
	var useCount, maxUses int
	var expires sql.NullTime
	e := s.DB.QueryRowContext(r.Context(), `SELECT status,use_count,max_uses,expires_at FROM invite_codes WHERE code_hash=$1`, codeHash).Scan(&status, &useCount, &maxUses, &expires)
	if e != nil {
		httpx.Error(w, 400, "invite_invalid", "邀请码无效")
		return
	}
	switch {
	case status == "revoked":
		httpx.Error(w, 400, "invite_revoked", "邀请码已被吊销")
	case expires.Valid && expires.Time.Before(time.Now()):
		httpx.Error(w, 400, "invite_expired", "邀请码已过期")
	case useCount >= maxUses:
		httpx.Error(w, 400, "invite_exhausted", "邀请码使用次数已用完")
	default:
		httpx.Error(w, 400, "invite_invalid", "邀请码无效")
	}
}

// --- administrator: invite codes ---

func (s *Service) AdminCreateInvite(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	var q struct {
		Note    string `json:"note"`
		Days    int    `json:"days"`
		MaxUses int    `json:"max_uses"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.Note = strings.TrimSpace(q.Note)
	maxUses := q.MaxUses
	if maxUses == 0 {
		maxUses = 1
	}
	if len(q.Note) > 120 || q.Days < 0 || q.Days > 365 || maxUses < 1 || maxUses > 100 {
		httpx.Error(w, 400, "invalid_input", "检查备注、有效天数（0–365）和可用次数（1–100）")
		return
	}
	code, e := generateInviteCode()
	if e != nil {
		httpx.Error(w, 500, "internal", "邀请码生成失败")
		return
	}
	id := httpx.NewID("inv")
	createdAt := time.Now().UTC()
	expiresAt := sql.NullTime{}
	if q.Days > 0 {
		expiresAt = sql.NullTime{Time: createdAt.AddDate(0, 0, q.Days), Valid: true}
	}
	// Store the normalized form so user input with different separators or
	// casing still matches.
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO invite_codes(id,code_hash,note,max_uses,expires_at,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, Hash(normalizeInviteCode(code)), q.Note, maxUses, expiresAt, Who(r).UserID, createdAt); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "invite_created")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "创建失败")
		return
	}
	out := map[string]any{"id": id, "code": code, "note": q.Note, "max_uses": maxUses, "status": "active"}
	if expiresAt.Valid {
		out["expires_at"] = expiresAt.Time.Format(time.RFC3339)
	}
	httpx.WriteJSON(w, 201, out)
}

func (s *Service) AdminInvites(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,note,max_uses,use_count,status,expires_at,created_at FROM invite_codes ORDER BY created_at DESC`)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "邀请码列表暂不可用")
		return
	}
	defer rows.Close()
	invites := []map[string]any{}
	for rows.Next() {
		var id, note, status, createdAt string
		var maxUses, useCount int
		var expires sql.NullTime
		if e = rows.Scan(&id, &note, &maxUses, &useCount, &status, &expires, &createdAt); e != nil {
			break
		}
		item := map[string]any{"id": id, "note": note, "max_uses": maxUses, "use_count": useCount, "status": status, "created_at": createdAt}
		if expires.Valid {
			item["expires_at"] = expires.Time.Format(time.RFC3339)
		}
		invites = append(invites, item)
	}
	if e != nil || rows.Err() != nil {
		httpx.Error(w, 503, "unavailable", "邀请码列表暂不可用")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"invites": invites})
}

func (s *Service) AdminInviteStatus(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	var q struct {
		Status string `json:"status"`
	}
	if httpx.Decode(r, &q) != nil || (q.Status != "active" && q.Status != "revoked") {
		httpx.Error(w, 400, "invalid_input", "状态只能为 active 或 revoked")
		return
	}
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(r.Context(), `UPDATE invite_codes SET status=$1 WHERE id=$2`, q.Status, r.PathValue("id"))
		if e != nil {
			return e
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound
		}
		action := "invite_restored"
		if q.Status == "revoked" {
			action = "invite_revoked"
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, action)
	})
	if e != nil {
		if errors.Is(e, errNotFound) {
			httpx.Error(w, 404, "invite_not_found", "邀请码不存在")
			return
		}
		httpx.Error(w, 503, "unavailable", "操作失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

// --- administrator: registration review ---

func (s *Service) AdminRegistrations(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	query := `SELECT a.id,a.email,a.note,a.status,a.created_at,a.reviewed_at,a.reject_reason,u.account_status,i.note FROM registration_applications a JOIN users u ON u.id=a.user_id JOIN invite_codes i ON i.id=a.invite_code_id`
	args := []any{}
	if status != "all" {
		if status != "pending" && status != "approved" && status != "rejected" {
			httpx.Error(w, 400, "invalid_input", "状态过滤条件不正确")
			return
		}
		query += ` WHERE a.status=$1`
		args = append(args, status)
	}
	query += ` ORDER BY a.created_at DESC LIMIT 200`
	rows, e := s.DB.QueryContext(r.Context(), query, args...)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "审核列表暂不可用")
		return
	}
	defer rows.Close()
	registrations := []map[string]any{}
	for rows.Next() {
		var id, email, note, appStatus, createdAt, userStatus, inviteNote string
		var reviewedAt sql.NullString
		var rejectReason string
		if e = rows.Scan(&id, &email, &note, &appStatus, &createdAt, &reviewedAt, &rejectReason, &userStatus, &inviteNote); e != nil {
			break
		}
		item := map[string]any{"id": id, "email": email, "note": note, "status": appStatus, "created_at": createdAt, "account_status": userStatus, "invite_note": inviteNote, "reject_reason": rejectReason}
		if reviewedAt.Valid {
			item["reviewed_at"] = reviewedAt.String
		}
		registrations = append(registrations, item)
	}
	if e != nil || rows.Err() != nil {
		httpx.Error(w, 503, "unavailable", "审核列表暂不可用")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"registrations": registrations})
}

func (s *Service) AdminApproveRegistration(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		var uid, appStatus string
		e := tx.QueryRowContext(r.Context(), `SELECT user_id,status FROM registration_applications WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&uid, &appStatus)
		if errors.Is(e, sql.ErrNoRows) {
			return errNotFound
		}
		if e != nil {
			return e
		}
		if appStatus != "pending" {
			return errAlreadyReviewed
		}
		res, e := tx.ExecContext(r.Context(), `UPDATE users SET account_status='active' WHERE id=$1 AND account_status='pending'`, uid)
		if e != nil {
			return e
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errAlreadyReviewed
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE registration_applications SET status='approved',reviewed_by=$1,reviewed_at=now() WHERE id=$2`, Who(r).UserID, r.PathValue("id")); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "registration_approved")
	})
	if e != nil {
		switch {
		case errors.Is(e, errNotFound):
			httpx.Error(w, 404, "registration_not_found", "注册申请不存在")
		case errors.Is(e, errAlreadyReviewed):
			httpx.Error(w, 409, "already_reviewed", "该申请已被处理")
		default:
			httpx.Error(w, 503, "unavailable", "操作失败")
		}
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func (s *Service) AdminRejectRegistration(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	var q struct {
		Reason string `json:"reason"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.Reason = strings.TrimSpace(q.Reason)
	if q.Reason == "" || len(q.Reason) > 200 {
		httpx.Error(w, 400, "reason_required", "请填写拒绝原因（200 字以内）")
		return
	}
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		var uid, appStatus string
		e := tx.QueryRowContext(r.Context(), `SELECT user_id,status FROM registration_applications WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&uid, &appStatus)
		if errors.Is(e, sql.ErrNoRows) {
			return errNotFound
		}
		if e != nil {
			return e
		}
		if appStatus != "pending" {
			return errAlreadyReviewed
		}
		res, e := tx.ExecContext(r.Context(), `UPDATE users SET account_status='disabled' WHERE id=$1 AND account_status='pending'`, uid)
		if e != nil {
			return e
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errAlreadyReviewed
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE registration_applications SET status='rejected',reviewed_by=$1,reviewed_at=now(),reject_reason=$2 WHERE id=$3`, Who(r).UserID, q.Reason, r.PathValue("id")); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, Who(r).UserID, Who(r).DeviceID, "registration_rejected")
	})
	if e != nil {
		switch {
		case errors.Is(e, errNotFound):
			httpx.Error(w, 404, "registration_not_found", "注册申请不存在")
		case errors.Is(e, errAlreadyReviewed):
			httpx.Error(w, 409, "already_reviewed", "该申请已被处理")
		default:
			httpx.Error(w, 503, "unavailable", "操作失败")
		}
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}
