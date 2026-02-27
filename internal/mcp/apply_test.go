package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCodexConfig_PreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("model = \"gpt-5-codex\"\n[mcp_servers.old]\ncommand = \"echo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reg, err := ParseRegistryJSON(`{"context7":{"command":"npx","args":["-y","@upstash/context7-mcp"]}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON: %v", err)
	}

	changed, err := ApplyCodexConfig(path, reg, nil, "linux", false)
	if err != nil {
		t.Fatalf("ApplyCodexConfig: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "model") {
		t.Fatalf("expected model preserved, got:\n%s", s)
	}
	// Merge mode preserves servers not present in registry.
	if !strings.Contains(s, "[mcp_servers.old]") {
		t.Fatalf("expected old mcp server preserved, got:\n%s", s)
	}
	if !strings.Contains(s, "[mcp_servers.context7]") {
		t.Fatalf("expected new mcp server, got:\n%s", s)
	}
}

func TestApplyCodexConfig_InvalidTomlForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("model = \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistryJSON(`{"s1":{"command":"echo","args":["ok"]}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON: %v", err)
	}
	_, err = ApplyCodexConfig(path, reg, nil, "linux", false)
	if err == nil {
		t.Fatalf("expected error without force")
	}
	_, err = ApplyCodexConfig(path, reg, nil, "linux", true)
	if err != nil {
		t.Fatalf("expected success with force, got: %v", err)
	}
}

func TestApplyClaudeConfig_OnlyWritesMcpServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(path, []byte("{\"foo\":1,\"mcpServers\":{\"old\":{}}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistryJSON(`{"s1":{"command":"echo","args":["ok"]}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON: %v", err)
	}
	_, err = ApplyClaudeConfig(path, reg, nil, "linux", false)
	if err != nil {
		t.Fatalf("ApplyClaudeConfig: %v", err)
	}
	raw, _ := os.ReadFile(path)
	s := string(raw)
	if !strings.Contains(s, "\"foo\": 1") {
		t.Fatalf("expected foo preserved, got:\n%s", s)
	}
	// Merge mode preserves servers not present in registry.
	if !strings.Contains(s, "\"old\"") {
		t.Fatalf("expected old preserved, got:\n%s", s)
	}
	if !strings.Contains(s, "\"s1\"") {
		t.Fatalf("expected s1 present, got:\n%s", s)
	}
}

func TestApplyGeminiConfig_OnlyWritesMcpServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{\"foo\":true,\"mcpServers\":{\"old\":{}}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistryJSON(`{"s1":{"type":"http","url":"https://example.com/mcp","tool_timeout_sec":2}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON: %v", err)
	}
	_, err = ApplyGeminiConfig(path, reg, nil, false)
	if err != nil {
		t.Fatalf("ApplyGeminiConfig: %v", err)
	}
	raw, _ := os.ReadFile(path)
	s := string(raw)
	if !strings.Contains(s, "\"foo\": true") {
		t.Fatalf("expected foo preserved, got:\n%s", s)
	}
	// Merge mode preserves servers not present in registry.
	if !strings.Contains(s, "\"old\"") {
		t.Fatalf("expected old preserved, got:\n%s", s)
	}
	if !strings.Contains(s, "\"s1\"") {
		t.Fatalf("expected s1 present, got:\n%s", s)
	}
	if !strings.Contains(s, "httpUrl") {
		t.Fatalf("expected httpUrl conversion, got:\n%s", s)
	}
}

func TestApplyClaudeConfig_RemoveIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(path, []byte("{\"mcpServers\":{\"old\":{\"command\":\"echo\"}}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistryJSON(`{"s1":{"command":"echo","args":["ok"]}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON: %v", err)
	}
	_, err = ApplyClaudeConfig(path, reg, []string{"old"}, "linux", false)
	if err != nil {
		t.Fatalf("ApplyClaudeConfig: %v", err)
	}
	raw, _ := os.ReadFile(path)
	s := string(raw)
	if strings.Contains(s, "\"old\"") {
		t.Fatalf("expected old removed, got:\n%s", s)
	}
	if !strings.Contains(s, "\"s1\"") {
		t.Fatalf("expected s1 present, got:\n%s", s)
	}
}
