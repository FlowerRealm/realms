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

func normalizeJSONIntFieldValue(payload map[string]any, key string) (*int64, bool, error) {
	if payload == nil {
		return nil, false, nil
	}
	value, ok := payload[key]
	if !ok {
		return nil, false, nil
	}
	if value == nil {
		delete(payload, key)
		return nil, false, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, true, errInvalidJSON
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, true, errInvalidJSON
	}
	payload[key] = n
	return &n, true, nil
}

func deletePayloadKeys(payload map[string]any, keys ...string) {
	if payload == nil {
		return
	}
	for _, key := range keys {
		delete(payload, key)
	}
}

func hasChatMessages(payload map[string]any) (bool, error) {
	if payload == nil {
		return false, nil
	}
	raw, ok := payload["messages"]
	if !ok || raw == nil {
		return false, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return false, errInvalidJSON
	}
	for _, item := range items {
		if item == nil {
			return false, errInvalidJSON
		}
		if _, ok := item.(map[string]any); !ok {
			return false, errInvalidJSON
		}
	}
	return len(items) > 0, nil
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
		return errInvalidJSON
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

func normalizeStreamOptions(payload map[string]any, stream bool) error {
	if payload == nil {
		return nil
	}
	raw, exists := payload["stream_options"]
	if !exists || raw == nil {
		if stream {
			payload["stream_options"] = map[string]any{"include_usage": true}
		}
		return nil
	}
	if !stream {
		delete(payload, "stream_options")
		return nil
	}
	options, ok := raw.(map[string]any)
	if !ok {
		return errInvalidJSON
	}
	options["include_usage"] = true
	payload["stream_options"] = options
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

	maxTokens, hasMaxTokens, err := normalizeJSONIntFieldValue(out, "max_tokens")
	if err != nil {
		return nil, err
	}
	maxOutputTokens, hasMaxOutputTokens, err := normalizeJSONIntFieldValue(out, "max_output_tokens")
	if err != nil {
		return nil, err
	}
	maxCompletionTokens, hasMaxCompletionTokens, err := normalizeJSONIntFieldValue(out, "max_completion_tokens")
	if err != nil {
		return nil, err
	}
	maxTokensToSample, hasMaxTokensToSample, err := normalizeJSONIntFieldValue(out, "max_tokens_to_sample")
	if err != nil {
		return nil, err
	}

	switch {
	case hasMaxTokens:
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	case hasMaxOutputTokens:
		out["max_tokens"] = *maxOutputTokens
		maxTokens = maxOutputTokens
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	case hasMaxCompletionTokens:
		out["max_tokens"] = *maxCompletionTokens
		maxTokens = maxCompletionTokens
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	case hasMaxTokensToSample:
		out["max_tokens"] = *maxTokensToSample
		maxTokens = maxTokensToSample
		deletePayloadKeys(out, "max_output_tokens", "max_completion_tokens", "max_tokens_to_sample")
	}

	if maxTokens == nil && defaultMaxTokens > 0 {
		out["max_tokens"] = int64(defaultMaxTokens)
		v := int64(defaultMaxTokens)
		maxTokens = &v
	}
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
	hasMessages, err := hasChatMessages(out)
	if err != nil {
		return nil, err
	}
	if !hasMessages && !hasChatPromptBoundary(out) {
		return nil, errors.New("messages 不能为空")
	}
	if err := normalizeWebSearchOptions(out); err != nil {
		return nil, err
	}

	maxCompletionTokens, hasMaxCompletionTokens, err := normalizeJSONIntFieldValue(out, "max_completion_tokens")
	if err != nil {
		return nil, err
	}
	maxTokens, hasMaxTokens, err := normalizeJSONIntFieldValue(out, "max_tokens")
	if err != nil {
		return nil, err
	}
	maxOutputTokens, hasMaxOutputTokens, err := normalizeJSONIntFieldValue(out, "max_output_tokens")
	if err != nil {
		return nil, err
	}

	switch {
	case hasMaxCompletionTokens:
		deletePayloadKeys(out, "max_tokens", "max_output_tokens")
	case hasMaxTokens:
		deletePayloadKeys(out, "max_completion_tokens", "max_output_tokens")
	case hasMaxOutputTokens:
		out["max_tokens"] = *maxOutputTokens
		maxTokens = maxOutputTokens
		deletePayloadKeys(out, "max_completion_tokens", "max_output_tokens")
	}

	if maxCompletionTokens == nil && maxTokens == nil && defaultMaxTokens > 0 {
		out["max_tokens"] = int64(defaultMaxTokens)
	}

	if err := normalizeStreamOptions(out, boolFromAny(out["stream"])); err != nil {
		return nil, err
	}

	return out, nil
}
