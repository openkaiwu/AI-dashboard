package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ArchiveFormat is the frozen canonical archive envelope (aihub.conversation-archive v1).
const ArchiveFormat = "aihub.conversation-archive"

// ArchiveProject carries project-level metadata through export and re-import.
type ArchiveProject struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ExternalSource string `json:"external_source,omitempty"`
	ExternalID     string `json:"external_id,omitempty"`
}

type archiveEnvelope struct {
	Format        string           `json:"format"`
	Version       int              `json:"version"`
	ExportedAt    *time.Time       `json:"exported_at,omitempty"`
	Projects      []ArchiveProject `json:"projects,omitempty"`
	Conversations []Conversation   `json:"conversations"`
}

// BuildArchive renders the canonical archive v1 envelope for re-import round-trips.
func BuildArchive(conversations []Conversation, projects []ArchiveProject) ([]byte, error) {
	out, e := json.MarshalIndent(archiveEnvelope{
		Format:        ArchiveFormat,
		Version:       1,
		ExportedAt:    nowUTC(),
		Projects:      projects,
		Conversations: conversations,
	}, "", "  ")
	if e != nil {
		return nil, e
	}
	return append(out, '\n'), nil
}

// BuildMarkdown renders a human-readable transcript for one conversation.
func BuildMarkdown(c *Conversation) string {
	var b strings.Builder
	b.WriteString("# " + c.Title + "\n\n")
	fmt.Fprintf(&b, "- 来源: %s\n", c.ProviderSlug)
	if c.CreatedAt != nil {
		fmt.Fprintf(&b, "- 创建时间: %s\n", c.CreatedAt.UTC().Format(time.RFC3339))
	}
	b.WriteString("\n")
	for _, br := range c.Branches {
		if br.Name != "main" {
			fmt.Fprintf(&b, "## 分支 %s\n\n", br.Name)
		}
		for _, m := range br.Messages {
			fmt.Fprintf(&b, "### %s\n\n", m.Role)
			b.WriteString(strings.ReplaceAll(m.Content, "\r\n", "\n"))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func nowUTC() *time.Time {
	t := time.Now().UTC()
	return &t
}
