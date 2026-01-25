package openai

import (
	"encoding/json"
	"strings"

	"realms/internal/scheduler"
)

func buildParamOverrideContext(sel scheduler.Selection, originalModel, upstreamModel, requestPath string) map[string]any {
	ctx := make(map[string]any, 8)
	if strings.TrimSpace(upstreamModel) != "" {
		ctx["model"] = upstreamModel
		ctx["upstream_model"] = upstreamModel
	}
	if strings.TrimSpace(originalModel) != "" {
		ctx["original_model"] = originalModel
		if _, ok := ctx["model"]; !ok {
			ctx["model"] = originalModel
		}
	}
	if strings.TrimSpace(requestPath) != "" {
		ctx["request_path"] = requestPath
	}
	ctx["is_channel_test"] = false
	if strings.TrimSpace(sel.ChannelType) != "" {
		ctx["channel_type"] = sel.ChannelType
	}
	if sel.ChannelID > 0 {
		ctx["channel_id"] = sel.ChannelID
	}
	return ctx
}

func applyChannelParamOverride(body []byte, sel scheduler.Selection, conditionContext map[string]any) ([]byte, error) {
	override := strings.TrimSpace(sel.ParamOverride)
	if override == "" {
		return body, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(override), &parsed); err != nil {
		return nil, err
	}
	return ApplyParamOverride(body, parsed, conditionContext)
}
