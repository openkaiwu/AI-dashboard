package conversation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ParseImport converts raw bytes from a known source into a validated canonical batch.
// All parsing is size-capped and side-effect free; nothing here executes input.
func ParseImport(sourceType string, raw []byte) (*Import, error) {
	if len(raw) > MaxFileBytes {
		return nil, ErrTooLarge
	}
	var im Import
	switch sourceType {
	case "archive":
		var env archiveEnvelope
		if e := json.NewDecoder(bytes.NewReader(raw)).Decode(&env); e != nil {
			return nil, fmt.Errorf("%w: archive json: %v", ErrFormat, e)
		}
		if env.Format != ArchiveFormat || env.Version != 1 {
			return nil, fmt.Errorf("%w: unsupported archive %q v%d", ErrFormat, env.Format, env.Version)
		}
		im.Conversations = env.Conversations
		im.Projects = env.Projects
	case "chatgpt_export":
		im.Conversations = parseChatGPTExport(raw)
	case "codex_cli_jsonl":
		im.Conversations = parseCodexJSONL(raw)
	default:
		return nil, fmt.Errorf("%w: unknown source %q", ErrFormat, sourceType)
	}
	im.SourceType = sourceType
	if e := im.Validate(); e != nil {
		return nil, e
	}
	for i := range im.Conversations {
		c := &im.Conversations[i]
		c.ContentHash = ContentHash(c)
		if c.ExternalID == "" {
			// Content-derived identity so archive round-trips dedup without server ids.
			c.ExternalID = c.ContentHash[:24]
		}
		c.DedupKey = c.ExternalID
	}
	return &im, nil
}

