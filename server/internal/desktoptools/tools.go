// Package desktoptools reuses the domain parsers without a server, database,
// network access or arbitrary command execution. Only explicit inputs are read.
package desktoptools

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	cfg "aihub.dev/server/internal/config"
	conv "aihub.dev/server/internal/conversation"
)

const MaxImportBytes = 32 * 1024 * 1024

type Input struct {
	Action   string   `json:"action"`
	Name     string   `json:"name"`
	Platform string   `json:"platform"`
	Kind     string   `json:"kind"`
	Data     []byte   `json:"data"`
	Roots    []string `json:"roots,omitempty"`
}
type Packet struct {
	Source        string `json:"source"`
	Body          string `json:"body"`
	SHA256        string `json:"sha256"`
	Conversations int    `json:"conversations"`
	Messages      int    `json:"messages"`
}
type ImportPlan struct {
	Source      string   `json:"source"`
	SHA256      string   `json:"sha256"`
	Packets     []Packet `json:"packets"`
	Warnings    []string `json:"warnings"`
	Attachments []string `json:"attachments"`
}
type ConfigPreview struct {
	Kind     string          `json:"kind"`
	Platform string          `json:"platform"`
	Content  map[string]any  `json:"content"`
	Loss     []cfg.LossEntry `json:"loss_report"`
}

func digest(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }

func Run(in Input) (any, error) {
	switch in.Action {
	case "conversation":
		return PlanImport(in.Name, in.Data)
	case "config":
		return PreviewConfig(in.Platform, in.Kind, in.Data)
	default:
		return nil, errors.New("unsupported desktop tool")
	}
}

// A selected ZIP is inspected in memory. Entries are never extracted to disk.
func unpack(data []byte) (string, []byte, []string, error) {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return "", nil, nil, errors.New("ZIP 文件损坏")
	}
	if len(z.File) > 2000 {
		return "", nil, nil, errors.New("ZIP 文件数量超过 2000")
	}
	var found []byte
	var name string
	var total uint64
	attachments := []string{}
	seen := map[string]bool{}
	for _, f := range z.File {
		n := f.Name
		if strings.Contains(n, "\\") || strings.Contains(n, ":") || strings.HasPrefix(n, "/") || strings.ContainsRune(n, 0) || path.Clean(n) != strings.TrimSuffix(n, "/") || strings.HasPrefix(n, "../") || f.Mode()&os.ModeSymlink != 0 {
			return "", nil, nil, errors.New("ZIP 包含不安全路径或链接")
		}
		if seen[n] {
			return "", nil, nil, errors.New("ZIP 包含重复路径")
		}
		seen[n] = true
		total += f.UncompressedSize64
		if total > 128*1024*1024 || f.UncompressedSize64 > MaxImportBytes || (f.UncompressedSize64 > 1024*1024 && f.UncompressedSize64 > 100*(f.CompressedSize64+1)) {
			return "", nil, nil, errors.New("ZIP 解压大小或压缩比超过限制")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		base := path.Base(n)
		if base == "conversations.json" || base == "archive.json" || base == "conversations.jsonl" {
			if found != nil {
				return "", nil, nil, errors.New("ZIP 内有多个会话文件，请单独选择一个导入")
			}
			r, err := f.Open()
			if err != nil {
				return "", nil, nil, errors.New("无法读取 ZIP 会话文件")
			}
			found, err = io.ReadAll(io.LimitReader(r, MaxImportBytes+1))
			r.Close()
			if err != nil || len(found) > MaxImportBytes {
				return "", nil, nil, errors.New("ZIP 会话文件无法完整读取")
			}
			name = base
		} else if strings.HasPrefix(n, "attachments/") || strings.HasPrefix(n, "files/") {
			attachments = append(attachments, n)
		}
	}
	if found == nil {
		return "", nil, nil, errors.New("ZIP 内未找到 conversations.json、archive.json 或 conversations.jsonl")
	}
	return name, found, attachments, nil
}

