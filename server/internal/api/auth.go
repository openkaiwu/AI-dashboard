package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/notification"
	"golang.org/x/crypto/bcrypt"
)

type authReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || len(req.Password) < 8 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "邮箱格式不正确，或密码少于 8 位")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "无法保存密码")
		return
	}
	id := httpx.NewID("usr")
	now := s.nowRFC()
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "无法开始事务")
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, id, email, string(hash), now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			httpx.Error(w, http.StatusConflict, "email_taken", "该邮箱已注册")
			return
		}
		logErr("register insert", err, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "注册失败")
		return
	}
	if err := insertDefaultRules(ctx, tx, id, now); err != nil {
		logErr("register rules", err, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "初始化提醒规则失败")
		return
	}
	token, exp, err := createSession(ctx, tx, id, s.clock.Now())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "创建会话失败")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "注册失败")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"token":      token,
		"expires_at": exp.Format(time.RFC3339),
		"user":       map[string]any{"id": id, "email": email},
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json", "请求格式不正确")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	var id, hash string
	err := s.db.QueryRowContext(r.Context(), `SELECT id, password_hash FROM users WHERE email = ?`, email).Scan(&id, &hash)
	if err == sql.ErrNoRows {
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "登录失败")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	token, exp, err := createSession(r.Context(), s.db, id, s.clock.Now())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "创建会话失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp.Format(time.RFC3339),
		"user":       map[string]any{"id": id, "email": email},
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	var email, created string
	err := s.db.QueryRowContext(r.Context(), `SELECT email, created_at FROM users WHERE id = ?`, userID(r)).Scan(&email, &created)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取用户失败")
		return
	}
	var unread int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND status = 'unread'`, userID(r)).Scan(&unread)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":            userID(r),
		"email":         email,
		"created_at":    created,
		"unread_count":  unread,
	})
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertDefaultRules(ctx context.Context, tx execer, userID, now string) error {
	for _, def := range notification.DefaultRules() {
		raw, _ := json.Marshal(def.Params)
		_, err := tx.ExecContext(ctx, `INSERT INTO notification_rules (id, user_id, name, rule_type, enabled, params_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, 1, ?, ?, ?)`, httpx.NewID("rule"), userID, def.Name, def.Type, string(raw), now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func createSession(ctx context.Context, tx execer, userID string, now time.Time) (string, time.Time, error) {
	token := httpx.Token()
	exp := now.UTC().Add(7 * 24 * time.Hour)
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, userID, exp.Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	return token, exp, err
}
