package config

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"aihub.dev/server/internal/workspace"
)

// Service owns the portable config HTTP surface (M4).
type Service struct {
	DB *sql.DB
	// OnShare binds/unbinds a config asset to a workspace (composition wiring).
	OnShare func(ctx context.Context, userID, resourceID, workspaceID string) error
}

// ScanEntry is one bridge-side config discovery: metadata and secret key NAMES only.
type ScanEntry struct {
	Path           string     `json:"path"`
	Platform       string     `json:"platform"`
	SizeBytes      int64      `json:"size_bytes"`
	ModifiedAt     *time.Time `json:"modified_at,omitempty"`
	SecretKeys     []string   `json:"secret_keys"`
	DetectedFormat string     `json:"detected_format"`
}

// StoreScan upserts bridge discovery records; called from the bridge-token
// endpoint mounted in the connector package. Values are never transported.
func StoreScan(ctx context.Context, database *sql.DB, userID, deviceID string, entries []ScanEntry) error {
	return db.Tx(ctx, database, func(tx *sql.Tx) error {
		for _, entry := range entries {
			if entry.Path == "" || len(entry.Path) > 1024 || len(entry.Platform) > 60 || len(entry.DetectedFormat) > 60 || len(entry.SecretKeys) > 64 {
				continue
			}
			keys := strings.Join(entry.SecretKeys, ",")
			if len(keys) > 2000 {
				keys = keys[:2000]
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO config_discoveries(id,user_id,device_id,path,platform,size_bytes,modified_at,secret_keys,detected_format)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
				ON CONFLICT (device_id,path) DO UPDATE SET platform=EXCLUDED.platform,size_bytes=EXCLUDED.size_bytes,
				modified_at=EXCLUDED.modified_at,secret_keys=EXCLUDED.secret_keys,detected_format=EXCLUDED.detected_format,updated_at=now()`,
				httpx.NewID("cdsc"), userID, deviceID, entry.Path, entry.Platform, entry.SizeBytes, entry.ModifiedAt, keys, entry.DetectedFormat); e != nil {
				return e
			}
		}
		return nil
	})
}

func (s *Service) ListDiscoveries(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT d.id,d.path,d.platform,d.size_bytes,d.modified_at,d.secret_keys,d.detected_format,d.status,d.device_id
		FROM config_discoveries d WHERE d.user_id=$1 ORDER BY d.created_at DESC LIMIT 200`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, path, platform, keys, format, status, deviceID string
		var size int64
		var modified sql.NullTime
		if e := rows.Scan(&id, &path, &platform, &size, &modified, &keys, &format, &status, &deviceID); e != nil {
			break
		}
		var modifiedAt any
		if modified.Valid {
			modifiedAt = modified.Time
		}
		var keyList []string
		if keys != "" {
			keyList = strings.Split(keys, ",")
		}
		out = append(out, map[string]any{"id": id, "path": path, "platform": platform, "size_bytes": size,
			"modified_at": modifiedAt, "secret_keys": keyList, "detected_format": format, "status": status, "device_id": deviceID})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"discoveries": out})
}

func hashContent(content map[string]any) string {
	// json.Marshal sorts map keys, giving a stable canonical hash.
	raw, _ := json.Marshal(content)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name           string         `json:"name"`
		Kind           string         `json:"kind"`
		SourcePlatform string         `json:"source_platform"`
		Content        map[string]any `json:"content"`
	}
	if httpx.Decode(r, &q) != nil || len(strings.TrimSpace(q.Name)) == 0 || len([]rune(q.Name)) > MaxAssetNameRunes {
		httpx.Error(w, 400, "invalid_input", "检查名称")
		return
	}
	if q.Content == nil {
		httpx.Error(w, 400, "invalid_input", "内容不能为空")
		return
	}
	loss := []LossEntry{}
	contentAny := ClassifySecrets(q.Content, "", &loss)
	content, ok := contentAny.(map[string]any)
	if !ok {
		httpx.Error(w, 400, "invalid_input", "内容必须是对象")
		return
	}
	if e := ValidateContent(q.Kind, content); e != nil {
		if errors.Is(e, ErrUnknownKind) {
			httpx.Error(w, 400, "invalid_input", "kind 必须是 mcp_server、prompt_template 或 agent_profile")
			return
		}
		httpx.Error(w, 400, "invalid_input", "内容不符合规范: "+e.Error())
		return
	}
	if q.SourcePlatform == "" {
		q.SourcePlatform = "canonical"
	}
	id := httpx.NewID("cfg")
	e := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO config_assets(id,user_id,name,kind,source_platform,latest_version) VALUES($1,$2,$3,$4,$5,1)`,
			id, auth.Who(r).UserID, strings.TrimSpace(q.Name), q.Kind, q.SourcePlatform); e != nil {
			return e
		}
		return insertVersion(r.Context(), tx, id, auth.Who(r).UserID, 1, content, loss, "manual")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "创建失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}

func insertVersion(ctx context.Context, tx *sql.Tx, assetID, userID string, version int, content map[string]any, loss []LossEntry, createdBy string) error {
	raw, e := json.Marshal(content)
	if e != nil {
		return e
	}
	lossRaw, e := json.Marshal(loss)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO config_versions(id,asset_id,user_id,version,content,content_hash,loss_report,created_by) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7::jsonb,$8)`,
		httpx.NewID("cfv"), assetID, userID, version, string(raw), hashContent(content), string(lossRaw), createdBy)
	return e
}

