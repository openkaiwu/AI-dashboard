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
	Manifest() Manifest
}

type Manifest struct {
	Slug         string   `json:"slug"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Acquisition  string   `json:"acquisition"`
	Status       string   `json:"status"`
}

type codexConnector struct{}

func (codexConnector) Slug() string       { return SlugCodex }
func (codexConnector) UploadPath() string { return "/api/v1/codex/snapshot" }
func (codexConnector) Manifest() Manifest {
	return Manifest{Slug: SlugCodex, Version: "m2-1", Capabilities: []string{"quota.snapshot", "quota.history", "plan.manual"}, Acquisition: "desktop_bridge", Status: "available"}
}

type cursorConnector struct{}

func (cursorConnector) Slug() string       { return SlugCursor }
func (cursorConnector) UploadPath() string { return "/api/v1/cursor/snapshot" }
func (cursorConnector) Manifest() Manifest {
	return Manifest{Slug: SlugCursor, Version: "m2-1", Capabilities: []string{"quota.snapshot", "quota.history"}, Acquisition: "desktop_bridge", Status: "partial"}
}

// LocalConnectors returns the M0 dual-target bridge connectors.
func LocalConnectors() []Connector {
	return []Connector{codexConnector{}, cursorConnector{}}
}
