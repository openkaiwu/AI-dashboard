// Package conversation owns the canonical project/conversation graph (M3):
// import parsing, dedup identity, archive round-trip and its HTTP surface.
// Models are validate-only; persistence stays in this package's service.
package conversation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaxFileBytes      = 900 * 1024 // httpx.Guard caps bodies at 1MiB; keep headroom for framing.
	MaxConversations  = 50
	MaxMessagesPerConv = 4000
	MaxBranchesPerConv = 20
	MaxContentRunes   = 64 * 1024
	MaxTitleRunes     = 300
)

var (
	// ErrTooLarge marks imports beyond the frozen v1 size boundary.
	ErrTooLarge = errors.New("import exceeds 900KiB v1 boundary")
	// ErrTooMany marks imports beyond per-batch/per-conversation limits.
	ErrTooMany = errors.New("import exceeds v1 entity limits")
	// ErrFormat marks structurally invalid source payloads.
	ErrFormat = errors.New("unrecognized source format")
)

// Branch is a named lineage inside one conversation; messages reference it by branch_id.
type Branch struct {
	Name    string `json:"name"`
	Messages []Message `json:"messages"`
}

// Message is one node of the append-only conversation graph.
type Message struct {
	Role     string     `json:"role"`
	Content  string     `json:"content"`
	SentAt   *time.Time `json:"sent_at,omitempty"`
	ParentIdx *int      `json:"parent_index,omitempty"` // index into the owning Branch.Messages
	ExternalRef string  `json:"external_ref,omitempty"`
}

// Conversation is the canonical unit; DedupKey is (provider_slug, external identity)
// and ContentHash is the full-content fingerprint filled in by ParseImport.
type Conversation struct {
	Title        string    `json:"title"`
	ProviderSlug string    `json:"provider_slug"`
	ExternalID   string    `json:"external_id,omitempty"`
	DedupKey     string    `json:"dedup_key,omitempty"`
	ContentHash  string    `json:"content_hash,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	Branches     []Branch  `json:"branches"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
}

// totalMessages counts every message across branches.
func (c *Conversation) totalMessages() int {
	n := 0
	for _, b := range c.Branches {
		n += len(b.Messages)
	}
	return n
}

// Import is a parsed batch ready for persistence.
type Import struct {
	SourceType   string
	Conversations []Conversation
	Projects     []ArchiveProject
}

func validRole(role string) bool {
	switch role {
	case "user", "assistant", "system", "tool":
		return true
	}
	return false
}

func validateConversation(c *Conversation) error {
	c.Title = strings.TrimSpace(c.Title)
	if c.Title == "" {
		c.Title = "未命名会话"
	}
	if len([]rune(c.Title)) > MaxTitleRunes {
		return fmt.Errorf("%w: title too long", ErrFormat)
	}
	c.ProviderSlug = strings.ToLower(strings.TrimSpace(c.ProviderSlug))
	if c.ProviderSlug == "" || len(c.ProviderSlug) > 40 {
		return fmt.Errorf("%w: provider slug required", ErrFormat)
	}
	if len(c.ExternalID) > 200 || len(c.ProjectName) > 300 {
		return fmt.Errorf("%w: identity fields too long", ErrFormat)
	}
	if len(c.Branches) == 0 {
		return fmt.Errorf("%w: conversation has no branch", ErrFormat)
	}
	if len(c.Branches) > MaxBranchesPerConv {
		return ErrTooMany
	}
	seen := map[string]bool{}
	for i := range c.Branches {
		b := &c.Branches[i]
		b.Name = strings.TrimSpace(b.Name)
		if b.Name == "" {
			b.Name = "main"
		}
		if seen[b.Name] {
			return fmt.Errorf("%w: duplicate branch %q", ErrFormat, b.Name)
		}
		seen[b.Name] = true
		if len(b.Messages) == 0 {
			return fmt.Errorf("%w: branch %q is empty", ErrFormat, b.Name)
		}
		if len(b.Messages) > MaxMessagesPerConv {
			return ErrTooMany
		}
		for j := range b.Messages {
			m := &b.Messages[j]
			if !validRole(m.Role) {
				return fmt.Errorf("%w: invalid role %q", ErrFormat, m.Role)
			}
			if len([]rune(m.Content)) > MaxContentRunes {
				return fmt.Errorf("%w: message content too long", ErrFormat)
			}
			if m.ParentIdx != nil && (*m.ParentIdx < 0 || *m.ParentIdx >= j) {
				// Parents must reference an earlier message in the same branch; cycles are impossible then.
				return fmt.Errorf("%w: parent_index must point backwards", ErrFormat)
			}
		}
	}
	return nil
}

// Validate checks the whole batch; it mutates defaults in place so callers persist what was validated.
func (im *Import) Validate() error {
	if im.SourceType == "" {
		return fmt.Errorf("%w: source type required", ErrFormat)
	}
	if len(im.Conversations) == 0 {
		return fmt.Errorf("%w: no conversations", ErrFormat)
	}
	if len(im.Conversations) > MaxConversations {
		return ErrTooMany
	}
	for i := range im.Conversations {
		if e := validateConversation(&im.Conversations[i]); e != nil {
			return fmt.Errorf("conversation %d: %w", i+1, e)
		}
	}
	return nil
}
