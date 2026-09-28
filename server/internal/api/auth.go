package api

import (
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/notification"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
)

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	var email, created string
	err := s.db.QueryRowContext(r.Context(), `SELECT email, created_at FROM users WHERE id = $1`, userID(r)).Scan(&email, &created)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "读取用户失败")
		return
	}
	var unread int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND status = 'unread'`, userID(r)).Scan(&unread)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":           userID(r),
		"email":        email,
		"created_at":   created,
		"unread_count": unread,
	})
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertDefaultRules(ctx context.Context, tx execer, userID, now string) error {
	for _, def := range notification.DefaultRules() {
		raw, _ := json.Marshal(def.Params)
		_, err := tx.ExecContext(ctx, `INSERT INTO notification_rules (id, user_id, name, rule_type, enabled, params_json, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 1, $5, $6, $7)`, httpx.NewID("rule"), userID, def.Name, def.Type, string(raw), now, now)
		if err != nil {
			return err
		}
	}
	return nil
}
