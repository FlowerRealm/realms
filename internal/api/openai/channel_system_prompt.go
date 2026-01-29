package openai

import (
	"strings"

	"realms/internal/scheduler"
)

func applyChannelSystemPromptToResponsesPayload(payload map[string]any, sel scheduler.Selection) {
	systemPrompt := strings.TrimSpace(sel.SystemPrompt)
	if systemPrompt == "" || payload == nil {
		return
	}

	raw, _ := payload["instructions"].(string)
	existing := strings.TrimSpace(raw)
	switch {
	case existing == "":
		payload["instructions"] = systemPrompt
	case sel.SystemPromptOverride:
		payload["instructions"] = systemPrompt + "\n" + existing
	}
}

func applyChannelSystemPromptToChatCompletionsPayload(payload map[string]any, sel scheduler.Selection) {
	systemPrompt := strings.TrimSpace(sel.SystemPrompt)
	if systemPrompt == "" || payload == nil {
		return
	}

	arr, ok := payload["messages"].([]any)
	if !ok || len(arr) == 0 {
		return
	}

	systemIdx := -1
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(stringFromAny(m["role"]))
		if role == "system" {
			systemIdx = i
			break
		}
	}

	if systemIdx == -1 {
		// 无 system 消息：插入到最前。
		msg := map[string]any{
			"role":    "system",
			"content": systemPrompt,
		}
		payload["messages"] = append([]any{msg}, arr...)
		return
	}

	if !sel.SystemPromptOverride {
		return
	}

	m, ok := arr[systemIdx].(map[string]any)
	if !ok {
		return
	}

	switch v := m["content"].(type) {
	case string:
		m["content"] = systemPrompt + "\n" + v
	case []any:
		parts := make([]any, 0, len(v)+1)
		parts = append(parts, map[string]any{
			"type": "text",
			"text": systemPrompt,
		})
		parts = append(parts, v...)
		m["content"] = parts
	default:
		m["content"] = systemPrompt
	}
	arr[systemIdx] = m
	payload["messages"] = arr
}

func applyChannelSystemPromptToMessagesPayload(payload map[string]any, sel scheduler.Selection) {
	systemPrompt := strings.TrimSpace(sel.SystemPrompt)
	if systemPrompt == "" || payload == nil {
		return
	}

	raw := payload["system"]
	switch v := raw.(type) {
	case nil:
		payload["system"] = systemPrompt
	case string:
		existing := strings.TrimSpace(v)
		switch {
		case existing == "":
			payload["system"] = systemPrompt
		case sel.SystemPromptOverride:
			payload["system"] = systemPrompt + "\n" + v
		}
	case []any:
		if !sel.SystemPromptOverride {
			return
		}
		blocks := make([]any, 0, len(v)+1)
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": systemPrompt,
		})
		blocks = append(blocks, v...)
		payload["system"] = blocks
	default:
		payload["system"] = systemPrompt
	}
}
