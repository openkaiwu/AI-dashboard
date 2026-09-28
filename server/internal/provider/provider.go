// Package provider defines shared connector identifiers for local acquisition.
package provider

const (
	SlugCodex  = "codex"
	SlugCursor = "cursor"
)

// Connector describes a local bridge upload target.
type Connector interface {
	Slug() string
	UploadPath() string
}

type codexConnector struct{}

func (codexConnector) Slug() string       { return SlugCodex }
func (codexConnector) UploadPath() string { return "/api/v1/codex/snapshot" }

type cursorConnector struct{}

func (cursorConnector) Slug() string       { return SlugCursor }
func (cursorConnector) UploadPath() string { return "/api/v1/cursor/snapshot" }

// LocalConnectors returns the M0 dual-target bridge connectors.
func LocalConnectors() []Connector {
	return []Connector{codexConnector{}, cursorConnector{}}
}
