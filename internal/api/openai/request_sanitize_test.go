package openai

import (
	"errors"
	"testing"
)

func TestSanitizeMessagesPayload_PreservesUnknownAndAliasesMaxTokensToSample(t *testing.T) {
	payload, err := sanitizeMessagesPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_tokens_to_sample":12,"unknown":{"keep":true},"mcp_servers":[{"name":"svc"}]}`), 0, true)
	if err != nil {
		t.Fatalf("sanitizeMessagesPayload: %v", err)
	}

	if v, ok := payload["max_tokens"].(int64); !ok || v != 12 {
		t.Fatalf("expected max_tokens=12, got=%#v", payload["max_tokens"])
	}
	for _, key := range []string{"max_output_tokens", "max_completion_tokens", "max_tokens_to_sample"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("expected %s to be removed, got=%#v", key, payload[key])
		}
	}
	if _, ok := payload["unknown"].(map[string]any); !ok {
		t.Fatalf("expected unknown to be preserved, got=%#v", payload["unknown"])
	}
	if _, ok := payload["mcp_servers"].([]any); !ok {
		t.Fatalf("expected mcp_servers to be preserved, got=%#v", payload["mcp_servers"])
	}
}

func TestSanitizeMessagesPayload_StripsMCPServersWhenDisabled(t *testing.T) {
	payload, err := sanitizeMessagesPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_tokens":7,"mcp_servers":[{"name":"svc"}]}`), 0, false)
	if err != nil {
		t.Fatalf("sanitizeMessagesPayload: %v", err)
	}
	if _, ok := payload["mcp_servers"]; ok {
		t.Fatalf("expected mcp_servers to be removed, got=%#v", payload["mcp_servers"])
	}
}

func TestSanitizeMessagesPayload_RejectsInvalidTokenAliasType(t *testing.T) {
	_, err := sanitizeMessagesPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_tokens_to_sample":"bad"}`), 0, true)
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got=%v", err)
	}
}

func TestSanitizeChatCompletionsPayload_PreservesUnknownAndNormalizesSpecialFields(t *testing.T) {
	payload, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_output_tokens":9,"unknown":123,"stream_options":{"include_usage":false},"web_search_options":{}}`), 0)
	if err != nil {
		t.Fatalf("sanitizeChatCompletionsPayload: %v", err)
	}

	if v, ok := payload["max_tokens"].(int64); !ok || v != 9 {
		t.Fatalf("expected max_tokens=9, got=%#v", payload["max_tokens"])
	}
	if _, ok := payload["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be removed, got=%#v", payload["max_output_tokens"])
	}
	if _, ok := payload["stream_options"]; ok {
		t.Fatalf("expected stream_options to be removed for non-stream request, got=%#v", payload["stream_options"])
	}
	if v, ok := payload["unknown"].(float64); !ok || int64(v) != 123 {
		t.Fatalf("expected unknown to be preserved, got=%#v", payload["unknown"])
	}
	options, ok := payload["web_search_options"].(map[string]any)
	if !ok {
		t.Fatalf("expected web_search_options map, got=%#v", payload["web_search_options"])
	}
	if got := stringFromAny(options["search_context_size"]); got != "medium" {
		t.Fatalf("expected search_context_size=medium, got=%q", got)
	}
}

func TestSanitizeChatCompletionsPayload_RejectsInvalidSearchContextSize(t *testing.T) {
	_, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"web_search_options":{"search_context_size":"huge"}}`), 0)
	if err == nil || err.Error() != "search_context_size 非法" {
		t.Fatalf("expected search_context_size error, got=%v", err)
	}
}

func TestSanitizeChatCompletionsPayload_RejectsNonArrayMessages(t *testing.T) {
	_, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":"not-an-array"}`), 0)
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got=%v", err)
	}
}

func TestSanitizeChatCompletionsPayload_RejectsInvalidMessageElements(t *testing.T) {
	_, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":[123]}`), 0)
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got=%v", err)
	}
}

func TestSanitizeChatCompletionsPayload_RejectsInvalidTokenType(t *testing.T) {
	_, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_output_tokens":"bad"}`), 0)
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got=%v", err)
	}
}

func TestSanitizeChatCompletionsPayload_RejectsNonObjectWebSearchOptions(t *testing.T) {
	_, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"web_search_options":"bad"}`), 0)
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got=%v", err)
	}
}

func TestSanitizeChatCompletionsPayload_RejectsNonObjectStreamOptions(t *testing.T) {
	_, err := sanitizeChatCompletionsPayload([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":"bad"}`), 0)
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got=%v", err)
	}
}
