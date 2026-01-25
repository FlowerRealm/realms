package openai

func normalizeMaxOutputTokensInPayload(payload map[string]any) {
	if payload == nil {
		return
	}
	if _, ok := payload["max_output_tokens"]; ok {
		delete(payload, "max_tokens")
		delete(payload, "max_completion_tokens")
		return
	}
	if v, ok := payload["max_tokens"]; ok {
		payload["max_output_tokens"] = v
		delete(payload, "max_tokens")
		delete(payload, "max_completion_tokens")
		return
	}
	if v, ok := payload["max_completion_tokens"]; ok {
		payload["max_output_tokens"] = v
		delete(payload, "max_completion_tokens")
		delete(payload, "max_tokens")
		return
	}
}

func normalizeMaxTokensInPayload(payload map[string]any) {
	if payload == nil {
		return
	}
	if _, ok := payload["max_tokens"]; ok {
		delete(payload, "max_output_tokens")
		delete(payload, "max_completion_tokens")
		return
	}
	if v, ok := payload["max_output_tokens"]; ok {
		payload["max_tokens"] = v
		delete(payload, "max_output_tokens")
		delete(payload, "max_completion_tokens")
		return
	}
	if v, ok := payload["max_completion_tokens"]; ok {
		payload["max_tokens"] = v
		delete(payload, "max_completion_tokens")
		delete(payload, "max_output_tokens")
		return
	}
}
