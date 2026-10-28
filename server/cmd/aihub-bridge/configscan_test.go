package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigScanDoesNotFollowSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	content := []byte(`{"mcpServers":{"x":{"command":"tool","env":{"API_KEY":"sensitive"}}}}`)
	target := filepath.Join(outside, "claude_desktop_config.json")
	if e := os.WriteFile(target, content, 0o600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, filepath.Join(root, "claude_desktop_config.json")); e != nil {
		t.Skip("OS does not allow symlinks:", e)
	}
	if got := collectConfigScan([]string{root}); len(got) != 0 {
		t.Fatal("file symlink escaped authorized directory")
	}
	if e := os.Symlink(outside, filepath.Join(root, "linked")); e != nil {
		t.Fatal(e)
	}
	if got := collectConfigScan([]string{root}); len(got) != 0 {
		t.Fatal("directory symlink escaped authorized directory")
	}
	if got := collectConfigScan([]string{filepath.Join(root, "linked")}); len(got) != 0 {
		t.Fatal("root symlink accepted")
	}
	if e := os.WriteFile(filepath.Join(root, "mcp.json"), content, 0o600); e != nil {
		t.Fatal(e)
	}
	got := collectConfigScan([]string{root})
	if len(got) != 1 || len(got[0].SecretKeys) != 1 || got[0].SecretKeys[0] != "API_KEY" {
		t.Fatalf("ordinary authorized config unavailable: %+v", got)
	}
}
