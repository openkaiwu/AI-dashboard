// Package workspace owns collaboration v1 (M5): workspaces, membership/ACL,
// invites, workspace comments and SSE change hints. Workspaces scope shared
// canonical assets only; provider accounts and quotas stay strictly personal.
package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aihub.dev/server/internal/audit"
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
)

// Service owns the workspace HTTP surface.
type Service struct {
	DB  *sql.DB
	hub *Hub
}

// UseHub attaches the process-wide change-hint hub; it must be called once at wiring.
func (s *Service) UseHub(hub *Hub) { s.hub = hub }

// RowQueryer is satisfied by *sql.DB and *sql.Tx.
type RowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var ErrNotMember = errors.New("not a workspace member")

// RoleFor returns the caller's role in the workspace; sql.ErrNoRows when absent.
func RoleFor(ctx context.Context, q RowQueryer, workspaceID, userID string) (string, error) {
	var role string
	e := q.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, userID).Scan(&role)
	return role, e
}

// CanComment: editors and owners may annotate shared resources.
func CanComment(role string) bool { return role == "owner" || role == "editor" }

// ReadableScope is the shared SQL fragment expressing "visible to the caller
// through workspace membership". The membership table's ownership stays inside
// this package; conversation/config embed the fragment via this helper only.
// param is the 1-based placeholder index of the user id in the outer query.
func ReadableScope(alias string, param int) string {
	return fmt.Sprintf(` (%s.user_id=$%d OR EXISTS (SELECT 1 FROM workspace_members m WHERE m.workspace_id=%s.workspace_id AND m.user_id=$%d))`, alias, param, alias, param)
}

func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name string `json:"name"`
	}
	who := auth.Who(r)
	if httpx.Decode(r, &q) != nil || len(strings.TrimSpace(q.Name)) == 0 || len(q.Name) > 200 {
		httpx.Error(w, 400, "invalid_input", "检查工作区名称")
		return
	}
	id := httpx.NewID("ws")
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO workspaces(id,name,owner_user_id) VALUES($1,$2,$3)`, id, strings.TrimSpace(q.Name), who.UserID); e != nil {
			return e
		}
		_, e := tx.ExecContext(r.Context(), `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'owner')`, id, who.UserID)
		if e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, who.UserID, who.DeviceID, "workspace_created")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "创建失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}

func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT w.id,w.name,w.owner_user_id,m.role,w.created_at,
		(SELECT count(*) FROM workspace_members x WHERE x.workspace_id=w.id) AS members
		FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id AND m.user_id=$1 ORDER BY w.created_at DESC`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, owner, role string
		var members int64
		var created time.Time
		if e := rows.Scan(&id, &name, &owner, &role, &created, &members); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "name": name, "owner": owner == auth.Who(r).UserID, "role": role, "members": members, "created_at": created})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"workspaces": out})
}

// SetResourceWorkspace is the frozen sharing primitive: only the resource owner
// may bind or unbind it, and binding requires owner/editor in the workspace.
// Used by conversation and config PATCH handlers.
func (s *Service) SetResourceWorkspace(ctx context.Context, userID, resourceOwner, workspaceID, resourceType, resourceID string) error {
	return db.Tx(ctx, s.DB, func(tx *sql.Tx) error {
		if workspaceID != "" {
			role, e := RoleFor(ctx, tx, workspaceID, userID)
			if errors.Is(e, sql.ErrNoRows) {
				return ErrNotMember
			}
			if e != nil {
				return e
			}
			if !CanComment(role) {
				return ErrNotMember
			}
		}
		var n int
		table := map[string]string{"conversation": "conversations", "config_asset": "config_assets"}[resourceType]
		if table == "" {
			return errors.New("unknown resource type")
		}
		if e := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE id=$1 AND user_id=$2`, table), resourceID, resourceOwner).Scan(&n); e != nil || n == 0 {
			return sql.ErrNoRows
		}
		var target any
		if workspaceID != "" {
			target = workspaceID
		}
		if _, e := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET workspace_id=$1 WHERE id=$2 AND user_id=$3`, table), target, resourceID, resourceOwner); e != nil {
			return e
		}
		return audit.Record(ctx, tx, userID, "", "resource_shared")
	})
}

// --- members & invites (INH-503) ---

