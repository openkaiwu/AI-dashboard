package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/jobs"
	"aihub.dev/server/internal/workspace"
)

// Service owns the conversation portability HTTP surface and persistence.
type Service struct {
	DB *sql.DB
	// OnShare binds/unbinds a conversation to a workspace (wired to the workspace
	// service at composition; keeps this package free of collaboration imports).
	OnShare func(ctx context.Context, userID, resourceID, workspaceID string) error
}

const importJobKind = "conversation_import"

// --- HTTP: projects ---

func (s *Service) CreateProject(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if httpx.Decode(r, &q) != nil || len(strings.TrimSpace(q.Name)) == 0 || len(q.Name) > 300 || len(q.Description) > 4000 {
		httpx.Error(w, 400, "invalid_input", "检查项目名称与描述")
		return
	}
	id := httpx.NewID("proj")
	_, e := s.DB.ExecContext(r.Context(), `INSERT INTO projects(id,user_id,name,description) VALUES($1,$2,$3,$4)`, id, auth.Who(r).UserID, strings.TrimSpace(q.Name), q.Description)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "创建失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}

func (s *Service) ListProjects(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT p.id,p.name,p.description,
		(SELECT count(*) FROM conversations c WHERE c.project_id=p.id) AS conversations,
		p.created_at,p.updated_at FROM projects p WHERE p.user_id=$1 ORDER BY p.created_at DESC`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, description string
		var conversations int64
		var created, updated time.Time
		if e := rows.Scan(&id, &name, &description, &conversations, &created, &updated); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "name": name, "description": description, "conversations": conversations, "created_at": created, "updated_at": updated})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"projects": out})
}

func (s *Service) DeleteProject(w http.ResponseWriter, r *http.Request) {
	result, e := s.DB.ExecContext(r.Context(), `DELETE FROM projects WHERE id=$1 AND user_id=$2`, r.PathValue("id"), auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "删除失败")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		httpx.Error(w, 404, "not_found", "项目不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// --- HTTP: import ---

// Import accepts a raw source file (chatgpt_export / codex_cli_jsonl / archive),
// stores it as a raw snapshot and enqueues async processing.
func (s *Service) Import(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	switch source {
	case "chatgpt_export", "codex_cli_jsonl", "archive":
	default:
		httpx.Error(w, 400, "invalid_input", "source 必须是 chatgpt_export、codex_cli_jsonl 或 archive")
		return
	}
	raw, e := io.ReadAll(io.LimitReader(r.Body, MaxFileBytes+1))
	if e != nil || len(raw) == 0 {
		httpx.Error(w, 400, "invalid_input", "导入内容为空或不可读")
		return
	}
	if len(raw) > MaxFileBytes {
		httpx.Error(w, 400, "import_too_large", "导入文件超过 900KiB 的 v1 上限")
		return
	}
	if _, e = ParseImport(source, raw); e != nil {
		// Fail fast on structurally invalid input; the async pass re-parses the retained snapshot.
		if errors.Is(e, ErrFormat) || errors.Is(e, ErrTooLarge) || errors.Is(e, ErrTooMany) {
			httpx.Error(w, 400, "invalid_import", "导入格式无效: "+e.Error())
			return
		}
		httpx.Error(w, 503, "unavailable", "导入解析失败")
		return
	}
	fileName := strings.TrimSpace(r.Header.Get("X-File-Name"))
	if len(fileName) > 300 {
		fileName = ""
	}
	who := auth.Who(r)
	importID := httpx.NewID("imp")
	hash := sha256.Sum256(raw)
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO conversation_imports(id,user_id,source_type,file_name,size_bytes) VALUES($1,$2,$3,$4,$5)`,
			importID, who.UserID, source, fileName, len(raw)); e != nil {
			return e
		}
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO conversation_raw_snapshots(id,import_id,user_id,source_type,content,content_hash) VALUES($1,$2,$3,$4,$5,$6)`,
			httpx.NewID("rsnap"), importID, who.UserID, source, string(raw), hex.EncodeToString(hash[:])); e != nil {
			return e
		}
		payload, _ := json.Marshal(map[string]string{"import_id": importID})
		return jobs.Enqueue(r.Context(), tx, importJobKind, payload)
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "导入登记失败")
		return
	}
	httpx.WriteJSON(w, 202, map[string]any{"id": importID, "status": "pending"})
}

func (s *Service) ListImports(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,source_type,file_name,size_bytes,status,conversations_created,conversations_deduplicated,messages_imported,branches_created,error,created_at,completed_at
		FROM conversation_imports WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, e := scanImportRow(rows)
		if e != nil {
			break
		}
		out = append(out, m)
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"imports": out})
}

func (s *Service) ImportStatus(w http.ResponseWriter, r *http.Request) {
	row := s.DB.QueryRowContext(r.Context(), `SELECT id,source_type,file_name,size_bytes,status,conversations_created,conversations_deduplicated,messages_imported,branches_created,error,created_at,completed_at
		FROM conversation_imports WHERE id=$1 AND user_id=$2`, r.PathValue("id"), auth.Who(r).UserID)
	m, e := scanImportRow(row)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "导入不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, m)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanImportRow(row rowScanner) (map[string]any, error) {
	var id, source, status, errText, fileName string
	var size, created, deduped, messages, branches int64
	var createdAt time.Time
	var completed sql.NullTime
	if e := row.Scan(&id, &source, &fileName, &size, &status, &created, &deduped, &messages, &branches, &errText, &createdAt, &completed); e != nil {
		return nil, e
	}
	var completedAt any
	if completed.Valid {
		completedAt = completed.Time
	}
	return map[string]any{"id": id, "source_type": source, "file_name": fileName, "size_bytes": size, "status": status,
		"conversations_created": created, "conversations_deduplicated": deduped, "messages_imported": messages,
		"branches_created": branches, "error": errText, "created_at": createdAt, "completed_at": completedAt}, nil
}

// ImportRaw returns the retained raw snapshot to its owner (provenance/re-import).
func (s *Service) ImportRaw(w http.ResponseWriter, r *http.Request) {
	var content string
	e := s.DB.QueryRowContext(r.Context(), `SELECT s.content FROM conversation_raw_snapshots s
		JOIN conversation_imports i ON i.id=s.import_id WHERE s.import_id=$1 AND s.user_id=$2`,
		r.PathValue("id"), auth.Who(r).UserID).Scan(&content)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "原始导入不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write([]byte(content))
}

// --- HTTP: conversations ---

func (s *Service) ListConversations(w http.ResponseWriter, r *http.Request) {
	projectFilter := strings.TrimSpace(r.URL.Query().Get("project_id"))
	query := `SELECT c.id,c.title,c.provider_slug,c.message_count,c.project_id,p.name,c.created_at,c.updated_at,
		(SELECT count(*) FROM conversation_branches b WHERE b.conversation_id=c.id)
		FROM conversations c LEFT JOIN projects p ON p.id=c.project_id WHERE` + workspace.ReadableScope("c", 1)
	args := []any{auth.Who(r).UserID}
	if projectFilter != "" {
		query += ` AND c.project_id=$2`
		args = append(args, projectFilter)
	}
	query += ` ORDER BY c.updated_at DESC LIMIT 200`
	rows, e := s.DB.QueryContext(r.Context(), query, args...)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, title, slug string
		var messages, branches int64
		var projectID, projectName sql.NullString
		var created, updated time.Time
		if e := rows.Scan(&id, &title, &slug, &messages, &projectID, &projectName, &created, &updated, &branches); e != nil {
			break
		}
		var pid, pname any
		if projectID.Valid {
			pid = projectID.String
		}
		if projectName.Valid {
			pname = projectName.String
		}
		out = append(out, map[string]any{"id": id, "title": title, "provider_slug": slug, "message_count": messages,
			"branch_count": branches, "project_id": pid, "project_name": pname, "created_at": created, "updated_at": updated})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"conversations": out})
}

type branchView struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Messages []messageView `json:"messages"`
}

type messageView struct {
	ID          string     `json:"id"`
	ParentID    *string    `json:"parent_id"`
	Position    int        `json:"position"`
	Role        string     `json:"role"`
	Content     string     `json:"content"`
	SentAt      *time.Time `json:"sent_at"`
	ExternalRef string     `json:"external_ref"`
}

// GetConversation returns the full graph: conversation row, named branches and messages.
func (s *Service) GetConversation(w http.ResponseWriter, r *http.Request) {
	conv, e := s.loadConversation(r.Context(), auth.Who(r).UserID, r.PathValue("id"))
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "会话不存在")
		return
	}
	if e != nil {
		slog.Error("conversation load failed", "error", e.Error())
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, conv)
}

type conversationView struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	ProviderSlug string         `json:"provider_slug"`
	ExternalID   string         `json:"external_id"`
	ProjectID    *string        `json:"project_id"`
	ProjectName  *string        `json:"project_name"`
	MessageCount int            `json:"message_count"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Branches     []branchView   `json:"branches"`
}

