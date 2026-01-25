package openai

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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

func normalizeMaxOutputTokensInBody(body []byte) ([]byte, error) {
	out := body

	if v := gjson.GetBytes(out, "max_tokens"); v.Exists() {
		var err error
		out, err = sjson.SetBytes(out, "max_output_tokens", v.Value())
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_tokens")
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_completion_tokens")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	if v := gjson.GetBytes(out, "max_completion_tokens"); v.Exists() {
		var err error
		out, err = sjson.SetBytes(out, "max_output_tokens", v.Value())
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_completion_tokens")
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_tokens")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	if gjson.GetBytes(out, "max_output_tokens").Exists() {
		var err error
		out, err = sjson.DeleteBytes(out, "max_tokens")
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_completion_tokens")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	return out, nil
}

func normalizeMaxTokensInBody(body []byte) ([]byte, error) {
	out := body

	if v := gjson.GetBytes(out, "max_output_tokens"); v.Exists() {
		var err error
		out, err = sjson.SetBytes(out, "max_tokens", v.Value())
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_output_tokens")
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_completion_tokens")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	if v := gjson.GetBytes(out, "max_completion_tokens"); v.Exists() {
		var err error
		out, err = sjson.SetBytes(out, "max_tokens", v.Value())
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_completion_tokens")
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_output_tokens")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	if gjson.GetBytes(out, "max_tokens").Exists() {
		var err error
		out, err = sjson.DeleteBytes(out, "max_output_tokens")
		if err != nil {
			return nil, err
		}
		out, err = sjson.DeleteBytes(out, "max_completion_tokens")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	return out, nil
}
