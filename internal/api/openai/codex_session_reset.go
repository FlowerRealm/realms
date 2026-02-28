package openai

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

type codexResetResult struct {
	body        []byte
	payload     map[string]any
	changed     bool
	hasUserText bool
}

func codexHasStatefulContinuationSignalsFromBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if strings.TrimSpace(gjson.GetBytes(body, "compaction.encrypted_content").String()) != "" {
		return true
	}
	if strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "" {
		return true
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return false
	}
	if input.IsArray() {
		for _, item := range input.Array() {
			t := strings.TrimSpace(item.Get("type").String())
			if t == "item_reference" {
				return true
			}
			if strings.HasSuffix(t, "_call_output") {
				return true
			}
		}
	}
	return false
}

func resetCodexStatefulContinuation(body []byte) (codexResetResult, error) {
	if len(body) == 0 {
		return codexResetResult{body: body}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return codexResetResult{}, err
	}

	changed := false
	if _, ok := payload["previous_response_id"]; ok {
		delete(payload, "previous_response_id")
		changed = true
	}
	if _, ok := payload["compaction"]; ok {
		delete(payload, "compaction")
		changed = true
	}

	if inputAny, ok := payload["input"]; ok {
		switch input := inputAny.(type) {
		case []any:
			filtered := make([]any, 0, len(input))
			for _, item := range input {
				m, ok := item.(map[string]any)
				if !ok {
					filtered = append(filtered, item)
					continue
				}
				typ := strings.TrimSpace(stringFromAny(m["type"]))
				if typ == "item_reference" {
					changed = true
					continue
				}
				filtered = append(filtered, item)
			}
			payload["input"] = filtered
		default:
		}
	}

	hasUserText := codexPayloadHasUserText(payload)

	if !changed {
		return codexResetResult{
			body:        body,
			payload:     payload,
			changed:     false,
			hasUserText: hasUserText,
		}, nil
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return codexResetResult{}, err
	}
	return codexResetResult{
		body:        out,
		payload:     payload,
		changed:     true,
		hasUserText: hasUserText,
	}, nil
}

func codexPayloadHasUserText(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	inputAny, ok := payload["input"]
	if !ok || inputAny == nil {
		return false
	}
	if s, ok := inputAny.(string); ok {
		return strings.TrimSpace(s) != ""
	}
	items, ok := inputAny.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(stringFromAny(m["type"])))
		switch typ {
		case "message":
			role := strings.ToLower(strings.TrimSpace(stringFromAny(m["role"])))
			if role != "" && role != "user" {
				continue
			}
			if strings.TrimSpace(extractTextContent(m["content"])) != "" {
				return true
			}
		case "input_text", "text":
			if strings.TrimSpace(stringFromAny(m["text"])) != "" {
				return true
			}
		default:
		}
	}
	return false
}