// ContentHash is the dedup fingerprint: ordered (branch, role, content) triples.
func ContentHash(c *Conversation) string {
	h := sha256.New()
	for _, b := range c.Branches {
		h.Write([]byte("b\x00" + b.Name + "\x00"))
		for _, m := range b.Messages {
			h.Write([]byte(m.Role + "\x00" + m.Content + "\x00"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// --- chatgpt_export: conversations.json whose mapping is a message tree ---

type gptNode struct {
	Parent   string   `json:"parent"`
	Children []string `json:"children"`
	Message  *struct {
		Author struct {
			Role string `json:"role"`
		} `json:"author"`
		Content struct {
			ContentType string `json:"content_type"`
			Parts       []any  `json:"parts"`
			Text        string `json:"text"`
		} `json:"content"`
		CreateTime *float64 `json:"create_time"`
	} `json:"message"`
}

type gptConversation struct {
	Title       string             `json:"title"`
	CreateTime  *float64           `json:"create_time"`
	Mapping     map[string]gptNode `json:"mapping"`
	CurrentNode string             `json:"current_node"`
}

func gptParts(n *gptNode) string {
	c := n.Message.Content
	if c.ContentType == "text" && c.Text != "" {
		return c.Text
	}
	var b strings.Builder
	for _, p := range c.Parts {
		s, ok := p.(string)
		if !ok || s == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}
	return b.String()
}

func gptTime(v *float64) *time.Time {
	if v == nil || *v <= 0 {
		return nil
	}
	t := time.Unix(int64(*v), 0).UTC()
	return &t
}

func gptLineage(mapping map[string]gptNode, from, to string) []string {
	var rev []string
	cur := from
	for cur != "" && cur != to {
		n, ok := mapping[cur]
		if !ok {
			break
		}
		rev = append(rev, cur)
		if n.Parent == cur {
			break
		}
		cur = n.Parent
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func appendGptBranch(conv *Conversation, mapping map[string]gptNode, path []string, name string) bool {
	b := Branch{Name: name}
	for _, id := range path {
		n := mapping[id]
		if n.Message == nil {
			continue // wrapper/meta nodes carry no content
		}
		content := gptParts(&n)
		if strings.TrimSpace(content) == "" {
			continue
		}
		m := Message{Role: n.Message.Author.Role, Content: content, SentAt: gptTime(n.Message.CreateTime), ExternalRef: id}
		if len(b.Messages) > 0 {
			// Within a branch the parent chain is the previous content-bearing message;
			// the branch root itself has no parent (fork point recorded via branch name).
			v := len(b.Messages) - 1
			m.ParentIdx = &v
		}
		b.Messages = append(b.Messages, m)
		if len(b.Messages) >= MaxMessagesPerConv {
			break
		}
	}
	if len(b.Messages) == 0 {
		return false
	}
	conv.Branches = append(conv.Branches, b)
	return true
}

// parseChatGPTExport walks the current path as "main" and turns every divergent
// subtree into a named branch, preserving in-branch parent chains.
func parseChatGPTExport(raw []byte) []Conversation {
	var src []gptConversation
	if e := json.Unmarshal(raw, &src); e != nil {
		return nil
	}
	out := make([]Conversation, 0, len(src))
	for _, g := range src {
		if len(g.Mapping) == 0 {
			continue
		}
		conv := Conversation{Title: g.Title, ProviderSlug: "chatgpt", CreatedAt: gptTime(g.CreateTime)}
		main := gptLineage(g.Mapping, g.CurrentNode, "")
		if !appendGptBranch(&conv, g.Mapping, main, "main") {
			continue
		}
		onMain := map[string]bool{}
		for _, id := range main {
			onMain[id] = true
		}
		// Deterministic branch naming: sort divergent roots by node id.
		var roots []string
		for id, node := range g.Mapping {
			if node.Message == nil || onMain[id] || !onMain[node.Parent] {
				continue
			}
			if len(g.Mapping[node.Parent].Children) < 2 {
				continue
			}
			roots = append(roots, id)
		}
		sort.Strings(roots)
		for i, root := range roots {
			if len(conv.Branches) >= MaxBranchesPerConv {
				break
			}
			// Capture the full alternate lineage down to its deepest node.
			var deep string
			cur := root
			for {
				deep = cur
				cn := g.Mapping[cur]
				if len(cn.Children) == 0 {
					break
				}
				cur = cn.Children[len(cn.Children)-1]
			}
			appendGptBranch(&conv, g.Mapping, gptLineage(g.Mapping, deep, g.Mapping[root].Parent), fmt.Sprintf("branch-%d", i+1))
		}
		out = append(out, conv)
	}
	return out
}

// --- codex_cli_jsonl: tolerant line-delimited session export ---

func parseCodexJSONL(raw []byte) []Conversation {
	conv := Conversation{Title: "Codex 会话", ProviderSlug: "codex_cli"}
	branch := Branch{Name: "main"}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item struct {
			Timestamp string `json:"timestamp"`
			Payload   struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
				Text    string `json:"text"`
			} `json:"payload"`
			Role    string `json:"role"`
			Content any    `json:"content"`
		}
		if e := json.Unmarshal([]byte(line), &item); e != nil {
			continue // tolerate foreign lines; the format is append-only per line
		}
		role := item.Payload.Role
		if role == "" {
			role = item.Role
		}
		content := item.Payload.Text
		if content == "" {
			content = flattenContent(item.Payload.Content)
		}
		if content == "" {
			content = flattenContent(item.Content)
		}
		if role == "" || strings.TrimSpace(content) == "" {
			continue
		}
		m := Message{Role: role, Content: content}
		if t, e := time.Parse(time.RFC3339, item.Timestamp); e == nil {
			m.SentAt = &t
		}
		branch.Messages = append(branch.Messages, m)
		if len(branch.Messages) >= MaxMessagesPerConv {
			break
		}
	}
	if len(branch.Messages) == 0 {
		return nil
	}
	conv.Branches = []Branch{branch}
	return []Conversation{conv}
}

// flattenContent renders Codex-style content blocks [{type:text|input_text|output_text,...}].
func flattenContent(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case []any:
		var b strings.Builder
		for _, part := range tv {
			pm, ok := part.(map[string]any)
			if !ok {
				continue
			}
			text, _ := pm["text"].(string)
			for _, key := range []string{"input_text", "output_text"} {
				if text == "" {
					text, _ = pm[key].(string)
				}
			}
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
		return b.String()
	case map[string]any:
		text, _ := tv["text"].(string)
		return text
	}
	return ""
}
