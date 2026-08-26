package api

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aihub.dev/server/internal/clock"
	"aihub.dev/server/internal/httpx"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

type Server struct {
	db    *sql.DB
	clock clock.Clock
	dist  string
}

func New(database *sql.DB, clk clock.Clock, distDir string) *Server {
	if clk == nil {
		clk = clock.Real{}
	}
	return &Server{db: database, clock: clk, dist: distDir}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)

	mux.Handle("GET /api/v1/me", s.authed(s.me))
	mux.Handle("GET /api/v1/providers", s.authed(s.listProviders))
	mux.Handle("GET /api/v1/dashboard", s.authed(s.dashboard))

	mux.Handle("GET /api/v1/provider-accounts", s.authed(s.listAccounts))
	mux.Handle("POST /api/v1/provider-accounts", s.authed(s.createAccount))
	mux.Handle("GET /api/v1/provider-accounts/{id}", s.authed(s.getAccount))
	mux.Handle("DELETE /api/v1/provider-accounts/{id}", s.authed(s.deleteAccount))
	mux.Handle("POST /api/v1/provider-accounts/{id}/quota-buckets", s.authed(s.createBucket))

	mux.Handle("POST /api/v1/quota-buckets/{id}/manual-snapshot", s.authed(s.manualSnapshot))
	mux.Handle("GET /api/v1/quota-buckets/{id}/snapshots", s.authed(s.listSnapshots))

	mux.Handle("GET /api/v1/notification-rules", s.authed(s.listRules))
	mux.Handle("POST /api/v1/notification-rules", s.authed(s.createRule))
	mux.Handle("PATCH /api/v1/notification-rules/{id}", s.authed(s.patchRule))
	mux.Handle("DELETE /api/v1/notification-rules/{id}", s.authed(s.deleteRule))
	mux.Handle("POST /api/v1/notification-rules/preview", s.authed(s.previewRuleBody))
	mux.Handle("POST /api/v1/notification-rules/{id}/preview", s.authed(s.previewRule))

	mux.Handle("GET /api/v1/notifications", s.authed(s.listNotifications))
	mux.Handle("POST /api/v1/notifications/{id}/read", s.authed(s.readNotification))
	mux.Handle("POST /api/v1/notifications/read-all", s.authed(s.readAllNotifications))

	if s.dist != "" {
		if _, err := os.Stat(s.dist); err == nil {
			mux.Handle("/", spaHandler(s.dist))
		}
	}
	return httpx.CORS(httpx.Middleware(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	err := s.db.PingContext(r.Context())
	status := "ok"
	code := http.StatusOK
	if err != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	httpx.WriteJSON(w, code, map[string]any{
		"status":     status,
		"time":       s.clock.Now().Format(time.RFC3339),
		"request_id": httpx.RequestID(r),
	})
}

func (s *Server) authed(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
		token = strings.TrimSpace(token)
		if token == "" {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "需要登录")
			return
		}
		var userID, expires string
		err := s.db.QueryRowContext(r.Context(), `SELECT user_id, expires_at FROM sessions WHERE token = ?`, token).Scan(&userID, &expires)
		if err == sql.ErrNoRows {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "登录已失效")
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "读取会话失败")
			return
		}
		exp, err := time.Parse(time.RFC3339, expires)
		if err != nil || exp.Before(s.clock.Now()) {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "登录已过期")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next(w, r.WithContext(ctx))
	})
}

func userID(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}

func spaHandler(dir string) http.Handler {
	root := os.DirFS(dir)
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(root, p); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func (s *Server) nowRFC() string {
	return s.clock.Now().UTC().Format(time.RFC3339)
}

func parseTimePtr(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04", s)
	}
	if err != nil {
		return nil, err
	}
	utc := t.UTC()
	return &utc, nil
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func nullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, ns.String)
	if err != nil {
		return nil
	}
	return &t
}

func nullF64(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func timeOut(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func f64Out(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func argTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func argF64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func logErr(msg string, err error, requestID string) {
	slog.Error(msg, "error", err.Error(), "request_id", requestID)
}
