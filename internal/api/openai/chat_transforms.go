package openai

import (
	"strings"

	"realms/internal/scheduler"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func applyChatCompletionsModelSuffixTransforms(body []byte, sel scheduler.Selection, originalModel string) ([]byte, error) {
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
	return sjson.SetBytes(out, "reasoning_effort", effort)
}

func applyChatCompletionsModelRules(body []byte) ([]byte, error) {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return body, nil
	}

	out := body
	var err error

	// o/gpt-5: max_tokens -> max_completion_tokens
	if strings.HasPrefix(model, "o") || strings.HasPrefix(model, "gpt-5") {
		if !gjson.GetBytes(out, "max_completion_tokens").Exists() {
			if v := gjson.GetBytes(out, "max_tokens"); v.Exists() {
				out, err = sjson.SetBytes(out, "max_completion_tokens", v.Value())
				if err != nil {
					return nil, err
				}
				out, err = sjson.DeleteBytes(out, "max_tokens")
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// o: 不支持 temperature；并将首条 system 角色改为 developer（o1-mini/o1-preview 例外）。
	if strings.HasPrefix(model, "o") {
		out, err = sjson.DeleteBytes(out, "temperature")
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(model, "o1-mini") && !strings.HasPrefix(model, "o1-preview") {
			role := gjson.GetBytes(out, "messages.0.role").String()
			if role == "system" {
				out, err = sjson.SetBytes(out, "messages.0.role", "developer")
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// gpt-5: 不支持 temperature/top_p/logprobs。
	if strings.HasPrefix(model, "gpt-5") {
		for _, path := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
			out, err = sjson.DeleteBytes(out, path)
			if err != nil {
				return nil, err
			}
		}
	}

	return out, nil
}
