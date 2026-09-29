package api_test

import (
	"aihub.dev/server/internal/testdb"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestConfigPortabilitySecretHygieneTransformAndRollback(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	a := login(t, ts, true, "desktop")
	at := a["token"].(string)

	raw, e := os.ReadFile("testdata/claude_desktop_config.json")
	if e != nil {
		t.Fatal(e)
	}
	secret := "sk-super-secret-value-123"
	leak := func(phase string, body []byte) {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("%s leaked the secret value: %s", phase, body)
		}
	}

	// 1. Platform import classifies secrets: value replaced by a name-only ref.
	code, out := call(t, ts, "POST", "/api/v1/config-assets/import", at, map[string]string{"platform": "claude_desktop", "name": "Claude Desktop", "content": string(raw)})
	if code != 201 {
		t.Fatalf("import %d: %v", code, out)
	}
	assetID := out["id"].(string)
	if loss, ok := out["loss_report"].([]any); !ok || len(loss) == 0 {
		t.Fatalf("loss report empty: %v", out)
	}

	// 2. Canonical storage and every later response stay secret-free.
	code, detail := call(t, ts, "GET", "/api/v1/config-assets/"+assetID, at, nil)
	if code != 200 {
		t.Fatalf("get %d", code)
	}
	detailBytes, _ := json.Marshal(detail)
	leak("get", detailBytes)
	latest := detail["latest_content"].(map[string]any)
	servers := latest["mcp_servers"].([]any)
	filesystem := servers[0].(map[string]any)
	env := filesystem["env"].(map[string]any)
	tokenRef, ok := env["API_TOKEN"].(map[string]any)
	if !ok || tokenRef["secret_ref"] != "env:API_TOKEN" {
		t.Fatalf("secret ref missing: %v", env)
	}
	if env["NODE_OPTIONS"] != "--max-old-space-size=4096" {
		t.Fatalf("non-secret env lost: %v", env)
	}

	// 3. Reference transform claude_desktop → canonical → codex_cli is golden.
	code, tr := call(t, ts, "POST", "/api/v1/config-assets/"+assetID+"/transform", at, map[string]string{"target_platform": "codex_cli"})
	if code != 200 {
		t.Fatalf("transform %d: %v", code, tr)
	}
	rendered := tr["content"].(string)
	for _, golden := range []string{
		"[mcp_servers.filesystem]",
		"command = \"npx\"",
		"args = [\"-y\", \"@modelcontextprotocol/server-filesystem\", \"/tmp\"]",
		"[mcp_servers.filesystem.env]",
		"API_TOKEN = \"${API_TOKEN}\"",
		"[mcp_servers.remote-docs]",
		"url = \"https://docs.example.com/mcp\"",
	} {
		if !strings.Contains(rendered, golden) {
			t.Fatalf("golden transform missing %q in:\n%s", golden, rendered)
		}
	}
	trBytes, _ := json.Marshal(tr)
	leak("transform", trBytes)
	foundUnavailable := false
	for _, l := range tr["loss_report"].([]any) {
		entry := l.(map[string]any)
		if entry["reason"] == "secret_value_unavailable" {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatalf("secret loss not reported: %v", tr["loss_report"])
	}

	// 4. Versions are append-only; diff is semantic; rollback restores old content as a new version.
	changed := map[string]any{"mcp_servers": []any{map[string]any{
		"name": "filesystem", "command": "bun", "args": []any{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		"env": map[string]any{"NODE_OPTIONS": "--max-old-space-size=4096", "API_TOKEN": map[string]any{"secret_ref": "env:API_TOKEN"}},
	}}}
	code, out = call(t, ts, "POST", "/api/v1/config-assets/"+assetID+"/versions", at, map[string]any{"content": changed})
	if code != 201 || num(out["version"]) != 2 {
		t.Fatalf("add version %d: %v", code, out)
	}
	code, diff := call(t, ts, "GET", "/api/v1/config-assets/"+assetID+"/diff?from=1&to=2", at, nil)
	if code != 200 {
		t.Fatalf("diff %d", code)
	}
	foundCommand := false
	for _, d := range diff["diff"].([]any) {
		entry := d.(map[string]any)
		if entry["op"] == "change" && strings.Contains(entry["path"].(string), "command") {
			foundCommand = true
		}
	}
	if !foundCommand {
		t.Fatalf("command change missing from diff: %v", diff)
	}
	code, out = call(t, ts, "POST", "/api/v1/config-assets/"+assetID+"/rollback", at, map[string]any{"version": 1})
	if code != 201 || num(out["version"]) != 3 {
		t.Fatalf("rollback %d: %v", code, out)
	}
	code, detail = call(t, ts, "GET", "/api/v1/config-assets/"+assetID, at, nil)
	if code != 200 || num(detail["asset"].(map[string]any)["latest_version"]) != 3 {
		t.Fatalf("latest version wrong: %v", detail["asset"])
	}
	back := detail["latest_content"].(map[string]any)
	if back["mcp_servers"].([]any)[0].(map[string]any)["command"] != "npx" {
		t.Fatal("rollback did not restore v1 content")
	}
	versions := detail["versions"].([]any)
	if len(versions) != 3 {
		t.Fatalf("version history wrong: %v", versions)
	}
	createdBy := map[bool]string{}
	_ = createdBy
	for _, v := range versions {
		vm := v.(map[string]any)
		switch num(vm["version"]) {
		case 1:
			if vm["created_by"] != "import" {
				t.Fatalf("v1 provenance wrong: %v", vm)
			}
		case 2:
			if vm["created_by"] != "manual" {
				t.Fatalf("v2 provenance wrong: %v", vm)
			}
		case 3:
			if vm["created_by"] != "rollback" {
				t.Fatalf("v3 provenance wrong: %v", vm)
			}
		}
	}

	// 5. Full list responses remain secret-free.
	code, list := call(t, ts, "GET", "/api/v1/config-assets", at, nil)
	listBytes, _ := json.Marshal(list)
	leak("list", listBytes)

	// 6. Bridge config discovery stores key NAMES only and requires the bridge token.
	code, bridge := call(t, ts, "POST", "/api/v1/codex/bridges", at, map[string]string{"name": "Scan desktop"})
	if code != 201 {
		t.Fatalf("bridge create %d: %v", code, bridge)
	}
	bridgeToken := bridge["token"].(string)
	if code, _ := call(t, ts, "POST", "/api/v1/bridge/config-scan", "", map[string]any{"entries": []any{}}); code != 401 {
		t.Fatalf("unauthenticated scan %d", code)
	}
	manifest := map[string]any{"entries": []any{map[string]any{
		"path": "/home/user/.claude/claude_desktop_config.json", "platform": "claude_desktop",
		"size_bytes": 1024, "secret_keys": []string{"API_TOKEN"}, "detected_format": "json",
	}}}
	if code, out = call(t, ts, "POST", "/api/v1/bridge/config-scan", bridgeToken, manifest); code != 200 {
		t.Fatalf("scan %d: %v", code, out)
	}
	code, disc := call(t, ts, "GET", "/api/v1/config-discoveries", at, nil)
	if code != 200 {
		t.Fatalf("discoveries %d", code)
	}
	entries := disc["discoveries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("discovery rows wrong: %v", disc)
	}
	entry := entries[0].(map[string]any)
	keys, _ := entry["secret_keys"].([]any)
	if len(keys) != 1 || keys[0] != "API_TOKEN" {
		t.Fatalf("secret key names wrong: %v", entry)
	}
	discBytes, _ := json.Marshal(disc)
	leak("discoveries", discBytes)
}