func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	query := `SELECT a.id,a.name,a.kind,a.source_platform,a.latest_version,a.created_at,a.updated_at,
		(SELECT count(*) FROM config_bindings b WHERE b.asset_id=a.id)
		FROM config_assets a WHERE` + workspace.ReadableScope("a", 1)
	args := []any{auth.Who(r).UserID}
	if kind != "" {
		query += ` AND a.kind=$2`
		args = append(args, kind)
	}
	query += ` ORDER BY a.updated_at DESC LIMIT 200`
	rows, e := s.DB.QueryContext(r.Context(), query, args...)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, assetKind, source string
		var latest int
		var created, updated time.Time
		var bindings int64
		if e := rows.Scan(&id, &name, &assetKind, &source, &latest, &created, &updated, &bindings); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "name": name, "kind": assetKind, "source_platform": source,
			"latest_version": latest, "bindings": bindings, "created_at": created, "updated_at": updated})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"assets": out})
}

func (s *Service) Get(w http.ResponseWriter, r *http.Request) {
	assetID, userID := r.PathValue("id"), auth.Who(r).UserID
	var name, kind, source string
	var latest int
	var created, updated time.Time
	e := s.DB.QueryRowContext(r.Context(), `SELECT name,kind,source_platform,latest_version,created_at,updated_at FROM config_assets WHERE id=$1 AND`+workspace.ReadableScope("config_assets", 2), assetID, userID).
		Scan(&name, &kind, &source, &latest, &created, &updated)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "配置不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	vrows, e := s.DB.QueryContext(r.Context(), `SELECT id,version,content_hash,created_by,created_at,loss_report FROM config_versions WHERE asset_id=$1 AND user_id=$2 ORDER BY version DESC`, assetID, userID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer vrows.Close()
	versions := []map[string]any{}
	var latestContent map[string]any
	for vrows.Next() {
		var vid, hash, createdBy string
		var version int
		var createdAt time.Time
		var lossRaw []byte
		if e := vrows.Scan(&vid, &version, &hash, &createdBy, &createdAt, &lossRaw); e != nil {
			break
		}
		loss := []any{}
		_ = json.Unmarshal(lossRaw, &loss)
		versions = append(versions, map[string]any{"id": vid, "version": version, "content_hash": hash,
			"created_by": createdBy, "created_at": createdAt, "loss_report": loss})
	}
	if e := vrows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	var latestRaw []byte
	if e := s.DB.QueryRowContext(r.Context(), `SELECT content FROM config_versions WHERE asset_id=$1 AND version=$2`, assetID, latest).Scan(&latestRaw); e != nil && !errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if latestRaw != nil {
		latestContent = map[string]any{}
		if e := json.Unmarshal(latestRaw, &latestContent); e != nil {
			httpx.Error(w, 503, "unavailable", "读取失败")
			return
		}
	}
	brows, e := s.DB.QueryContext(r.Context(), `SELECT id,target_platform,target_path,last_applied_version,status,created_at FROM config_bindings WHERE asset_id=$1 AND user_id=$2 ORDER BY created_at`, assetID, userID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer brows.Close()
	bindings := []map[string]any{}
	for brows.Next() {
		var bid, platform, path, status string
		var applied int
		var createdAt time.Time
		if e := brows.Scan(&bid, &platform, &path, &applied, &status, &createdAt); e != nil {
			break
		}
		bindings = append(bindings, map[string]any{"id": bid, "target_platform": platform, "target_path": path,
			"last_applied_version": applied, "status": status, "created_at": createdAt})
	}
	if e := brows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"asset": map[string]any{"id": assetID, "name": name, "kind": kind,
		"source_platform": source, "latest_version": latest, "created_at": created, "updated_at": updated},
		"versions": versions, "latest_content": latestContent, "bindings": bindings})
}

