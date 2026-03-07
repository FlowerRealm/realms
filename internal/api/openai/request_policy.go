package openai

import (
	"github.com/tidwall/sjson"

	"realms/internal/scheduler"
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
	serviceTier, _, err := requestedServiceTierFromJSONBytesStrict(out)
	if err != nil {
		return nil, err
	}
	if !sel.AllowServiceTier {
		if serviceTier != nil && *serviceTier == "priority" {
			return nil, errSelectedChannelFastModeUnsupported
		}
		out, err = sjson.DeleteBytes(out, "service_tier")
		if err != nil {
			return nil, err
		}
	} else if serviceTier != nil {
		if *serviceTier == "priority" && !sel.FastMode {
			return nil, errSelectedChannelFastModeUnsupported
		}
		out, err = sjson.SetBytes(out, "service_tier", *serviceTier)
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
