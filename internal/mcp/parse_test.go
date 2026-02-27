package mcp

import "testing"

func TestParseTargetContentToStoreV2_ClaudeJSON(t *testing.T) {
	raw := `{
  "mcpServers": {
    "a": { "type": "stdio", "command": "npx", "args": ["-y", "x"] }
  }
}`
	s, err := ParseTargetContentToStoreV2("claude", raw)
	if err != nil {
		t.Fatalf("ParseTargetContentToStoreV2: %v", err)
	}
	if s.Version != 2 {
		t.Fatalf("version = %d, want 2", s.Version)
	}
	sv, ok := s.Servers["a"]
	if !ok {
		t.Fatalf("missing server a")
	}
	if sv.Transport != "stdio" {
		t.Fatalf("transport = %q, want stdio", sv.Transport)
	}
	if sv.Stdio == nil || sv.Stdio.Command != "npx" {
		t.Fatalf("stdio.command = %#v, want npx", sv.Stdio)
	}
}

func TestParseTargetContentToStoreV2_GeminiJSON_HTTPUrl(t *testing.T) {
	raw := `{
  "mcpServers": {
    "b": { "httpUrl": "http://127.0.0.1:9999/mcp" }
  }
}`
	s, err := ParseTargetContentToStoreV2("gemini", raw)
	if err != nil {
		t.Fatalf("ParseTargetContentToStoreV2: %v", err)
	}
	sv, ok := s.Servers["b"]
	if !ok {
		t.Fatalf("missing server b")
	}
	if sv.Transport != "http" {
		t.Fatalf("transport = %q, want http", sv.Transport)
	}
	if sv.HTTP == nil || sv.HTTP.URL != "http://127.0.0.1:9999/mcp" {
		t.Fatalf("http.url = %#v", sv.HTTP)
	}
}

func TestParseTargetContentToStoreV2_CodexTOML_URLOnlyBecomesHTTP(t *testing.T) {
	raw := `
[mcp_servers.c]
url = "http://127.0.0.1:9999/mcp"
`
	s, err := ParseTargetContentToStoreV2("codex", raw)
	if err != nil {
		t.Fatalf("ParseTargetContentToStoreV2: %v", err)
	}
	sv, ok := s.Servers["c"]
	if !ok {
		t.Fatalf("missing server c")
	}
	if sv.Transport != "http" {
		t.Fatalf("transport = %q, want http", sv.Transport)
	}
	if sv.HTTP == nil || sv.HTTP.URL != "http://127.0.0.1:9999/mcp" {
		t.Fatalf("http.url = %#v", sv.HTTP)
	}
}

func TestParseTargetContentToStoreV2_RealmsStoreV2JSON(t *testing.T) {
	raw := `{
  "version": 2,
  "servers": {
    "d": { "transport": "sse", "http": { "url": "http://127.0.0.1:9999/sse" } }
  }
}`
	s, err := ParseTargetContentToStoreV2("realms", raw)
	if err != nil {
		t.Fatalf("ParseTargetContentToStoreV2: %v", err)
	}
	sv, ok := s.Servers["d"]
	if !ok {
		t.Fatalf("missing server d")
	}
	if sv.Transport != "sse" {
		t.Fatalf("transport = %q, want sse", sv.Transport)
	}
	if sv.HTTP == nil || sv.HTTP.URL != "http://127.0.0.1:9999/sse" {
		t.Fatalf("http.url = %#v", sv.HTTP)
	}
}