// Patch renames an asset (owner) and shares/unshares it into a workspace.
func (s *Service) Patch(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name        string `json:"name"`
		WorkspaceID *string `json:"workspace_id"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	if strings.TrimSpace(q.Name) != "" && len([]rune(q.Name)) <= MaxAssetNameRunes {
		result, e := s.DB.ExecContext(r.Context(), `UPDATE config_assets SET name=$1,updated_at=now() WHERE id=$2 AND user_id=$3`, strings.TrimSpace(q.Name), r.PathValue("id"), auth.Who(r).UserID)
		if e != nil {
			httpx.Error(w, 503, "unavailable", "更新失败")
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			httpx.Error(w, 404, "not_found", "配置不存在")
			return
		}
	}
	if q.WorkspaceID != nil {
		if s.OnShare == nil {
			httpx.Error(w, 503, "unavailable", "协作未启用")
			return
		}
		if e := s.OnShare(r.Context(), auth.Who(r).UserID, r.PathValue("id"), strings.TrimSpace(*q.WorkspaceID)); e != nil {
			if errors.Is(e, workspace.ErrNotMember) {
				httpx.Error(w, 403, "not_authorized", "需要目标工作区的编辑者或所有者角色")
				return
			}
			httpx.Error(w, 404, "not_found", "配置不存在")
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Service) Delete(w http.ResponseWriter, r *http.Request) {
	result, e := s.DB.ExecContext(r.Context(), `DELETE FROM config_assets WHERE id=$1 AND user_id=$2`, r.PathValue("id"), auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "删除失败")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		httpx.Error(w, 404, "not_found", "配置不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// AddVersion appends a new version; history is immutable by contract.
func (s *Service) AddVersion(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Content map[string]any `json:"content"`
	}
	assetID, userID := r.PathValue("id"), auth.Who(r).UserID
	if httpx.Decode(r, &q) != nil || q.Content == nil {
		httpx.Error(w, 400, "invalid_input", "内容不能为空")
		return
	}
	var kind string
	var latest int
	e := s.DB.QueryRowContext(r.Context(), `SELECT kind,latest_version FROM config_assets WHERE id=$1 AND user_id=$2`, assetID, userID).Scan(&kind, &latest)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "配置不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if latest >= MaxVersionsPerAsset {
		httpx.Error(w, 400, "invalid_input", "版本数量已达上限")
		return
	}
	loss := []LossEntry{}
	contentAny := ClassifySecrets(q.Content, "", &loss)
	content, ok := contentAny.(map[string]any)
	if !ok {
		httpx.Error(w, 400, "invalid_input", "内容必须是对象")
		return
	}
	if e := ValidateContent(kind, content); e != nil {
		httpx.Error(w, 400, "invalid_input", "内容不符合规范: "+e.Error())
		return
	}
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `UPDATE config_assets SET latest_version=$1,updated_at=now() WHERE id=$2`, latest+1, assetID); e != nil {
			return e
		}
		return insertVersion(r.Context(), tx, assetID, userID, latest+1, content, loss, "manual")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "写入失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"version": latest + 1, "ok": true})
}

func loadVersionContent(ctx context.Context, s *Service, assetID, userID string, version int) (map[string]any, error) {
	var raw []byte
	e := s.DB.QueryRowContext(ctx, `SELECT content FROM config_versions WHERE asset_id=$1 AND user_id=$2 AND version=$3`, assetID, userID, version).Scan(&raw)
	if e != nil {
		return nil, e
	}
	content := map[string]any{}
	if e := json.Unmarshal(raw, &content); e != nil {
		return nil, e
	}
	return content, nil
}

// Diff returns the semantic difference between two versions.
func (s *Service) Diff(w http.ResponseWriter, r *http.Request) {
	assetID, userID := r.PathValue("id"), auth.Who(r).UserID
	var fromV, toV int
	if _, e := fmt.Sscan(r.URL.Query().Get("from"), &fromV); e != nil {
		httpx.Error(w, 400, "invalid_input", "from 版本无效")
		return
	}
	if _, e := fmt.Sscan(r.URL.Query().Get("to"), &toV); e != nil {
		httpx.Error(w, 400, "invalid_input", "to 版本无效")
		return
	}
	from, e := loadVersionContent(r.Context(), s, assetID, userID, fromV)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "源版本不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	to, e := loadVersionContent(r.Context(), s, assetID, userID, toV)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "目标版本不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"diff": Diff(from, to)})
}

// Rollback appends a new version with an older version's content; history stays.
func (s *Service) Rollback(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Version int `json:"version"`
	}
	assetID, userID := r.PathValue("id"), auth.Who(r).UserID
	if httpx.Decode(r, &q) != nil || q.Version <= 0 {
		httpx.Error(w, 400, "invalid_input", "version 无效")
		return
	}
	var latest int
	e := s.DB.QueryRowContext(r.Context(), `SELECT latest_version FROM config_assets WHERE id=$1 AND user_id=$2`, assetID, userID).Scan(&latest)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "配置不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if latest >= MaxVersionsPerAsset {
		httpx.Error(w, 400, "invalid_input", "版本数量已达上限")
		return
	}
	content, e := loadVersionContent(r.Context(), s, assetID, userID, q.Version)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "目标版本不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	var lossRaw []byte
	if e := s.DB.QueryRowContext(r.Context(), `SELECT loss_report FROM config_versions WHERE asset_id=$1 AND version=$2`, assetID, q.Version).Scan(&lossRaw); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	loss := []LossEntry{}
	_ = json.Unmarshal(lossRaw, &loss)
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `UPDATE config_assets SET latest_version=$1,updated_at=now() WHERE id=$2`, latest+1, assetID); e != nil {
			return e
		}
		return insertVersion(r.Context(), tx, assetID, userID, latest+1, content, loss, "rollback")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "回滚失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"version": latest + 1, "ok": true})
}

// TransformPreview renders the latest version for a target platform without persisting.
func (s *Service) TransformPreview(w http.ResponseWriter, r *http.Request) {
	var q struct {
		TargetPlatform string `json:"target_platform"`
	}
	assetID, userID := r.PathValue("id"), auth.Who(r).UserID
	if httpx.Decode(r, &q) != nil || q.TargetPlatform != "codex_cli" {
		httpx.Error(w, 400, "invalid_input", "v1 只支持 target_platform=codex_cli")
		return
	}
	var kind string
	var latest int
	e := s.DB.QueryRowContext(r.Context(), `SELECT kind,latest_version FROM config_assets WHERE id=$1 AND user_id=$2`, assetID, userID).Scan(&kind, &latest)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "配置不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	content, e := loadVersionContent(r.Context(), s, assetID, userID, latest)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if kind != "mcp_server" {
		httpx.Error(w, 400, "invalid_input", "v1 transform 只支持 mcp_server")
		return
	}
	out, loss, e := TransformToCodex(content)
	if e != nil {
		httpx.Error(w, 400, "invalid_input", "转换失败: "+e.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"target_platform": q.TargetPlatform, "content": out, "loss_report": loss})
}

// CreateBinding declares an install target; actual file writes stay with the desktop bridge.
func (s *Service) CreateBinding(w http.ResponseWriter, r *http.Request) {
	var q struct {
		TargetPlatform string `json:"target_platform"`
		TargetPath     string `json:"target_path"`
	}
	assetID, userID := r.PathValue("id"), auth.Who(r).UserID
	if httpx.Decode(r, &q) != nil || q.TargetPlatform == "" || len(q.TargetPath) > 1024 {
		httpx.Error(w, 400, "invalid_input", "检查绑定目标")
		return
	}
	var n int
	if e := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM config_assets WHERE id=$1 AND user_id=$2`, assetID, userID).Scan(&n); e != nil || n == 0 {
		httpx.Error(w, 404, "not_found", "配置不存在")
		return
	}
	_, e := s.DB.ExecContext(r.Context(), `INSERT INTO config_bindings(id,asset_id,user_id,target_platform,target_path)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (asset_id,target_platform,target_path) DO UPDATE SET last_applied_version=0,status='declared'`,
		httpx.NewID("cfb"), assetID, userID, q.TargetPlatform, q.TargetPath)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "绑定失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]bool{"ok": true})
}

