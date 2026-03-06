package openai

import (
	"encoding/json"
	"errors"
	"strings"
)

var errInvalidJSON = errors.New("invalid json")

func unmarshalRequestPayload(body []byte) (map[string]any, error) {
	out := make(map[string]any)
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, errInvalidJSON
	}
	return out, nil
}

func normalizeIntFieldValue(payload map[string]any, key string) *int64 {
	if payload == nil {
		return nil
	}
	value, ok := payload[key]
	if !ok {
		return nil
	}
	n := intFromAny(value)
	if n == nil {
		return nil
	}
	payload[key] = *n
	return n
}

func deletePayloadKeys(payload map[string]any, keys ...string) {
	if payload == nil {
		return
	}
	for _, key := range keys {
		delete(payload, key)
	}
}

func hasChatMessages(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	raw, ok := payload["messages"]
	if !ok || raw == nil {
		return false
	}
	items, ok := raw.([]any)
	if !ok {
		return true
	}
	return len(items) > 0
}

func hasChatPromptBoundary(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	for _, key := range []string{"prefix", "suffix"} {
		if value, ok := payload[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func normalizeWebSearchOptions(payload map[string]any) error {
	if payload == nil {
		return nil
	}
	raw, ok := payload["web_search_options"]
	if !ok || raw == nil {
		return nil
	}
	options, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	size := strings.TrimSpace(stringFromAny(options["search_context_size"]))
	switch size {
	case "":
		options["search_context_size"] = "medium"
	case "high", "medium", "low":
	default:
		return errors.New("search_context_size 非法")
	}
	payload["web_search_options"] = options
	return nil
}

// sanitizeResponsesPayload 用于解析 /v1/responses 请求体，并做最小的结构校验。
//
// 注意：这里不做字段白名单过滤，也不做 tokens 字段别名/补齐，避免“中转改写导致上游校验失败”。
func sanitizeResponsesPayload(body []byte) (map[string]any, error) {
	out, err := unmarshalRequestPayload(body)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(stringFromAny(out["model"])) == "" {
		return nil, errors.New("model 不能为空")
	}
	if _, ok := out["input"]; !ok || out["input"] == nil {
		return nil, errors.New("input 不能为空")
	}
	return out, nil
}

// sanitizeMessagesPayload 仅做最小结构校验与必要字段改写；未知字段默认透传。
func sanitizeMessagesPayload(body []byte, defaultMaxTokens int, allowMCPServers bool) (map[string]any, error) {
	out, err := unmarshalRequestPayload(body)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(stringFromAny(out["model"])) == "" {
		return nil, errors.New("model 不能为空")
	}
	if _, ok := out["messages"]; !ok || out["messages"] == nil {
		return nil, errors.New("messages 不能为空")
	}

	switch {
	case normalizeIntFieldValue(out, "max_tokens") != nil:
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	case normalizeIntFieldValue(out, "max_output_tokens") != nil:
		out["max_tokens"] = out["max_output_tokens"]
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	case normalizeIntFieldValue(out, "max_completion_tokens") != nil:
		out["max_tokens"] = out["max_completion_tokens"]
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	case normalizeIntFieldValue(out, "max_tokens_to_sample") != nil:
		out["max_tokens"] = out["max_tokens_to_sample"]
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	}

	if normalizeIntFieldValue(out, "max_tokens") == nil && defaultMaxTokens > 0 {
		out["max_tokens"] = int64(defaultMaxTokens)
	}
	maxTokens := normalizeIntFieldValue(out, "max_tokens")
	if maxTokens == nil || *maxTokens <= 0 {
		return nil, errors.New("max_tokens 不能为空")
	}

	if !allowMCPServers {
		delete(out, "mcp_servers")
	}

	return out, nil
}

// sanitizeChatCompletionsPayload 仅做最小结构校验与必要字段改写；未知字段默认透传。
func sanitizeChatCompletionsPayload(body []byte, defaultMaxTokens int) (map[string]any, error) {
	out, err := unmarshalRequestPayload(body)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(stringFromAny(out["model"])) == "" {
		return nil, errors.New("model 不能为空")
	}
	if !hasChatMessages(out) && !hasChatPromptBoundary(out) {
		return nil, errors.New("messages 不能为空")
	}
	if err := normalizeWebSearchOptions(out); err != nil {
		return nil, err
	}

	switch {
	case normalizeIntFieldValue(out, "max_completion_tokens") != nil:
		deletePayloadKeys(out, "max_tokens", "max_output_tokens")
	case normalizeIntFieldValue(out, "max_tokens") != nil:
		deletePayloadKeys(out, "max_completion_tokens", "max_output_tokens")
	case normalizeIntFieldValue(out, "max_output_tokens") != nil:
		out["max_tokens"] = out["max_output_tokens"]
		deletePayloadKeys(out, "max_completion_tokens", "max_output_tokens")
	}

	if normalizeIntFieldValue(out, "max_completion_tokens") == nil && normalizeIntFieldValue(out, "max_tokens") == nil && defaultMaxTokens > 0 {
		out["max_tokens"] = int64(defaultMaxTokens)
	}

	if !boolFromAny(out["stream"]) {
		delete(out, "stream_options")
	} else {
		options, _ := out["stream_options"].(map[string]any)
		if options == nil {
			options = make(map[string]any, 1)
		}
		options["include_usage"] = true
		out["stream_options"] = options
	}

	return out, nil
}