func (s *Service) Members(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("id")
	if _, e := RoleFor(r.Context(), s.DB, workspaceID, auth.Who(r).UserID); e != nil {
		httpx.Error(w, 404, "not_found", "工作区不存在")
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT m.user_id,m.role,u.email,m.created_at FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE m.workspace_id=$1 ORDER BY m.created_at`, workspaceID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var uid, role, email string
		var created time.Time
		if e := rows.Scan(&uid, &role, &email, &created); e != nil {
			break
		}
		out = append(out, map[string]any{"user_id": uid, "role": role, "email": email, "joined_at": created})
	}
	httpx.WriteJSON(w, 200, map[string]any{"members": out})
}

func (s *Service) ListInvites(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("id")
	if role, e := RoleFor(r.Context(), s.DB, workspaceID, auth.Who(r).UserID); e != nil || role != "owner" {
		httpx.Error(w, 404, "not_found", "工作区不存在")
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,email,role,status,created_at FROM workspace_invites WHERE workspace_id=$1 ORDER BY created_at DESC`, workspaceID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, email, role, status string
		var created time.Time
		if e := rows.Scan(&id, &email, &role, &status, &created); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "email": email, "role": role, "status": status, "created_at": created})
	}
	httpx.WriteJSON(w, 200, map[string]any{"invites": out})
}

func (s *Service) Invite(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("id")
	who := auth.Who(r)
	var q struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if httpx.Decode(r, &q) != nil || (q.Role != "editor" && q.Role != "viewer") {
		httpx.Error(w, 400, "invalid_input", "role 只能是 editor 或 viewer")
		return
	}
	q.Email = strings.ToLower(strings.TrimSpace(q.Email))
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		role, e := RoleFor(r.Context(), tx, workspaceID, who.UserID)
		if e != nil {
			return sql.ErrNoRows
		}
		if role != "owner" {
			return errors.New("owner required")
		}
		id := httpx.NewID("inv")
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO workspace_invites(id,workspace_id,email,role,created_by) VALUES($1,$2,$3,$4,$5)`, id, workspaceID, q.Email, q.Role, who.UserID); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, who.UserID, who.DeviceID, "workspace_invited")
	})
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			httpx.Error(w, 404, "not_found", "只有工作区所有者可以邀请")
			return
		}
		if strings.Contains(e.Error(), "23505") {
			httpx.Error(w, 409, "invite_exists", "该邮箱已有待处理邀请")
			return
		}
		httpx.Error(w, 503, "unavailable", "邀请失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]bool{"ok": true})
}

// AcceptInvite binds the current account to a pending invite addressed to its email.
func (s *Service) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	who := auth.Who(r)
	var workspaceID, role string
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		var email, status string
		if e := tx.QueryRowContext(r.Context(), `SELECT i.workspace_id,i.role,i.email,i.status FROM workspace_invites i
			JOIN users u ON u.email=i.email WHERE i.id=$1 AND u.id=$2 FOR UPDATE`, r.PathValue("id"), who.UserID).Scan(&workspaceID, &role, &email, &status); e != nil {
			return sql.ErrNoRows
		}
		if status != "pending" {
			return errors.New("invite not pending")
		}
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT (workspace_id,user_id) DO UPDATE SET role=EXCLUDED.role`, workspaceID, who.UserID, role); e != nil {
			return e
		}
		if _, e := tx.ExecContext(r.Context(), `UPDATE workspace_invites SET status='accepted',accepted_at=now() WHERE id=$1`, r.PathValue("id")); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, who.UserID, who.DeviceID, "workspace_member_added")
	})
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			httpx.Error(w, 404, "not_found", "邀请不存在或不属于当前账户")
			return
		}
		httpx.Error(w, 503, "unavailable", "接受邀请失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"workspace_id": workspaceID, "role": role, "ok": true})
}

