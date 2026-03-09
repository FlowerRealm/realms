package upstream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedOpencodeInstructionsCache(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := filepath.Join(home, ".opencode", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write cache content: %v", err)
	}
	metaBytes, _ := json.Marshal(opencodeCacheMetadata{LastChecked: time.Now().UnixMilli()})
	if err := os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header-meta.json"), metaBytes, 0o644); err != nil {
		t.Fatalf("write cache meta: %v", err)
	}
}

func TestApplyCodexOAuthTransform_Basics(t *testing.T) {
	seedOpencodeInstructionsCache(t, "cached-instructions")

	reqBody := map[string]any{
		"model":             "gpt-5.4",
		"stream":            false,
		"store":             true,
		"max_output_tokens": 123,
		"temperature":       0.5,
		"input": []any{
			map[string]any{"type": "tool_call", "id": "call1"},
			map[string]any{"type": "input_text", "text": "hi", "id": "t1", "call_id": "c1"},
		},
	}

	out := applyCodexOAuthTransform(reqBody, false)
	if !out.Modified {
		t.Fatalf("expected Modified=true")
	}
	if got := strings.TrimSpace(reqBody["model"].(string)); got != "gpt-5.4" {
		t.Fatalf("model = %q, want %q", got, "gpt-5.4")
	}
	if v, ok := reqBody["store"].(bool); !ok || v != false {
		t.Fatalf("store = (%T)%v, want bool(false)", reqBody["store"], reqBody["store"])
	}
	if v, ok := reqBody["stream"].(bool); !ok || v != true {
		t.Fatalf("stream = (%T)%v, want bool(true)", reqBody["stream"], reqBody["stream"])
	}
	for _, k := range []string{"max_output_tokens", "temperature"} {
		if _, ok := reqBody[k]; ok {
			t.Fatalf("expected %s to be stripped", k)
		}
	}
	if got := strings.TrimSpace(stringFromAny(reqBody["instructions"])); got != "cached-instructions" {
		t.Fatalf("instructions = %q, want %q", got, "cached-instructions")
	}

	input, ok := reqBody["input"].([]any)
	if !ok {
		t.Fatalf("input = %#v", reqBody["input"])
	}
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(stringFromAny(m["type"]))
		if _, ok := m["id"]; ok && !isCodexToolCallItemType(strings.TrimSpace(stringFromAny(m["type"]))) {
			t.Fatalf("expected id to be stripped for non-tool-call items: %#v", m)
		}
		if typ == "tool_call" {
			if strings.TrimSpace(stringFromAny(m["call_id"])) == "" {
				t.Fatalf("expected tool_call.call_id to be set: %#v", m)
			}
			if strings.TrimSpace(stringFromAny(m["id"])) != "" {
				t.Fatalf("expected tool_call.id to be stripped: %#v", m)
			}
		}
	}
}

func TestApplyCodexOAuthTransform_NormalizeTools(t *testing.T) {
	seedOpencodeInstructionsCache(t, "cached-instructions")

	reqBody := map[string]any{
		"model":  "gpt-5.1",
		"stream": true,
		"store":  false,
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "f",
					"description": "d",
					"parameters":  map[string]any{"type": "object"},
					"strict":      true,
				},
			},
		},
	}

	applyCodexOAuthTransform(reqBody, false)

	tools, ok := reqBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", reqBody["tools"])
	}
	tool0, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tools[0] = %#v", tools[0])
	}
	if got := strings.TrimSpace(stringFromAny(tool0["name"])); got != "f" {
		t.Fatalf("tool.name = %q, want %q", got, "f")
	}
	if tool0["parameters"] == nil {
		t.Fatalf("expected tool.parameters to be set")
	}
}

func TestApplyCodexOAuthTransform_PreservesReferencesWhenItemReferencePresent(t *testing.T) {
	seedOpencodeInstructionsCache(t, "cached-instructions")

	reqBody := map[string]any{
		"model":  "gpt-5.1",
		"stream": true,
		"store":  false,
		"input": []any{
			map[string]any{"type": "item_reference", "id": "ref1"},
			map[string]any{"type": "tool_call", "id": "call1"},
		},
	}

	applyCodexOAuthTransform(reqBody, false)

	input, ok := reqBody["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("input = %#v", reqBody["input"])
	}
	item0, _ := input[0].(map[string]any)
	if strings.TrimSpace(stringFromAny(item0["type"])) != "item_reference" {
		t.Fatalf("expected item_reference to be preserved, got %#v", item0)
	}
}

func TestApplyCodexOAuthTransform_CLI_CanonicalizesInstructionAlias(t *testing.T) {
	seedOpencodeInstructionsCache(t, "cached-instructions")

	reqBody := map[string]any{
		"model":       "gpt-5.2",
		"stream":      true,
		"store":       false,
		"input":       "hi",
		"instruction": "user-instruction",
	}

	applyCodexOAuthTransform(reqBody, true)

	if got := strings.TrimSpace(stringFromAny(reqBody["instructions"])); got != "user-instruction" {
		t.Fatalf("instructions = %q, want %q", got, "user-instruction")
	}
	if _, ok := reqBody["instruction"]; ok {
		t.Fatalf("expected instruction alias to be removed, got %#v", reqBody["instruction"])
	}
}

func TestApplyCodexOAuthTransform_CLI_PrefersInstructionsOverInstructionAlias(t *testing.T) {
	seedOpencodeInstructionsCache(t, "cached-instructions")

	reqBody := map[string]any{
		"model":        "gpt-5.2",
		"stream":       true,
		"store":        false,
		"input":        "hi",
		"instruction":  "alias-instruction",
		"instructions": "canonical-instruction",
	}

	applyCodexOAuthTransform(reqBody, true)

	if got := strings.TrimSpace(stringFromAny(reqBody["instructions"])); got != "canonical-instruction" {
		t.Fatalf("instructions = %q, want %q", got, "canonical-instruction")
	}
	if _, ok := reqBody["instruction"]; ok {
		t.Fatalf("expected instruction alias to be removed, got %#v", reqBody["instruction"])
	}
}

func TestApplyCodexOAuthTransform_NonCLI_PrefixesExistingInstructionsWithoutDuplication(t *testing.T) {
	seedOpencodeInstructionsCache(t, "cached-instructions")

	reqBody := map[string]any{
		"model":        "gpt-5.2",
		"stream":       true,
		"store":        false,
		"input":        "hi",
		"instructions": "user-instruction",
	}

	applyCodexOAuthTransform(reqBody, false)
	first := strings.TrimSpace(stringFromAny(reqBody["instructions"]))
	if first != "cached-instructions\n\nuser-instruction" {
		t.Fatalf("instructions = %q, want %q", first, "cached-instructions\n\nuser-instruction")
	}

	applyCodexOAuthTransform(reqBody, false)
	second := strings.TrimSpace(stringFromAny(reqBody["instructions"]))
	if second != first {
		t.Fatalf("expected instructions prefix to be deduplicated, got %q want %q", second, first)
	}
}
