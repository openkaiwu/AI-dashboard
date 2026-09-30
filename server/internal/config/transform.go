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

// TransformToClaude renders canonical content as a claude_desktop_config.json body.
// Secret refs become ${NAME} placeholders; values stay unknown.
func TransformToClaude(content map[string]any) (string, []LossEntry, error) {
	loss := []LossEntry{}
	servers, ok := content["mcp_servers"].([]any)
	if !ok {
		return "", loss, fmt.Errorf("%w: mcp_servers required for claude_desktop", ErrInvalidContent)
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
	out := map[string]any{"mcpServers": map[string]any{}}
	target := out["mcpServers"].(map[string]any)
	for _, server := range sorted {
		name, _ := server["name"].(string)
		entry := map[string]any{}
		if command, ok := server["command"].(string); ok && command != "" {
			entry["command"] = command
		}
		if url, ok := server["url"].(string); ok && url != "" {
			entry["url"] = url
			loss = append(loss, LossEntry{Path: "mcpServers." + name + ".url", Reason: "claude_desktop_v1_remote_transport_unverified", Detail: "url kept; verify server support"})
		}
		if args, ok := server["args"].([]any); ok && len(args) > 0 {
			entry["args"] = args
		}
		if env, ok := server["env"].(map[string]any); ok && len(env) > 0 {
			envOut := map[string]any{}
			keys := make([]string, 0, len(env))
			for key := range env {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				switch v := env[key].(type) {
				case string:
					envOut[key] = v
				case map[string]any:
					ref, _ := v["secret_ref"].(string)
					varName := strings.TrimPrefix(ref, "env:")
					envOut[key] = "${" + varName + "}"
					loss = append(loss, LossEntry{Path: "mcpServers." + name + ".env." + key, Reason: "secret_value_unavailable", Detail: ref})
				}
			}
			entry["env"] = envOut
		}
		for field := range server {
			switch field {
			case "name", "command", "args", "url", "env":
			default:
				loss = append(loss, LossEntry{Path: "mcp_servers." + name + "." + field, Reason: "field_not_representable_in_claude_desktop_v1"})
			}
		}
		target[name] = entry
	}
	raw, e := json.MarshalIndent(out, "", "  ")
	if e != nil {
		return "", loss, e
	}
	return string(raw), loss, nil
}

// ParseCodexTOML parses the constrained Codex CLI config.toml fragment produced by
// TransformToCodex back into canonical content (INH-473 reverse transformer).
// Unknown keys are recorded, never silently dropped; ${NAME} placeholders map back
// to secret refs.
func ParseCodexTOML(raw []byte) (map[string]any, []LossEntry, error) {
	loss := []LossEntry{}
	servers := map[string]map[string]any{}
	order := []string{}
	current := ""
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(line[1 : len(line)-1])
			parts := strings.Split(section, ".")
			if len(parts) >= 2 && parts[0] == "mcp_servers" {
				name := strings.Trim(parts[1], "\"")
				if _, ok := servers[name]; !ok {
					servers[name] = map[string]any{"name": name}
					order = append(order, name)
				}
				if len(parts) >= 3 && parts[2] == "env" {
					current = name + "|env"
				} else if len(parts) == 2 {
					current = name
				} else {
					loss = append(loss, LossEntry{Path: section, Reason: "section_not_representable_in_canonical_v1"})
					current = ""
				}
			} else {
				loss = append(loss, LossEntry{Path: section, Reason: "top_level_section_not_representable_in_canonical_v1"})
				current = ""
			}
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 || current == "" {
			if eq >= 0 {
				loss = append(loss, LossEntry{Path: strings.TrimSpace(line[:eq]), Reason: "key_outside_known_section"})
			}
			continue
		}
		key := strings.Trim(strings.TrimSpace(line[:eq]), "\"")
		value := strings.TrimSpace(line[eq+1:])
		if current == "" {
			continue
		}
		name, kind, _ := strings.Cut(current, "|")
		server := servers[name]
		switch {
		case kind == "env":
			env, _ := server["env"].(map[string]any)
			if env == nil {
				env = map[string]any{}
				server["env"] = env
			}
			env[key] = tomltValue(value, "mcpServers."+name+".env."+key, &loss)
		case key == "command" || key == "url":
			server[key] = tomltString(value)
		case key == "args":
			inner := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
			items := []any{}
			for _, part := range strings.Split(inner, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					items = append(items, tomltString(part))
				}
			}
			server[key] = items
		default:
			loss = append(loss, LossEntry{Path: fmt.Sprintf("mcp_servers.%s.%s", name, key), Reason: "field_not_representable_in_canonical_v1"})
		}
		_ = lineNo
	}
	list := []any{}
	names := append([]string(nil), order...)
	sort.Strings(names)
	for _, name := range names {
		list = append(list, servers[name])
	}
	content := map[string]any{"mcp_servers": list}
	contentAny := ClassifySecrets(content, "", &loss)
	canonical, _ := contentAny.(map[string]any)
	if e := ValidateContent("mcp_server", canonical); e != nil {
		return nil, loss, e
	}
	return canonical, loss, nil
}

func tomltString(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		inner := v[1 : len(v)-1]
		r := strings.NewReplacer("\\\\", "\\", "\\\"", "\"", "\n", "\n", "\t", "\t")
		return r.Replace(inner)
	}
	return v
}

// tomltValue maps a TOML scalar back to canonical form; ${NAME} placeholders
// become secret refs so values stay unknown.
func tomltValue(v, path string, loss *[]LossEntry) any {
	s := tomltString(v)
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		name := s[2 : len(s)-1]
		*loss = append(*loss, LossEntry{Path: path, Reason: "secret_value_unavailable", Detail: "env:" + name})
		return map[string]any{"secret_ref": "env:" + name}
	}
	return s
}
