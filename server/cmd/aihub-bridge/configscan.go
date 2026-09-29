package main

// Config discovery (M4/INH-467): scan only directories explicitly authorized in
// bridge.json `scan_directories`, match known provider config files, classify
// secret KEY NAMES, and upload the sanitized manifest. File contents and secret
// values never leave this process.

import (
	cfgpkg "aihub.dev/server/internal/config"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	scanMaxFiles      = 500
	scanMaxDepth      = 3
	scanMaxFileBytes  = 256 * 1024
	scanMaxEntries    = 100
)

type knownFile struct {
	Platform string
	Format   string
}

// matchKnown recognizes a small whitelist of well-known config files.
func matchKnown(path, name string) (knownFile, bool) {
	parent := filepath.Base(filepath.Dir(path))
	switch {
	case name == "claude_desktop_config.json":
		return knownFile{"claude_desktop", "json"}, true
	case parent == ".codex" && name == "config.toml":
		return knownFile{"codex_cli", "toml"}, true
	case parent == ".cursor" && name == "mcp.json":
		return knownFile{"cursor", "json"}, true
	case name == "mcp.json" || name == ".mcp.json":
		return knownFile{"mcp_generic", "json"}, true
	}
	return knownFile{}, false
}

var scanSkipDirs = map[string]bool{".git": true, "node_modules": true, "__pycache__": true, "venv": true, ".venv": true}

func collectConfigScan(roots []string) []cfgpkg.ScanEntry {
	entries := []cfgpkg.ScanEntry{}
	for _, root := range roots {
		info, e := os.Stat(root)
		if e != nil || !info.IsDir() {
			continue
		}
		count := 0
		base := filepath.Clean(root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if count >= scanMaxFiles || len(entries) >= scanMaxEntries {
				return fs.SkipAll
			}
			if d.IsDir() {
				if scanSkipDirs[d.Name()] && path != base {
					return fs.SkipDir
				}
				if strings.Count(strings.TrimPrefix(strings.TrimPrefix(path, base), string(os.PathSeparator)), string(os.PathSeparator)) >= scanMaxDepth {
					return fs.SkipDir
				}
				return nil
			}
			count++
			match, ok := matchKnown(path, d.Name())
			if !ok {
				return nil
			}
			fi, e := d.Info()
			if e != nil || fi.Size() > scanMaxFileBytes {
				return nil
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return nil
			}
			mod := fi.ModTime().UTC()
			keys := cfgpkg.SecretKeyNames(data, match.Format)
			sort.Strings(keys)
			entries = append(entries, cfgpkg.ScanEntry{Path: path, Platform: match.Platform,
				SizeBytes: fi.Size(), ModifiedAt: &mod, SecretKeys: keys, DetectedFormat: match.Format})
			return nil
		})
	}
	return entries
}