// MyInvites lists pending invites addressed to the current account's email.
func (s *Service) MyInvites(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT i.id,i.workspace_id,w.name,i.role,i.created_at FROM workspace_invites i
		JOIN workspaces w ON w.id=i.workspace_id
		JOIN users u ON u.email=i.email
		WHERE u.id=$1 AND i.status='pending' ORDER BY i.created_at DESC`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, workspaceID, name, role string
		var created time.Time
		if e := rows.Scan(&id, &workspaceID, &name, &role, &created); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "workspace_id": workspaceID, "workspace_name": name, "role": role, "created_at": created})
	}
	httpx.WriteJSON(w, 200, map[string]any{"invites": out})
}

func (s *Service) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	who := auth.Who(r)
	result, e := s.DB.ExecContext(r.Context(), `UPDATE workspace_invites SET status='revoked' WHERE id=$1
		AND status='pending' AND workspace_id IN (SELECT workspace_id FROM workspace_members WHERE user_id=$2 AND role='owner')`,
		r.PathValue("id"), who.UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "撤销失败")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		httpx.Error(w, 404, "not_found", "邀请不存在或已处理")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// RemoveMember revokes access immediately; the owner cannot be removed here.
func (s *Service) RemoveMember(w http.ResponseWriter, r *http.Request) {
	workspaceID, member := r.PathValue("id"), r.PathValue("user")
	who := auth.Who(r)
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		role, e := RoleFor(r.Context(), tx, workspaceID, who.UserID)
		if e != nil || role != "owner" {
			return errors.New("owner required")
		}
		result, e := tx.ExecContext(r.Context(), `DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2 AND role<>'owner'`, workspaceID, member)
		if e != nil {
			return e
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		return audit.Record(r.Context(), tx, who.UserID, who.DeviceID, "workspace_member_removed")
	})
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			httpx.Error(w, 404, "not_found", "成员不存在")
			return
		}
		httpx.Error(w, 403, "owner_required", "只有工作区所有者可以移除成员")
		return
	}
	s.hub.PublishToWorkspace(r.Context(), s.DB, workspaceID, "member_removed", map[string]string{"user": member})
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// --- workspace comments (INH-509, partial: mentions only, branch events deferred) ---

func (s *Service) CreateComment(w http.ResponseWriter, r *http.Request) {
	who := auth.Who(r)
	var q struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Body       string `json:"body"`
		Mention    string `json:"mention_user_id"`
	}
	if httpx.Decode(r, &q) != nil || (q.TargetType != "conversation" && q.TargetType != "config_asset") {
		httpx.Error(w, 400, "invalid_input", "target_type 只能是 conversation 或 config_asset")
		return
	}
	q.Body = strings.TrimSpace(q.Body)
	if q.Body == "" || len(q.Body) > 8000 {
		httpx.Error(w, 400, "invalid_input", "评论内容为空或过长")
		return
	}
	workspaceID, e := s.resourceWorkspace(r.Context(), q.TargetType, q.TargetID)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "资源未共享到任何工作区")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	role, e := RoleFor(r.Context(), s.DB, workspaceID, who.UserID)
	if errors.Is(e, sql.ErrNoRows) || !CanComment(role) {
		httpx.Error(w, 403, "not_authorized", "viewer 不能评论")
		return
	}
	var mention any
	if q.Mention != "" {
		var n int
		if e := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, q.Mention).Scan(&n); e != nil || n == 0 {
			httpx.Error(w, 400, "invalid_input", "被提及者不是该工作区成员")
			return
		}
		mention = q.Mention
	}
	id := httpx.NewID("cmt")
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO workspace_comments(id,workspace_id,user_id,target_type,target_id,body,mention_user_id) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			id, workspaceID, who.UserID, q.TargetType, q.TargetID, q.Body, mention); e != nil {
			return e
		}
		return audit.Record(r.Context(), tx, who.UserID, who.DeviceID, "workspace_comment")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "评论失败")
		return
	}
	s.hub.PublishToWorkspace(r.Context(), s.DB, workspaceID, "comment", map[string]string{"target_type": q.TargetType, "target_id": q.TargetID, "by": who.UserID})
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}

func (s *Service) ListComments(w http.ResponseWriter, r *http.Request) {
	targetType := r.URL.Query().Get("target_type")
	targetID := r.URL.Query().Get("target_id")
	if targetType == "" || targetID == "" {
		httpx.Error(w, 400, "invalid_input", "缺少 target_type 或 target_id")
		return
	}
	workspaceID, e := s.resourceWorkspace(r.Context(), targetType, targetID)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "资源未共享到任何工作区")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if _, e := RoleFor(r.Context(), s.DB, workspaceID, auth.Who(r).UserID); e != nil {
		httpx.Error(w, 404, "not_found", "资源未共享到任何工作区")
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT c.id,c.user_id,u.email,c.body,c.mention_user_id,c.created_at FROM workspace_comments c
		JOIN users u ON u.id=c.user_id WHERE c.workspace_id=$1 AND c.target_type=$2 AND c.target_id=$3 ORDER BY c.created_at LIMIT 500`,
		workspaceID, targetType, targetID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, uid, email, body string
		var mention sql.NullString
		var created time.Time
		if e := rows.Scan(&id, &uid, &email, &body, &mention, &created); e != nil {
			break
		}
		var m any
		if mention.Valid {
			m = mention.String
		}
		out = append(out, map[string]any{"id": id, "user_id": uid, "email": email, "body": body, "mention_user_id": m, "created_at": created})
	}
	httpx.WriteJSON(w, 200, map[string]any{"comments": out})
}

// resourceWorkspace returns the workspace a shared resource belongs to.
func (s *Service) resourceWorkspace(ctx context.Context, targetType, targetID string) (string, error) {
	table := map[string]string{"conversation": "conversations", "config_asset": "config_assets"}[targetType]
	if table == "" {
		return "", sql.ErrNoRows
	}
	var ws sql.NullString
	e := s.DB.QueryRowContext(ctx, fmt.Sprintf(`SELECT workspace_id FROM %s WHERE id=$1`, table), targetID).Scan(&ws)
	if e != nil {
		return "", e
	}
	if !ws.Valid || ws.String == "" {
		return "", sql.ErrNoRows
	}
	return ws.String, nil
}
