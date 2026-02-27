package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanTarget_Codex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
model = "gpt-5-codex"

[mcp_servers.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res := ScanTarget(TargetCodex, path)
	if !res.Exists {
		t.Fatalf("expected exists")
	}
	if res.ParseError != "" {
		t.Fatalf("parse_error=%s", res.ParseError)
	}
	if res.ServerCount != 1 {
		t.Fatalf("server_count=%d", res.ServerCount)
	}
	if res.Servers["context7"]["type"] != "stdio" {
		t.Fatalf("type=%v", res.Servers["context7"]["type"])
	}
}

func TestScanTarget_Claude(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"s1":{"command":"echo","args":["ok"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res := ScanTarget(TargetClaude, path)
	if res.ParseError != "" {
		t.Fatalf("parse_error=%s", res.ParseError)
	}
	if res.ServerCount != 1 {
		t.Fatalf("server_count=%d", res.ServerCount)
	}
	if res.Servers["s1"]["type"] != "stdio" {
		t.Fatalf("type=%v", res.Servers["s1"]["type"])
	}
}

func TestScanTarget_GeminiHttpUrl(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"g1":{"httpUrl":"https://example.com/mcp"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res := ScanTarget(TargetGemini, path)
	if res.ParseError != "" {
		t.Fatalf("parse_error=%s", res.ParseError)
	}
	if res.ServerCount != 1 {
		t.Fatalf("server_count=%d", res.ServerCount)
	}
	if res.Servers["g1"]["type"] != "http" {
		t.Fatalf("type=%v", res.Servers["g1"]["type"])
	}
}
