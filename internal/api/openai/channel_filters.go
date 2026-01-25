package openai

import (
	"encoding/json"
	"errors"
	"strings"

	"realms/internal/scheduler"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func parseStringArrayJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(parsed))
	for _, v := range parsed {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, errors.New("数组元素不能为空")
		}
		out = append(out, v)
	}
	return out, nil
}

func applyChannelBodyFilters(body []byte, sel scheduler.Selection) ([]byte, error) {
	out := body

	whitelist, err := parseStringArrayJSON(sel.RequestBodyWhitelist)
	if err != nil {
		return nil, err
	}
	if len(whitelist) > 0 {
		keep := []byte("{}")
		for _, path := range whitelist {
			v := gjson.GetBytes(out, path)
			if !v.Exists() {
				continue
			}
			keep, err = sjson.SetBytes(keep, path, v.Value())
			if err != nil {
				return nil, err
			}
		}
		out = keep
	}

	blacklist, err := parseStringArrayJSON(sel.RequestBodyBlacklist)
	if err != nil {
		return nil, err
	}
	for _, path := range blacklist {
		out, err = sjson.DeleteBytes(out, path)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

func parseReasoningEffortFromModelSuffix(model string) (string, string) {
	effortSuffixes := []string{"-high", "-minimal", "-low", "-medium", "-none", "-xhigh"}
	for _, suffix := range effortSuffixes {
		if strings.HasSuffix(model, suffix) {
			effort := strings.TrimPrefix(suffix, "-")
			originModel := strings.TrimSuffix(model, suffix)
			return effort, originModel
		}
	}
	return "", model
}

func modelSuffixPreserved(sel scheduler.Selection, originalModel string, upstreamModel string) bool {
	raw := strings.TrimSpace(sel.ModelSuffixPreserve)
	if raw == "" || raw == "[]" {
		return false
	}
	parsed, err := parseStringArrayJSON(raw)
	if err != nil {
		return false
	}
	for _, m := range parsed {
		if m == originalModel || m == upstreamModel {
			return true
		}
	}
	return false
}

func applyResponsesModelSuffixTransforms(body []byte, sel scheduler.Selection, originalModel string) ([]byte, error) {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return body, nil
	}
	if modelSuffixPreserved(sel, originalModel, model) {
		return body, nil
	}

	effort, originModel := parseReasoningEffortFromModelSuffix(model)
	if effort == "" {
		return body, nil
	}

	out, err := sjson.SetBytes(body, "model", originModel)
	if err != nil {
		return nil, err
	}
	out, err = sjson.SetBytes(out, "reasoning.effort", effort)
	if err == nil {
		return out, nil
	}
	return sjson.SetBytes(out, "reasoning", map[string]any{"effort": effort})
}
