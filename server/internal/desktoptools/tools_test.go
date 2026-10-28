package desktoptools

import (
	cfg "aihub.dev/server/internal/config"
	conv "aihub.dev/server/internal/conversation"
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func archiveFixture(t *testing.T) []byte {
	t.Helper()
	out, e := conv.BuildArchive([]conv.Conversation{{Title: "Fixture", ProviderSlug: "fixture", ExternalID: "stable", Branches: []conv.Branch{{Name: "main", Messages: []conv.Message{{Role: "user", Content: "hello"}}}}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func zipFixture(t *testing.T, name string, data []byte, mode os.FileMode) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: name, Method: zip.Store}
	h.SetMode(mode)
	w, e := z.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	w.Write(data)
	z.Close()
	return b.Bytes()
}
func TestArchivePlanAndJSONLRoundTrip(t *testing.T) {
	raw := archiveFixture(t)
	plan, e := PlanImport("archive.json", raw)
	if e != nil || len(plan.Packets) != 1 {
		t.Fatalf("%+v %v", plan, e)
	}
	im, e := conv.ParseImport("archive", []byte(plan.Packets[0].Body))
	if e != nil {
		t.Fatal(e)
	}
	jsonl, e := conv.BuildJSONL(im.Conversations)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := PlanImport("conversations.jsonl", jsonl)
	if e != nil {
		t.Fatal(e)
	}
	out, e := conv.ParseImport("archive", []byte(replay.Packets[0].Body))
	if e != nil || out.Conversations[0].ContentHash != im.Conversations[0].ContentHash {
		t.Fatal("round-trip changed graph", e)
	}
	zipped, e := PlanImport("export.zip", zipFixture(t, "archive.json", raw, 0o600))
	if e != nil || zipped.Packets[0].Messages != 1 {
		t.Fatal(e)
	}
}
func TestZIPAndVersionFailClosed(t *testing.T) {
	raw := archiveFixture(t)
	for _, name := range []string{"../archive.json", "/archive.json", "x/../../archive.json", "C:/archive.json", "x\\archive.json"} {
		if _, e := PlanImport("bad.zip", zipFixture(t, name, raw, 0o600)); e == nil {
			t.Fatal(name)
		}
	}
	if _, e := PlanImport("bad.zip", zipFixture(t, "archive.json", raw, os.ModeSymlink|0o777)); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := PlanImport("future.json", []byte(strings.Replace(string(raw), `"version": 1`, `"version": 9`, 1))); e == nil {
		t.Fatal("future schema accepted")
	}
	if _, e := PlanImport("empty.json", nil); e == nil {
		t.Fatal("empty import accepted")
	}
}
func TestConfigPreviewDropsSecretsBeforeNetwork(t *testing.T) {
	raw := []byte(`{"mcpServers":{"demo":{"command":"tool","args":["--api-key","fixture-private-value"],"env":{"API_KEY":"fixture-secret","VENDOR_CUSTOM":"opaque-sensitive"}}}}`)
	out, e := PreviewConfig("claude_desktop", "", raw)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(out)
	for _, v := range []string{"fixture-private-value", "fixture-secret", "opaque-sensitive"} {
		if bytes.Contains(encoded, []byte(v)) {
			t.Fatal("secret leaked")
		}
	}
	if !bytes.Contains(encoded, []byte("secret_ref")) || len(out.Loss) < 3 {
		t.Fatal("missing references or loss report")
	}
	toml, _, e := cfg.TransformToCodex(out.Content)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = PreviewConfig("codex_cli", "", []byte(toml)); e != nil {
		t.Fatal(e)
	}
}
