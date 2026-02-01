package openai

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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
