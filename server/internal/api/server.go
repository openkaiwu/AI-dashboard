package api

import (
	"aihub.dev/server/webassets"
	"bytes"
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/clock"
	"aihub.dev/server/internal/config"
	"aihub.dev/server/internal/conversation"
	"aihub.dev/server/internal/connector"
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/promotion"
	"aihub.dev/server/internal/provider"
	syncservice "aihub.dev/server/internal/sync"
	"aihub.dev/server/internal/telemetry"
	"aihub.dev/server/internal/workspace"
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
	authService := &auth.Service{DB: s.db, OnRegister: func(ctx context.Context, tx *sql.Tx, uid, now string) error {
		return insertDefaultRules(ctx, tx, uid, now)
	}}
	syncService := &syncservice.Service{DB: s.db}
	bridgeService := &connector.Service{DB: s.db, AfterQuotaWrite: func(ctx context.Context, uid string) error {
		return s.evaluateUser(ctx, uid)
	}}
	mux.Handle("GET /api/v1/codex/overview", authService.Middleware(bridgeService.Overview))
	mux.Handle("GET /api/v1/codex/consumption", authService.Middleware(bridgeService.Consumption))
	mux.Handle("GET /api/v1/codex/plan", authService.Middleware(bridgeService.Plan))
	mux.Handle("PUT /api/v1/codex/plan", authService.Middleware(bridgeService.Plan))
	mux.Handle("PATCH /api/v1/codex/preferences", authService.Middleware(bridgeService.Preferences))
	mux.Handle("POST /api/v1/codex/alerts/{id}", authService.Middleware(bridgeService.AlertAction))
	mux.Handle("GET /api/v1/codex/bridges", authService.Middleware(bridgeService.List))
	mux.Handle("POST /api/v1/codex/bridges", authService.Middleware(bridgeService.Create))
	mux.Handle("DELETE /api/v1/codex/bridges/{id}", authService.Middleware(bridgeService.Revoke))
	mux.HandleFunc("POST /api/v1/codex/snapshot", bridgeService.Upload)
	mux.Handle("POST /api/v1/admin/radar-token", authService.Middleware(bridgeService.RadarTokenCreate))
	mux.Handle("GET /api/v1/admin/radar-token", authService.Middleware(bridgeService.RadarTokenStatus))
	mux.Handle("DELETE /api/v1/admin/radar-token", authService.Middleware(bridgeService.RadarTokenRevoke))
	mux.HandleFunc("POST /api/v1/radar/news", bridgeService.RadarNews)
	mux.HandleFunc("POST /api/v1/cursor/snapshot", bridgeService.UploadCursor)
	mux.HandleFunc("POST /api/v1/official/snapshot", bridgeService.UploadOfficial)
	mux.HandleFunc("POST /api/v1/connectors/sample", bridgeService.SampleUpload)
	mux.HandleFunc("POST /api/v1/bridge/config-scan", bridgeService.ConfigScan)
	mux.HandleFunc("GET /ready", s.health)
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, 200, map[string]any{"protocol": 1, "schema": 1, "version": "0.2.0-m0"})
	})
	mux.HandleFunc("POST /api/v1/auth/refresh", authService.Refresh)
	mux.Handle("GET /api/v1/admin/users", authService.Middleware(authService.AdminUsers))
	mux.Handle("GET /api/v1/admin/telemetry", authService.Middleware(s.adminTelemetry))
	mux.Handle("GET /api/v1/admin/operations", authService.Middleware(s.adminOperations))
	mux.Handle("POST /api/v1/admin/users", authService.Middleware(authService.AdminCreateUser))
	mux.Handle("PATCH /api/v1/admin/users/{id}/status", authService.Middleware(authService.AdminStatus))
	mux.Handle("POST /api/v1/admin/users/{id}/password", authService.Middleware(authService.AdminPassword))
	mux.Handle("GET /api/v1/admin/users/{id}/devices", authService.Middleware(authService.AdminDevices))
	mux.Handle("DELETE /api/v1/admin/users/{id}/devices/{device}", authService.Middleware(authService.AdminUnbind))
	mux.Handle("GET /api/v1/admin/invites", authService.Middleware(authService.AdminInvites))
	mux.Handle("POST /api/v1/admin/invites", authService.Middleware(authService.AdminCreateInvite))
	mux.Handle("PATCH /api/v1/admin/invites/{id}/status", authService.Middleware(authService.AdminInviteStatus))
	mux.Handle("GET /api/v1/admin/registrations", authService.Middleware(authService.AdminRegistrations))
	mux.Handle("POST /api/v1/admin/registrations/{id}/approve", authService.Middleware(authService.AdminApproveRegistration))
	mux.Handle("POST /api/v1/admin/registrations/{id}/reject", authService.Middleware(authService.AdminRejectRegistration))
	mux.Handle("POST /api/v1/auth/logout", authService.Middleware(authService.Logout))
	mux.Handle("GET /api/v1/devices", authService.Middleware(authService.Devices))
	mux.Handle("DELETE /api/v1/devices/{id}", authService.Middleware(authService.Revoke))
	mux.Handle("POST /api/v1/sync/push", authService.Middleware(syncService.Push))
	mux.Handle("GET /api/v1/sync/pull", authService.Middleware(syncService.Pull))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/auth/register", authService.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authService.Login)

	mux.Handle("GET /api/v1/me", s.authed(s.me))
	mux.Handle("GET /api/v1/providers", s.authed(s.listProviders))
	mux.Handle("GET /api/v1/connectors", s.authed(func(w http.ResponseWriter, r *http.Request) {
		manifests := []provider.Manifest{}
		for _, c := range provider.LocalConnectors() {
			manifests = append(manifests, c.Manifest())
		}
		httpx.WriteJSON(w, 200, map[string]any{"connectors": manifests})
	}))
	mux.Handle("GET /api/v1/dashboard", s.authed(s.dashboard))

	mux.Handle("GET /api/v1/provider-accounts", s.authed(s.listAccounts))
	mux.Handle("POST /api/v1/provider-accounts", s.authed(s.createAccount))
	mux.Handle("GET /api/v1/provider-accounts/{id}", s.authed(s.getAccount))
	mux.Handle("PATCH /api/v1/provider-accounts/{id}", s.authed(s.patchAccount))
	mux.Handle("DELETE /api/v1/provider-accounts/{id}", s.authed(s.deleteAccount))
	mux.Handle("POST /api/v1/provider-accounts/{id}/quota-buckets", s.authed(s.createBucket))
	mux.Handle("GET /api/v1/provider-accounts/{id}/billing-events", s.authed(s.billingEvents))
	mux.Handle("POST /api/v1/provider-accounts/{id}/billing-events", s.authed(s.billingEvents))

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
	mux.Handle("POST /api/v1/notifications/{id}/action", s.authed(s.notificationAction))
	mux.Handle("POST /api/v1/notifications/read-all", s.authed(s.readAllNotifications))

	// M5 workspace & collaboration + M6 promotion intelligence.
	hub := workspace.NewHub()
	workspaceService := &workspace.Service{DB: s.db}
	workspaceService.UseHub(hub)
	promotionService := &promotion.Service{DB: s.db, Notify: s.promotionNotify}

	// M3 conversation portability.
	conversationService := &conversation.Service{DB: s.db, OnShare: func(ctx context.Context, userID, resourceID, wsID string) error {
		return workspaceService.SetResourceWorkspace(ctx, userID, userID, wsID, "conversation", resourceID)
	}}
	mux.Handle("POST /api/v1/projects", authService.Middleware(conversationService.CreateProject))
	mux.Handle("GET /api/v1/projects", authService.Middleware(conversationService.ListProjects))
	mux.Handle("DELETE /api/v1/projects/{id}", authService.Middleware(conversationService.DeleteProject))
	mux.Handle("POST /api/v1/conversations/import", authService.Middleware(conversationService.Import))
	mux.Handle("GET /api/v1/conversations", authService.Middleware(conversationService.ListConversations))
	mux.Handle("GET /api/v1/conversations/{id}", authService.Middleware(conversationService.GetConversation))
	mux.Handle("PATCH /api/v1/conversations/{id}", authService.Middleware(conversationService.PatchConversation))
	mux.Handle("DELETE /api/v1/conversations/{id}", authService.Middleware(conversationService.DeleteConversation))
	mux.Handle("GET /api/v1/conversations/{id}/export", authService.Middleware(conversationService.ExportConversation))
	mux.Handle("GET /api/v1/imports", authService.Middleware(conversationService.ListImports))
	mux.Handle("GET /api/v1/imports/{id}", authService.Middleware(conversationService.ImportStatus))
	mux.Handle("GET /api/v1/imports/{id}/raw", authService.Middleware(conversationService.ImportRaw))

	// M4 portable config.
	configService := &config.Service{DB: s.db, OnShare: func(ctx context.Context, userID, resourceID, wsID string) error {
		return workspaceService.SetResourceWorkspace(ctx, userID, userID, wsID, "config_asset", resourceID)
	}}
	mux.Handle("POST /api/v1/config-assets/import", authService.Middleware(configService.ImportFromPlatform))
	mux.Handle("POST /api/v1/config-assets", authService.Middleware(configService.Create))
	mux.Handle("GET /api/v1/config-assets", authService.Middleware(configService.List))
	mux.Handle("GET /api/v1/config-assets/{id}", authService.Middleware(configService.Get))
	mux.Handle("PATCH /api/v1/config-assets/{id}", authService.Middleware(configService.Patch))
	mux.Handle("DELETE /api/v1/config-assets/{id}", authService.Middleware(configService.Delete))
	mux.Handle("POST /api/v1/config-assets/{id}/versions", authService.Middleware(configService.AddVersion))
	mux.Handle("GET /api/v1/config-assets/{id}/diff", authService.Middleware(configService.Diff))
	mux.Handle("POST /api/v1/config-assets/{id}/rollback", authService.Middleware(configService.Rollback))
	mux.Handle("POST /api/v1/config-assets/{id}/transform", authService.Middleware(configService.TransformPreview))
	mux.Handle("POST /api/v1/config-assets/{id}/bindings", authService.Middleware(configService.CreateBinding))
	mux.Handle("GET /api/v1/config-discoveries", authService.Middleware(configService.ListDiscoveries))

	// Workspace, promotion routes (services constructed above).
	mux.Handle("POST /api/v1/workspaces", authService.Middleware(workspaceService.Create))
	mux.Handle("GET /api/v1/workspaces", authService.Middleware(workspaceService.List))
	mux.Handle("GET /api/v1/workspaces/{id}/members", authService.Middleware(workspaceService.Members))
	mux.Handle("POST /api/v1/workspaces/{id}/invites", authService.Middleware(workspaceService.Invite))
	mux.Handle("GET /api/v1/workspaces/{id}/invites", authService.Middleware(workspaceService.ListInvites))
	mux.Handle("POST /api/v1/invites/{id}/accept", authService.Middleware(workspaceService.AcceptInvite))
	mux.Handle("GET /api/v1/invites/mine", authService.Middleware(workspaceService.MyInvites))
	mux.Handle("DELETE /api/v1/invites/{id}", authService.Middleware(workspaceService.RevokeInvite))
	mux.Handle("DELETE /api/v1/workspaces/{id}/members/{user}", authService.Middleware(workspaceService.RemoveMember))
	mux.Handle("GET /api/v1/workspaces/{id}/events", authService.Middleware(workspaceService.Feed))
	mux.Handle("POST /api/v1/workspace-comments", authService.Middleware(workspaceService.CreateComment))
	mux.Handle("GET /api/v1/workspace-comments", authService.Middleware(workspaceService.ListComments))
	mux.Handle("GET /api/v1/change-hints", wsBearer(authService.Middleware(hub.Stream)))

	mux.Handle("GET /api/v1/promotion-sources", authService.Middleware(promotionService.ListSources))
	mux.Handle("POST /api/v1/promotion-sources", authService.Middleware(promotionService.CreateSource))
	mux.Handle("POST /api/v1/promotion-sources/{id}/ingest", authService.Middleware(promotionService.IngestSource))
	mux.Handle("POST /api/v1/promotions/submit", authService.Middleware(promotionService.Submit))
	mux.Handle("GET /api/v1/promotions", authService.Middleware(promotionService.ListFeed))
	mux.Handle("GET /api/v1/promotion-watchlist", authService.Middleware(promotionService.ListWatchlist))
	mux.Handle("POST /api/v1/promotion-watchlist", authService.Middleware(promotionService.CreateWatch))
	mux.Handle("DELETE /api/v1/promotion-watchlist/{id}", authService.Middleware(promotionService.DeleteWatch))

	if s.dist != "" {
		if _, err := os.Stat(s.dist); err == nil {
			mux.Handle("/", spaHandler(s.dist))
		} else {
			mux.Handle("/", spaFS(webassets.FS()))
		}
	} else {
		mux.Handle("/", spaFS(webassets.FS()))
	}

	return httpx.CORS(httpx.Middleware(httpx.Guard(mux)))
}