func (s *Service) loadConversation(ctx context.Context, userID, id string) (*conversationView, error) {
	var conv conversationView
	var projectID, projectName sql.NullString
	e := s.DB.QueryRowContext(ctx, `SELECT c.id,c.title,c.provider_slug,c.external_id,c.project_id,p.name,c.message_count,c.created_at,c.updated_at
		FROM conversations c LEFT JOIN projects p ON p.id=c.project_id
		WHERE c.id=$1 AND`+workspace.ReadableScope("c", 2), id, userID).
		Scan(&conv.ID, &conv.Title, &conv.ProviderSlug, &conv.ExternalID, &projectID, &projectName, &conv.MessageCount, &conv.CreatedAt, &conv.UpdatedAt)
	if e != nil {
		return nil, e
	}
	if projectID.Valid {
		v := projectID.String
		conv.ProjectID = &v
	}
	if projectName.Valid {
		v := projectName.String
		conv.ProjectName = &v
	}
	brows, e := s.DB.QueryContext(ctx, `SELECT id,name FROM conversation_branches WHERE conversation_id=$1 ORDER BY created_at`, id)
	if e != nil {
		return nil, e
	}
	defer brows.Close()
	branchOrder := []string{}
	branchNames := map[string]string{}
	for brows.Next() {
		var bid, name string
		if e := brows.Scan(&bid, &name); e != nil {
			return nil, e
		}
		branchOrder = append(branchOrder, bid)
		branchNames[bid] = name
		conv.Branches = append(conv.Branches, branchView{ID: bid, Name: name, Messages: []messageView{}})
	}
	if e := brows.Err(); e != nil {
		return nil, e
	}
	mrows, e := s.DB.QueryContext(ctx, `SELECT id,branch_id,parent_id,position,role,content,sent_at,external_ref
		FROM conversation_messages WHERE conversation_id=$1 ORDER BY branch_id,position`, id)
	if e != nil {
		return nil, e
	}
	defer mrows.Close()
	byBranch := map[string][]messageView{}
	for mrows.Next() {
		var mv messageView
		var branchID string
		var parentID sql.NullString
		var sentAt sql.NullTime
		if e := mrows.Scan(&mv.ID, &branchID, &parentID, &mv.Position, &mv.Role, &mv.Content, &sentAt, &mv.ExternalRef); e != nil {
			return nil, e
		}
		if parentID.Valid {
			v := parentID.String
			mv.ParentID = &v
		}
		if sentAt.Valid {
			t := sentAt.Time
			mv.SentAt = &t
		}
		byBranch[branchID] = append(byBranch[branchID], mv)
	}
	if e := mrows.Err(); e != nil {
		return nil, e
	}
	for i := range conv.Branches {
		conv.Branches[i].Messages = byBranch[conv.Branches[i].ID]
	}
	_ = branchNames
	_ = branchOrder
	return &conv, nil
}

