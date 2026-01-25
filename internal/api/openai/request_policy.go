package openai

import (
	"realms/internal/scheduler"

	"github.com/tidwall/sjson"
)

func clonePayload(base map[string]any) map[string]any {
	if base == nil {
		return nil
	}
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	return out
}

func applyChannelRequestPolicy(body []byte, sel scheduler.Selection) ([]byte, error) {
	out := body
	var err error
	if !sel.AllowServiceTier {
		out, err = sjson.DeleteBytes(out, "service_tier")
		if err != nil {
			return nil, err
		}
	}
	if sel.DisableStore {
		out, err = sjson.DeleteBytes(out, "store")
		if err != nil {
			return nil, err
		}
	}
	if !sel.AllowSafetyIdentifier {
		out, err = sjson.DeleteBytes(out, "safety_identifier")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
