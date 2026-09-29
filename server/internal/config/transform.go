package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Platforms (frozen v1): canonical, claude_desktop, codex_cli.
// Reference transform (INH-471): claude_desktop → canonical → codex_cli.

// ParseClaudeDesktop converts a claude_desktop_config.json body into canonical
// mcp_server content. Unknown top-level keys are recorded, never silently dropped.
func ParseClaudeDesktop(name string, raw []byte) (map[string]any, []LossEntry, error) {
	var doc map[string]any
	if e := json.Unmarshal(raw, &doc); e != nil {
		return nil, nil, fmt.Errorf("%w: claude desktop json: %v", ErrInvalidContent, e)
	}
	loss := []LossEntry{}
	servers := []any{}
	rawServers, _ := doc["mcpServers"].(map[string]any)
	names := make([]string, 0, len(rawServers))
	for key := range rawServers {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		server, _ := rawServers[key].(map[string]any)
		if server == nil {
			loss = append(loss, LossEntry{Path: "mcpServers." + key, Reason: "not_an_object"})
			continue
		}
		entry := map[string]any{"name": key}
		for _, field := range []string{"command", "args", "url"} {
			if v, ok := server[field]; ok {
				entry[field] = v
			}
		}
		if env, ok := server["env"].(map[string]any); ok && len(env) > 0 {
			entry["env"] = env
		}
		for field := range server {
			switch field {
			case "command", "args", "url", "env":
			default:
				loss = append(loss, LossEntry{Path: "mcpServers." + key + "." + field, Reason: "field_not_representable_in_canonical_v1"})
			}
		}
		servers = append(servers, entry)
	}
	for key := range doc {
		if key != "mcpServers" {
			loss = append(loss, LossEntry{Path: key, Reason: "top_level_key_not_representable_in_canonical_v1"})
		}
	}
	content := map[string]any{"mcp_servers": servers}
	contentAny := ClassifySecrets(content, "", &loss)
	canonical, _ := contentAny.(map[string]any)
	if e := ValidateContent("mcp_server", canonical); e != nil {
		return nil, loss, e
	}
	return canonical, loss, nil
}

// TransformToCodex renders canonical content as a Codex CLI config.toml fragment.
// Secret refs become ${NAME} placeholders; original values are long gone.
func TransformToCodex(content map[string]any) (string, []LossEntry, error) {
	loss := []LossEntry{}
	servers, ok := content["mcp_servers"].([]any)
	if !ok {
		return "", loss, fmt.Errorf("%w: mcp_servers required for codex_cli", ErrInvalidContent)
	}
	sorted := make([]map[string]any, 0, len(servers))
	for _, s := range servers {
		m, _ := s.(map[string]any)
		if m != nil {
			sorted = append(sorted, m)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, _ := sorted[i]["name"].(string)
		b, _ := sorted[j]["name"].(string)
		return a < b
	})
	var b strings.Builder
	for _, server := range sorted {
		name, _ := server["name"].(string)
		fmt.Fprintf(&b, "[mcp_servers.%s]\n", tomlKey(name))
		if command, ok := server["command"].(string); ok && command != "" {
			fmt.Fprintf(&b, "command = %s\n", tomlString(command))
		}
		if url, ok := server["url"].(string); ok && url != "" {
			fmt.Fprintf(&b, "url = %s\n", tomlString(url))
			loss = append(loss, LossEntry{Path: "mcp_servers." + name + ".url", Reason: "codex_cli_v1_remote_transport_unverified", Detail: "url kept; verify server support"})
		}
		if args, ok := server["args"].([]any); ok && len(args) > 0 {
			parts := make([]string, 0, len(args))
			for _, a := range args {
				s, _ := a.(string)
				parts = append(parts, tomlString(s))
			}
			fmt.Fprintf(&b, "args = [%s]\n", strings.Join(parts, ", "))
		}
		if env, ok := server["env"].(map[string]any); ok && len(env) > 0 {
			b.WriteString("[mcp_servers." + tomlKey(name) + ".env]\n")
			keys := make([]string, 0, len(env))
			for key := range env {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				switch v := env[key].(type) {
				case string:
					fmt.Fprintf(&b, "%s = %s\n", tomlKey(key), tomlString(v))
				case map[string]any:
					ref, _ := v["secret_ref"].(string)
					varName := strings.TrimPrefix(ref, "env:")
					fmt.Fprintf(&b, "%s = %s\n", tomlKey(key), tomlString("${"+varName+"}"))
					loss = append(loss, LossEntry{Path: "mcp_servers." + name + ".env." + key, Reason: "secret_value_unavailable", Detail: ref})
				}
			}
		}
		for field := range server {
			switch field {
			case "name", "command", "args", "url", "env":
			default:
				loss = append(loss, LossEntry{Path: "mcp_servers." + name + "." + field, Reason: "field_not_representable_in_codex_cli_v1"})
			}
		}
		b.WriteString("\n")
	}
	return b.String(), loss, nil
}

func tomlString(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t")
	return "\"" + r.Replace(s) + "\""
}

func tomlKey(s string) string {
	if s == "" || strings.ContainsAny(s, " .[]\"#=\n") {
		return tomlString(s)
	}
	return s
}