func (s *Service) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	result, e := s.DB.ExecContext(r.Context(), `DELETE FROM conversations WHERE id=$1 AND user_id=$2`, r.PathValue("id"), auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "删除失败")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		httpx.Error(w, 404, "not_found", "会话不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// PatchConversation assigns a conversation to a project (or detaches with null)
// and shares/unshares it into a workspace (owner only, wired via OnShare).
func (s *Service) PatchConversation(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ProjectID   *string `json:"project_id"`
		Title       *string `json:"title"`
		WorkspaceID *string `json:"workspace_id"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	who := auth.Who(r)
	if q.Title != nil {
		title := strings.TrimSpace(*q.Title)
		if title == "" || len([]rune(title)) > MaxTitleRunes {
			httpx.Error(w, 400, "invalid_input", "检查标题")
			return
		}
		if _, e := s.DB.ExecContext(r.Context(), `UPDATE conversations SET title=$1,updated_at=now() WHERE id=$2 AND user_id=$3`, title, r.PathValue("id"), who.UserID); e != nil {
			httpx.Error(w, 503, "unavailable", "更新失败")
			return
		}
	}
	if q.ProjectID != nil {
		projectID := strings.TrimSpace(*q.ProjectID)
		if projectID != "" {
			var n int
			if e := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM projects WHERE id=$1 AND user_id=$2`, projectID, who.UserID).Scan(&n); e != nil || n == 0 {
				httpx.Error(w, 400, "invalid_input", "项目不存在")
				return
			}
		}
		var target any
		if projectID != "" {
			target = projectID
		}
		if _, e := s.DB.ExecContext(r.Context(), `UPDATE conversations SET project_id=$1,updated_at=now() WHERE id=$2 AND user_id=$3`, target, r.PathValue("id"), who.UserID); e != nil {
			httpx.Error(w, 503, "unavailable", "更新失败")
			return
		}
	}
	if q.WorkspaceID != nil {
		if s.OnShare == nil {
			httpx.Error(w, 503, "unavailable", "协作未启用")
			return
		}
		if e := s.OnShare(r.Context(), who.UserID, r.PathValue("id"), strings.TrimSpace(*q.WorkspaceID)); e != nil {
			if errors.Is(e, workspace.ErrNotMember) {
				httpx.Error(w, 403, "not_authorized", "需要目标工作区的编辑者或所有者角色")
				return
			}
			httpx.Error(w, 404, "not_found", "会话不存在")
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// ExportConversation renders archive v1 or markdown for one conversation.
func (s *Service) ExportConversation(w http.ResponseWriter, r *http.Request) {
	conv, e := s.loadConversation(r.Context(), auth.Who(r).UserID, r.PathValue("id"))
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "会话不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "导出失败")
		return
	}
	canonical := viewToCanonical(conv)
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "archive"
	}
	switch format {
	case "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", conv.ID+".md"))
		_, _ = w.Write([]byte(BuildMarkdown(&canonical)))
	case "archive":
		out, e := BuildArchive([]Conversation{canonical}, nil)
		if e != nil {
			httpx.Error(w, 503, "unavailable", "导出失败")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", conv.ID+".archive.json"))
		_, _ = w.Write(out)
	default:
		httpx.Error(w, 400, "invalid_input", "format 只支持 archive 或 markdown")
	}
}

