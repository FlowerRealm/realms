package openai

import (
	"net/http/httptest"
	"testing"
)

func TestShouldEnableStickyRouting_UserAgent(t *testing.T) {
	r := httptest.NewRequest("POST", "http://example.com/v1/responses", nil)
	r.Header.Set("User-Agent", "codex_exec/0.106.0 (Ubuntu 24.4.0; x86_64) xterm-256color")

	payload := map[string]any{
		"model": "gpt-5.2",
		"input": []any{},
	}
	if !shouldEnableStickyRouting(payload, r, "payload") {
		t.Fatalf("expected sticky routing to be enabled for codex user-agent")
	}
}

func TestShouldEnableStickyRouting_SessionIDHeader(t *testing.T) {
	r := httptest.NewRequest("POST", "http://example.com/v1/responses", nil)
	r.Header.Set("session_id", "019c9f27-3c5a-7b93-90ee-a8a7eceff232")

	payload := map[string]any{
		"model": "gpt-5.2",
		"input": []any{},
	}
	if !shouldEnableStickyRouting(payload, r, "header") {
		t.Fatalf("expected sticky routing to be enabled for session_id header")
	}
}

func TestShouldEnableStickyRouting_PromptCacheKey(t *testing.T) {
	r := httptest.NewRequest("POST", "http://example.com/v1/responses", nil)

	payload := map[string]any{
		"model":            "gpt-5.2",
		"input":            []any{},
		"prompt_cache_key": "rk_test_123",
	}
	if !shouldEnableStickyRouting(payload, r, "payload") {
		t.Fatalf("expected sticky routing to be enabled for prompt_cache_key")
	}
}

func TestShouldEnableStickyRouting_DerivedIsDisabled(t *testing.T) {
	r := httptest.NewRequest("POST", "http://example.com/v1/responses", nil)
	r.Header.Set("User-Agent", "codex_exec/0.106.0 (Ubuntu 24.4.0; x86_64) xterm-256color")

	payload := map[string]any{
		"model": "gpt-5.2",
		"input": []any{},
	}
	if shouldEnableStickyRouting(payload, r, "derived") {
		t.Fatalf("expected derived route key to disable sticky routing")
	}
}