func PlanImport(name string, data []byte) (*ImportPlan, error) {
	if len(data) == 0 || len(data) > MaxImportBytes {
		return nil, errors.New("文件为空或超过 32 MiB")
	}
	plan := &ImportPlan{SHA256: digest(data), Packets: []Packet{}, Warnings: []string{}, Attachments: []string{}}
	if strings.EqualFold(path.Ext(name), ".zip") {
		var e error
		name, data, plan.Attachments, e = unpack(data)
		if e != nil {
			return nil, e
		}
		if len(plan.Attachments) > 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("发现 %d 个附件：当前服务器接口不接收附件，原 ZIP 请保留；会话文字可独立导入。", len(plan.Attachments)))
		}
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("会话文件为空")
	}
	add := func(source string, raw []byte) error {
		im, e := conv.ParseImport(source, raw)
		if e != nil {
			return fmt.Errorf("第 %d 个导入分段不符合 v1 限制: %w", len(plan.Packets)+1, e)
		}
		messages := 0
		for _, c := range im.Conversations {
			for _, b := range c.Branches {
				messages += len(b.Messages)
			}
		}
		plan.Packets = append(plan.Packets, Packet{source, string(raw), digest(raw), len(im.Conversations), messages})
		plan.Warnings = append(plan.Warnings, im.Warnings...)
		return nil
	}
	if data[0] == '[' {
		var rows []json.RawMessage
		if json.Unmarshal(data, &rows) != nil || len(rows) == 0 || len(rows) > 1000 {
			return nil, errors.New("ChatGPT 会话数组无效或超过 1000 个会话")
		}
		plan.Source = "chatgpt_export"
		for _, row := range rows {
			if e := add(plan.Source, append(append([]byte{'['}, row...), ']')); e != nil {
				return nil, e
			}
		}
	} else {
		var env struct {
			Format        string                `json:"format"`
			Version       int                   `json:"version"`
			Conversations []json.RawMessage     `json:"conversations"`
			Projects      []conv.ArchiveProject `json:"projects"`
		}
		if json.Unmarshal(data, &env) == nil && env.Format != "" {
			if env.Format != conv.ArchiveFormat || env.Version != 1 {
				return nil, errors.New("不支持此 Archive 格式或版本；请使用 Archive v1")
			}
			if len(env.Conversations) == 0 || len(env.Conversations) > 1000 {
				return nil, errors.New("Archive 需要 1–1000 个会话")
			}
			plan.Source = "archive"
			for _, row := range env.Conversations {
				raw, e := json.Marshal(map[string]any{"format": env.Format, "version": 1, "projects": env.Projects, "conversations": []json.RawMessage{row}})
				if e != nil {
					return nil, e
				}
				if e = add("archive", raw); e != nil {
					return nil, e
				}
			}
		} else if strings.EqualFold(path.Ext(name), ".jsonl") {
			lines := bytes.Split(data, []byte{'\n'})
			var first map[string]json.RawMessage
			_ = json.Unmarshal(bytes.TrimSpace(lines[0]), &first)
			if first["branches"] != nil && first["provider_slug"] != nil {
				plan.Source = "archive"
				for _, line := range lines {
					if len(bytes.TrimSpace(line)) == 0 {
						continue
					}
					var c conv.Conversation
					if json.Unmarshal(line, &c) != nil {
						return nil, errors.New("Canonical JSONL 有无效行")
					}
					raw, e := conv.BuildArchive([]conv.Conversation{c}, nil)
					if e != nil {
						return nil, e
					}
					if e = add("archive", raw); e != nil {
						return nil, e
					}
					if len(plan.Packets) > 1000 {
						return nil, errors.New("JSONL 超过 1000 个会话")
					}
				}
			} else {
				plan.Source = "codex_cli_jsonl"
				if e := add(plan.Source, data); e != nil {
					return nil, e
				}
			}
		} else {
			return nil, errors.New("无法识别格式：请选择 ChatGPT conversations.json、Codex JSONL 或 AI Hub Archive v1")
		}
	}
	return plan, nil
}

var sensitive = regexp.MustCompile(`(?i)(token|secret|password|passwd|api.?key|authorization|cookie|credential)`)
var tokenValue = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{12,}|Bearer\s+\S+|eyJ[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+)`)

func PreviewConfig(platform, kind string, raw []byte) (*ConfigPreview, error) {
	if len(raw) == 0 || len(raw) > cfg.MaxContentBytes {
		return nil, errors.New("配置文件为空或超过 256 KiB")
	}
	if kind == "" {
		kind = "mcp_server"
	}
	var content map[string]any
	var loss []cfg.LossEntry
	var e error
	switch platform {
	case "claude_desktop", "cursor", "mcp_generic":
		content, loss, e = cfg.ParseClaudeDesktop("", raw)
	case "codex_cli":
		content, loss, e = cfg.ParseCodexTOML(raw)
	case "canonical":
		if json.Unmarshal(raw, &content) != nil {
			return nil, errors.New("Canonical JSON 无效")
		}
		if format, ok := content["format"]; ok {
			if format != "aihub.portable-config" || content["version"] != float64(1) {
				return nil, errors.New("不支持此配置格式或版本")
			}
			if k, ok := content["kind"].(string); ok {
				kind = k
			}
			content, _ = content["content"].(map[string]any)
		}
		loss = []cfg.LossEntry{}
		content, _ = cfg.ClassifySecrets(content, "", &loss).(map[string]any)
	default:
		return nil, errors.New("不支持的配置来源")
	}
	if e != nil {
		return nil, errors.New("配置解析失败：请检查格式与必需字段")
	}
	// Environment values stay local even when a vendor uses an unexpected key.
	if servers, ok := content["mcp_servers"].([]any); ok {
		for _, item := range servers {
			server, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if env, ok := server["env"].(map[string]any); ok {
				for key, v := range env {
					if _, ok := v.(string); ok {
						env[key] = map[string]any{"secret_ref": "env:" + key}
						loss = append(loss, cfg.LossEntry{Path: "env." + key, Reason: "local_environment_reference"})
					}
				}
			}
			if args, ok := server["args"].([]any); ok {
				for i, v := range args {
					s, ok := v.(string)
					if !ok {
						continue
					}
					previous := ""
					if i > 0 {
						previous, _ = args[i-1].(string)
					}
					if tokenValue.MatchString(s) || (strings.HasPrefix(previous, "-") && sensitive.MatchString(previous)) || (strings.Contains(s, "=") && sensitive.MatchString(strings.SplitN(s, "=", 2)[0])) {
						args[i] = fmt.Sprintf("${LOCAL_ARG_%d}", i)
						loss = append(loss, cfg.LossEntry{Path: fmt.Sprintf("args[%d]", i), Reason: "local_argument_reference"})
					}
				}
			}
			if u, ok := server["url"].(string); ok && (strings.Contains(u, "@") || strings.Contains(u, "?") || tokenValue.MatchString(u)) {
				delete(server, "url")
				loss = append(loss, cfg.LossEntry{Path: "url", Reason: "credential_or_query_url_removed"})
			}
		}
	}
	if e = cfg.ValidateContent(kind, content); e != nil {
		return nil, errors.New("配置无法安全转换：请移除带凭据的 URL，并检查必需字段")
	}
	return &ConfigPreview{kind, platform, content, loss}, nil
}