// viewToCanonical projects stored rows back into the canonical model for export.
func viewToCanonical(v *conversationView) Conversation {
	c := Conversation{Title: v.Title, ProviderSlug: v.ProviderSlug, ExternalID: v.ExternalID}
	for _, b := range v.Branches {
		nb := Branch{Name: b.Name}
		for _, m := range b.Messages {
			nm := Message{Role: m.Role, Content: m.Content, SentAt: m.SentAt, ExternalRef: m.ExternalRef}
			if m.ParentID != nil {
				// Point back at the position of the referenced message inside this branch.
				for i, prev := range b.Messages {
					if prev.ID == *m.ParentID {
						idx := i
						nm.ParentIdx = &idx
						break
					}
				}
			}
			nb.Messages = append(nb.Messages, nm)
		}
		c.Branches = append(c.Branches, nb)
	}
	return c
}

// --- async worker ---

// ProcessImports claims queued import jobs; the binary runs it on a ticker and
// tests call it directly. Parse failures mark the import failed; transient DB
// errors leave the job for lease expiry (retry, up to the attempt cap).
func ProcessImports(ctx context.Context, database *sql.DB) {
	for {
		job, e := jobs.Claim(ctx, database, 5*time.Minute)
		if errors.Is(e, sql.ErrNoRows) {
			return
		}
		if e != nil {
			return
		}
		var payload struct {
			ImportID string `json:"import_id"`
		}
		if e := json.Unmarshal(job.Payload, &payload); e != nil || payload.ImportID == "" {
			_, _ = jobs.Ack(ctx, database, job)
			continue
		}
		reason := runImport(ctx, database, payload.ImportID, job.Attempts)
		if reason == "" {
			_, _ = jobs.Ack(ctx, database, job)
			continue
		}
		if job.Attempts >= 3 {
			_, _ = database.ExecContext(ctx, `UPDATE conversation_imports SET status='failed',error=$1,completed_at=now() WHERE id=$2`, reason, payload.ImportID)
			_, _ = jobs.Ack(ctx, database, job)
			continue
		}
		// Un-acked: lease expiry re-releases the job for retry.
	}
}

