package upstream

import "strings"

// NeedsToolContinuation 判定请求是否需要工具调用续链处理。
// 满足以下任一信号即视为续链：previous_response_id、input 内包含 function_call_output/item_reference、
// 或显式声明 tools/tool_choice。
func NeedsToolContinuation(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	if hasNonEmptyString(reqBody["previous_response_id"]) {
		return true
	}
	if hasToolsSignal(reqBody) {
		return true
	}
	if hasToolChoiceSignal(reqBody) {
		return true
	}
	if inputHasType(reqBody, "function_call_output") {
		return true
	}
	if inputHasType(reqBody, "item_reference") {
		return true
	}
	return false
}

// inputHasType 判断 input 中是否存在指定类型的 item。
func inputHasType(reqBody map[string]any, want string) bool {
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		if itemType == want {
			return true
		}
	}
	return false
}

// hasNonEmptyString 判断字段是否为非空字符串。
func hasNonEmptyString(value any) bool {
	stringValue, ok := value.(string)
	return ok && strings.TrimSpace(stringValue) != ""
}

// hasToolsSignal 判断 tools 字段是否显式声明（存在且不为空）。
func hasToolsSignal(reqBody map[string]any) bool {
	raw, exists := reqBody["tools"]
	if !exists || raw == nil {
		return false
	}
	if tools, ok := raw.([]any); ok {
		return len(tools) > 0
	}
	return false
}

// hasToolChoiceSignal 判断 tool_choice 是否显式声明（非空或非 nil）。
func hasToolChoiceSignal(reqBody map[string]any) bool {
	raw, exists := reqBody["tool_choice"]
	if !exists || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case map[string]any:
		return len(value) > 0
	default:
		return false
	}
}

