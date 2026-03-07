package openai

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
)

var errSelectedChannelFastModeUnsupported = errors.New("所选渠道不支持 fast mode")

func parseServiceTier(raw string) (*string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return nil, false
	case "fast", "priority":
		tier := "priority"
		return &tier, false
	case "auto", "default", "flex":
		tier := strings.ToLower(strings.TrimSpace(raw))
		return &tier, false
	default:
		return nil, true
	}
}

func normalizeServiceTierInPayload(payload map[string]any) (*string, bool, error) {
	if payload == nil {
		return nil, false, nil
	}
	v, ok := payload["service_tier"]
	if !ok {
		return nil, false, nil
	}
	tierRaw, ok := v.(string)
	if !ok {
		return nil, true, errors.New("service_tier invalid")
	}
	tier, invalid := parseServiceTier(tierRaw)
	if invalid {
		return nil, true, errors.New("service_tier invalid")
	}
	if tier == nil {
		delete(payload, "service_tier")
		return nil, true, nil
	}
	payload["service_tier"] = *tier
	return tier, true, nil
}

func requestedServiceTierFromPayload(payload map[string]any) *string {
	tier, _, err := normalizeServiceTierInPayload(payload)
	if err != nil {
		return nil
	}
	return tier
}

func requestedServiceTierFromJSONBytes(body []byte) *string {
	tier, _, err := requestedServiceTierFromJSONBytesStrict(body)
	if err != nil {
		return nil
	}
	return tier
}

func requestedServiceTierFromJSONBytesStrict(body []byte) (*string, bool, error) {
	if len(body) == 0 {
		return nil, false, nil
	}
	res := gjson.GetBytes(body, "service_tier")
	if !res.Exists() {
		return nil, false, nil
	}
	if res.Type != gjson.String {
		return nil, true, errors.New("service_tier invalid")
	}
	tier, invalid := parseServiceTier(res.String())
	if invalid {
		return nil, true, errors.New("service_tier invalid")
	}
	return tier, true, nil
}

func normalizeRequestServiceTier(rawBody []byte, payload map[string]any) ([]byte, *string, error) {
	serviceTier, payloadPresent, err := normalizeServiceTierInPayload(payload)
	if err != nil {
		return nil, nil, err
	}
	if len(rawBody) == 0 {
		return rawBody, serviceTier, nil
	}
	fromBody, bodyPresent, err := requestedServiceTierFromJSONBytesStrict(rawBody)
	if err != nil {
		return nil, nil, err
	}
	if serviceTier == nil {
		serviceTier = fromBody
	}
	if serviceTier == nil {
		if payloadPresent || bodyPresent {
			out, err := sjson.DeleteBytes(rawBody, "service_tier")
			if err != nil {
				return nil, nil, err
			}
			rawBody = out
		}
		return rawBody, nil, nil
	}
	if cur := strings.TrimSpace(gjson.GetBytes(rawBody, "service_tier").String()); cur != *serviceTier {
		out, err := sjson.SetBytes(rawBody, "service_tier", *serviceTier)
		if err != nil {
			return nil, nil, err
		}
		rawBody = out
	}
	return rawBody, serviceTier, nil
}

func validateManagedModelServiceTier(mm store.ManagedModel, serviceTier *string) error {
	if serviceTier == nil {
		return nil
	}
	_, err := store.ResolveManagedModelPricing(mm, *serviceTier)
	return err
}

func serviceTierBadRequestMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "模型不存在"
	}
	if errors.Is(err, store.ErrManagedModelServiceTierUnsupported) || errors.Is(err, store.ErrManagedModelPriorityPricingMissing) {
		return err.Error()
	}
	return "service_tier 不支持"
}

func applyServiceTierConstraints(cons *scheduler.Constraints, serviceTier *string) {
	if cons == nil || serviceTier == nil {
		return
	}
	if store.IsPriorityServiceTier(*serviceTier) {
		cons.RequireFastMode = true
	}
}

func serviceTierSelectionBadRequestMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, scheduler.ErrFastModeUnsupported) || errors.Is(err, errSelectedChannelFastModeUnsupported) {
		return err.Error()
	}
	return ""
}

func reserveBadRequestMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, quota.ErrServiceTierUnsupported) || errors.Is(err, quota.ErrPriorityPricingMissing) || errors.Is(err, quota.ErrModelPricingMissing) {
		return err.Error()
	}
	return ""
}

func isPriorityServiceTier(serviceTier *string) bool {
	if serviceTier == nil {
		return false
	}
	return store.IsPriorityServiceTier(*serviceTier)
}