// runImport parses the retained raw snapshot and persists the batch.
// Empty return means success; non-empty is a stable failure reason.
func runImport(ctx context.Context, database *sql.DB, importID string, attempts int) string {
	var userID, source, content string
	e := database.QueryRowContext(ctx, `SELECT i.user_id,i.source_type,s.content FROM conversation_imports i
		JOIN conversation_raw_snapshots s ON s.import_id=i.id WHERE i.id=$1`, importID).Scan(&userID, &source, &content)
	if e != nil {
		return "import record missing"
	}
	if _, e := database.ExecContext(ctx, `UPDATE conversation_imports SET status='processing' WHERE id=$1`, importID); e != nil {
		return "mark processing failed"
	}
	im, pe := ParseImport(source, []byte(content))
	if pe != nil {
		msg := "导入格式无效"
		if errors.Is(pe, ErrTooLarge) {
			msg = "导入文件过大"
		} else if errors.Is(pe, ErrTooMany) {
			msg = "导入内容超出数量上限"
		}
		_, _ = database.ExecContext(ctx, `UPDATE conversation_imports SET status='failed',error=$1,completed_at=now() WHERE id=$2`, msg, importID)
		return ""
	}
	created, deduped, messages, branches, pe := persistImport(ctx, database, userID, im)
	if pe != nil {
		if attempts >= 3 {
			_, _ = database.ExecContext(ctx, `UPDATE conversation_imports SET status='failed',error='写入失败',completed_at=now() WHERE id=$1`, importID)
			return ""
		}
		return "persist failed"
	}
	_, pe = database.ExecContext(ctx, `UPDATE conversation_imports SET status='completed',conversations_created=$1,conversations_deduplicated=$2,messages_imported=$3,branches_created=$4,completed_at=now() WHERE id=$5`,
		created, deduped, messages, branches, importID)
	if pe != nil {
		return "finalize failed"
	}
	return ""
}