// ImportFromPlatform ingests a platform config file (v1: claude_desktop JSON),
// classifies secrets and stores canonical content + loss report.
func (s *Service) ImportFromPlatform(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Platform string `json:"platform"`
		Name     string `json:"name"`
		Content  string `json:"content"`
	}
	if httpx.Decode(r, &q) != nil || q.Platform != "claude_desktop" {
		httpx.Error(w, 400, "invalid_input", "v1 只支持 platform=claude_desktop")
		return
	}
	if q.Content == "" || len(q.Content) > MaxContentBytes {
		httpx.Error(w, 400, "invalid_input", "内容为空或超过 256KiB 上限")
		return
	}
	name := strings.TrimSpace(q.Name)
	if name == "" {
		name = "Claude Desktop 配置"
	}
	if len([]rune(name)) > MaxAssetNameRunes {
		httpx.Error(w, 400, "invalid_input", "名称过长")
		return
	}
	content, loss, e := ParseClaudeDesktop(name, []byte(q.Content))
	if e != nil {
		httpx.Error(w, 400, "invalid_import", "解析失败: "+e.Error())
		return
	}
	id := httpx.NewID("cfg")
	e = db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO config_assets(id,user_id,name,kind,source_platform,latest_version) VALUES($1,$2,$3,'mcp_server','claude_desktop',1)`,
			id, auth.Who(r).UserID, name); e != nil {
			return e
		}
		return insertVersion(r.Context(), tx, id, auth.Who(r).UserID, 1, content, loss, "import")
	})
	if e != nil {
		httpx.Error(w, 503, "unavailable", "导入失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true, "loss_report": loss})
}