// adminTelemetry serves the G1 alpha observation report (INH-421).
func (s *Server) adminTelemetry(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, http.StatusForbidden, "admin_required", "观察报表仅管理员可读")
		return
	}
	report, e := telemetry.Build(r.Context(), s.db)
	if e != nil {
		logErr("telemetry report", e, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "报表生成失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 200, report)
}

// wsBearer sits outside the auth middleware and lets WebSocket clients present
// the access token via the access_token query parameter: browsers cannot set
// custom headers on WebSocket connections. Applied only to the change-hints
// stream.
func wsBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if token := r.URL.Query().Get("access_token"); token != "" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// adminOperations serves self-host operations data (R3/INH-543 groundwork).
func (s *Server) adminOperations(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, http.StatusForbidden, "admin_required", "运维页仅管理员可读")
		return
	}
	ops, e := telemetry.Operations(r.Context(), s.db)
	if e != nil {
		logErr("operations snapshot", e, httpx.RequestID(r))
		httpx.Error(w, http.StatusInternalServerError, "internal", "运维快照失败")
		return
	}
	ops["time"] = s.clock.Now().UTC().Format(time.RFC3339)
	ops["request_id"] = httpx.RequestID(r)
	httpx.WriteJSON(w, 200, ops)
}

// promotionNotify writes matched-promotion notifications; the promotion package
// stays decoupled from the notification table via this callback.
func (s *Server) promotionNotify(ctx context.Context, uid, title, body, dedupeKey string) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO notifications (id,user_id,rule_id,provider_account_id,quota_bucket_id,title,body,severity,dedupe_key,status,created_at)
		VALUES ($1,$2,NULL,NULL,NULL,$3,$4,'info',$5,'unread',$6)`,
		httpx.NewID("ntf"), uid, title, body, dedupeKey, s.clock.Now().UTC().Format(time.RFC3339))
	return e
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
	service := &auth.Service{DB: s.db}
	return service.Middleware(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userIDKey, auth.Who(r).UserID)
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

func spaFS(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || (r.Method != "GET" && r.Method != "HEAD") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if p == "sw.js" || p == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if _, e := fs.Stat(root, p); e == nil {
			files.ServeHTTP(w, r)
			return
		}
		raw, e := fs.ReadFile(root, "index.html")
		if e != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(raw))
	})
}