// persistImport applies one parsed batch: projects first, then conversations with
// dedup (identical content) or branch-append (same external id, changed content).
func persistImport(ctx context.Context, database *sql.DB, userID string, im *Import) (created, deduped, messages, branches int, err error) {
	e := db.Tx(ctx, database, func(tx *sql.Tx) error {
		created, deduped, messages, branches = 0, 0, 0, 0
		projectIDs := map[string]string{}
		ensure := func(name string) (string, error) {
			if pid, ok := projectIDs[name]; ok {
				return pid, nil
			}
			pid, e := ensureProject(ctx, tx, userID, ArchiveProject{Name: name})
			if e != nil {
				return "", e
			}
			projectIDs[name] = pid
			return pid, nil
		}
		for _, p := range im.Projects {
			id, e := ensureProject(ctx, tx, userID, p)
			if e != nil {
				return e
			}
			if p.Name != "" {
				projectIDs[p.Name] = id
			}
		}
		for i := range im.Conversations {
			c := &im.Conversations[i]
			var projectID any
			if c.ProjectName != "" {
				pid, e := ensure(c.ProjectName)
				if e != nil {
					return e
				}
				projectID = pid
			}
			var existingID, existingHash string
			e := tx.QueryRowContext(ctx, `SELECT id,content_hash FROM conversations WHERE user_id=$1 AND provider_slug=$2 AND dedup_key=$3`,
				userID, c.ProviderSlug, c.DedupKey).Scan(&existingID, &existingHash)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if e == nil && existingHash == c.ContentHash {
				deduped++
				continue
			}
			if e == nil {
				// Same external identity, changed content: append new branches, never rewrite history.
				addedBranches, addedMessages, e := appendBranches(ctx, tx, userID, existingID, c)
				if e != nil {
					return e
				}
				branches += addedBranches
				messages += addedMessages
				combined := sha256.Sum256([]byte(existingHash + c.ContentHash))
				if _, e := tx.ExecContext(ctx, `UPDATE conversations SET content_hash=$1,message_count=message_count+$2,updated_at=now() WHERE id=$3`,
					hex.EncodeToString(combined[:]), addedMessages, existingID); e != nil {
					return e
				}
				continue
			}
			convID := httpx.NewID("conv")
			if _, e := tx.ExecContext(ctx, `INSERT INTO conversations(id,user_id,project_id,title,provider_slug,external_id,dedup_key,content_hash,message_count,imported_via) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				convID, userID, projectID, c.Title, c.ProviderSlug, c.ExternalID, c.DedupKey, c.ContentHash, c.totalMessages(), im.SourceType); e != nil {
				return e
			}
			created++
			addedBranches, addedMessages, e := insertBranches(ctx, tx, userID, convID, c)
			if e != nil {
				return e
			}
			branches += addedBranches
			messages += addedMessages
		}
		return nil
	})
	if e != nil {
		return 0, 0, 0, 0, e
	}
	return created, deduped, messages, branches, nil
}

// insertBranches writes branches and their message chains; parent_index maps to
// the message at that position inside the same branch (validated backwards only).
func insertBranches(ctx context.Context, tx *sql.Tx, userID, convID string, c *Conversation) (int, int, error) {
	branches, messages := 0, 0
	for _, b := range c.Branches {
		branchID := httpx.NewID("brn")
		if _, e := tx.ExecContext(ctx, `INSERT INTO conversation_branches(id,conversation_id,user_id,name) VALUES($1,$2,$3,$4)`, branchID, convID, userID, b.Name); e != nil {
			return 0, 0, e
		}
		branches++
		ids := make([]string, 0, len(b.Messages))
		for pos, m := range b.Messages {
			msgID := httpx.NewID("msg")
			var parentRef any
			if m.ParentIdx != nil && *m.ParentIdx >= 0 && *m.ParentIdx < len(ids) {
				parentRef = ids[*m.ParentIdx]
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO conversation_messages(id,conversation_id,user_id,branch_id,parent_id,position,role,content,sent_at,external_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				msgID, convID, userID, branchID, parentRef, pos, m.Role, m.Content, m.SentAt, m.ExternalRef); e != nil {
				return 0, 0, e
			}
			ids = append(ids, msgID)
			messages++
		}
	}
	return branches, messages, nil
}

// appendBranches adds changed-content branches to an existing conversation with
// collision-safe branch names; history rows are never rewritten.
func appendBranches(ctx context.Context, tx *sql.Tx, userID, convID string, c *Conversation) (int, int, error) {
	for i := range c.Branches {
		name := c.Branches[i].Name
		for attempt := 1; ; attempt++ {
			var n int
			if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM conversation_branches WHERE conversation_id=$1 AND name=$2`, convID, name).Scan(&n); e != nil {
				return 0, 0, e
			}
			if n == 0 {
				break
			}
			if attempt > MaxBranchesPerConv {
				return 0, 0, errors.New("branch name exhaustion")
			}
			name = fmt.Sprintf("%s-%d", c.Branches[i].Name, attempt+1)
		}
		c.Branches[i].Name = name
	}
	return insertBranches(ctx, tx, userID, convID, c)
}

func ensureProject(ctx context.Context, tx *sql.Tx, userID string, p ArchiveProject) (string, error) {
	var id string
	e := tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE user_id=$1 AND ((external_id<>'' AND external_source=$2 AND external_id=$3) OR (external_id='' AND name=$4)) LIMIT 1`,
		userID, p.ExternalSource, p.ExternalID, p.Name).Scan(&id)
	if e == nil {
		return id, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	id = httpx.NewID("proj")
	_, e = tx.ExecContext(ctx, `INSERT INTO projects(id,user_id,name,description,external_source,external_id) VALUES($1,$2,$3,$4,$5,$6)`,
		id, userID, p.Name, p.Description, p.ExternalSource, p.ExternalID)
	return id, e
}
