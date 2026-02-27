package mcp

import (
	"strings"
	"testing"
)

func TestParseRegistryJSON_InferType(t *testing.T) {
	reg, err := ParseRegistryJSON(`{
  "s1": {"command":"npx","args":["-y","foo"]},
  "s2": {"url":"https://example.com/mcp"}
}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON error: %v", err)
	}
	if reg["s1"]["type"] != "stdio" {
		t.Fatalf("s1 type=%v", reg["s1"]["type"])
	}
	if reg["s2"]["type"] != "sse" {
		t.Fatalf("s2 type=%v", reg["s2"]["type"])
	}
}

func TestParseRegistryJSON_GeminiHttpUrl(t *testing.T) {
	reg, err := ParseRegistryJSON(`{
  "g1": {"httpUrl":"https://example.com/mcp"}
}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON error: %v", err)
	}
	if reg["g1"]["type"] != "http" {
		t.Fatalf("type=%v", reg["g1"]["type"])
	}
	if reg["g1"]["url"] != "https://example.com/mcp" {
		t.Fatalf("url=%v", reg["g1"]["url"])
	}
}

func TestExportClaudeConfig_WindowsWrap(t *testing.T) {
	reg, err := ParseRegistryJSON(`{"s1":{"command":"npx","args":["-y","foo"]}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON error: %v", err)
	}
	cfg, err := ExportClaudeConfig(reg, "windows", true)
	if err != nil {
		t.Fatalf("ExportClaudeConfig error: %v", err)
	}
	mcpServers := cfg["mcpServers"].(map[string]any)
	s1 := mcpServers["s1"].(map[string]any)
	if s1["command"] != "cmd" {
		t.Fatalf("command=%v", s1["command"])
	}
	args := s1["args"].([]any)
	if len(args) < 3 || args[0] != "/c" || args[1] != "npx" {
		t.Fatalf("args=%v", args)
	}
}

func TestExportGeminiConfig_TimeoutConversion(t *testing.T) {
	reg, err := ParseRegistryJSON(`{"s1":{"command":"node","startup_timeout_sec":2,"tool_timeout_sec":3}}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON error: %v", err)
	}
	cfg, err := ExportGeminiConfig(reg)
	if err != nil {
		t.Fatalf("ExportGeminiConfig error: %v", err)
	}
	mcpServers := cfg["mcpServers"].(map[string]any)
	s1 := mcpServers["s1"].(map[string]any)
	if _, ok := s1["type"]; ok {
		t.Fatalf("type should be removed for gemini")
	}
	if s1["timeout"] != uint64(3000) && s1["timeout"] != float64(3000) && s1["timeout"] != int(3000) {
		t.Fatalf("timeout=%T %v", s1["timeout"], s1["timeout"])
	}
}

func TestExportCodexConfigTOML_Basic(t *testing.T) {
	reg, err := ParseRegistryJSON(`{
  "context7": { "command":"npx", "args":["-y","@upstash/context7-mcp"], "startup_timeout_sec": 5, "tool_timeout_sec": 30, "env": { "FOO": "bar" } },
  "figma-dev": { "type":"http", "url":"http://127.0.0.1:3845/mcp", "bearer_token_env_var":"FIGMA_TOKEN", "http_headers": { "X-Figma-Region": "us-east-1" } }
}`)
	if err != nil {
		t.Fatalf("ParseRegistryJSON error: %v", err)
	}
	out, err := ExportCodexConfigTOML(reg, "")
	if err != nil {
		t.Fatalf("ExportCodexConfigTOML error: %v", err)
	}
	if out == "" {
		t.Fatalf("empty output")
	}
	if !containsAll(out,
		`[mcp_servers.context7]`,
		`command = "npx"`,
		`args = ["-y", "@upstash/context7-mcp"]`,
		`startup_timeout_sec = 5`,
		`tool_timeout_sec = 30`,
		`[mcp_servers.context7.env]`,
		`FOO = "bar"`,
		`[mcp_servers.figma-dev]`,
		`url = "http://127.0.0.1:3845/mcp"`,
		`bearer_token_env_var = "FIGMA_TOKEN"`,
		`http_headers = { "X-Figma-Region" = "us-east-1" }`,
	) {
		t.Fatalf("unexpected toml:\n%s", out)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
