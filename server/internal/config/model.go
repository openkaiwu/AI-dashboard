// Package config owns AIPortableConfig v1 (M4): canonical config assets,
// append-only versions, secret hygiene (SecretRef), transforms and loss reports.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxAssetNameRunes   = 200
	MaxContentBytes     = 256 * 1024
	MaxServersPerConfig = 64
	MaxVersionsPerAsset = 200
)

var (
	ErrInvalidContent = errors.New("invalid canonical content")
	ErrUnknownKind    = errors.New("unknown asset kind")
)

var secretKeyPattern = regexp.MustCompile(`(?i)(token|secret|password|passwd|api_?key|authorization|cookie|private_?key|credential)`)

// LossEntry records one transformation loss; values are never included.
type LossEntry struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Canonical content per kind (frozen v1):
//   mcp_server:      {"mcp_servers":[{"name","command","args",["env"]}]}
//   prompt_template: {"name","body","variables"}
//   agent_profile:   {"name","instructions","model"}

// ClassifySecrets walks canonical content and replaces any secret-looking KEY's
// string value with {"secret_ref":"env:NAME"}. Original values are discarded and
// recorded by name only. Mutates content in place, returns loss entries.
func ClassifySecrets(node any, path string, loss *[]LossEntry) any {
	switch tv := node.(type) {
	case map[string]any:
		for key, value := range tv {
			childPath := path + "." + key
			if secretKeyPattern.MatchString(key) {
				if _, isRef := value.(map[string]any); !isRef {
					name := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(key, "_"))
					tv[key] = map[string]any{"secret_ref": "env:" + name}
					*loss = append(*loss, LossEntry{Path: childPath, Reason: "secret_value_replaced_by_ref", Detail: "env:" + name})
					continue
				}
			}
			tv[key] = ClassifySecrets(value, childPath, loss)
		}
		return tv
	case []any:
		for i, value := range tv {
			tv[i] = ClassifySecrets(value, fmt.Sprintf("%s[%d]", path, i), loss)
		}
		return tv
	default:
		return node
	}
}

// SecretKeyNames extracts secret-looking KEY names from a JSON object tree or
// TOML-ish text. Only names are returned; values are never retained.
func SecretKeyNames(data []byte, format string) []string {
	seen := map[string]bool{}
	keys := []string{}
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] || !secretKeyPattern.MatchString(key) {
			return
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if format == "json" {
		var tree any
		if json.Unmarshal(data, &tree) != nil {
			return keys
		}
		var walk func(node any)
		walk = func(node any) {
			switch tv := node.(type) {
			case map[string]any:
				for key, value := range tv {
					add(key)
					walk(value)
				}
			case []any:
				for _, value := range tv {
					walk(value)
				}
			}
		}
		walk(tree)
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if i := strings.IndexByte(line, '='); i > 0 {
				add(strings.Trim(line[:i], "\"'[] ."))
			}
		}
	}
	if len(keys) > 64 {
		keys = keys[:64]
	}
	return keys
}

// ValidateContent checks the canonical shape for the asset kind.
func ValidateContent(kind string, content map[string]any) error {
	switch kind {
	case "mcp_server":
		raw, ok := content["mcp_servers"].([]any)
		if !ok || len(raw) == 0 {
			return fmt.Errorf("%w: mcp_servers array required", ErrInvalidContent)
		}
		if len(raw) > MaxServersPerConfig {
			return fmt.Errorf("%w: too many mcp_servers", ErrInvalidContent)
		}
		for i, s := range raw {
			server, ok := s.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: mcp_servers[%d] must be an object", ErrInvalidContent, i)
			}
			name, _ := server["name"].(string)
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("%w: mcp_servers[%d].name required", ErrInvalidContent, i)
			}
			command, _ := server["command"].(string)
			url, _ := server["url"].(string)
			if strings.TrimSpace(command) == "" && strings.TrimSpace(url) == "" {
				return fmt.Errorf("%w: mcp_servers[%d] needs command or url", ErrInvalidContent, i)
			}
		}
	case "prompt_template":
		body, _ := content["body"].(string)
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("%w: prompt body required", ErrInvalidContent)
		}
	case "agent_profile":
		instructions, _ := content["instructions"].(string)
		if strings.TrimSpace(instructions) == "" {
			return fmt.Errorf("%w: agent instructions required", ErrInvalidContent)
		}
	default:
		return ErrUnknownKind
	}
	return nil
}
