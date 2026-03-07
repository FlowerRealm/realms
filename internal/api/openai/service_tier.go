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

func normalizeServiceTierInPayload(payload map[string]any) *string {
	if payload == nil {
		return nil
	}
	tier := store.NormalizeServiceTier(stringFromAny(payload["service_tier"]))
	if tier == "" {
		delete(payload, "service_tier")
		return nil
	}
	payload["service_tier"] = tier
	return &tier
}

func requestedServiceTierFromPayload(payload map[string]any) *string {
	return normalizeServiceTierInPayload(payload)
}

func requestedServiceTierFromJSONBytes(body []byte) *string {
	if len(body) == 0 {
		return nil
	}
	tier := store.NormalizeServiceTier(gjson.GetBytes(body, "service_tier").String())
	if tier == "" {
		return nil
	}
	return &tier
}

func normalizeRequestServiceTier(rawBody []byte, payload map[string]any) ([]byte, *string, error) {
	serviceTier := normalizeServiceTierInPayload(payload)
	if len(rawBody) == 0 {
		return rawBody, serviceTier, nil
	}
	fromBody := requestedServiceTierFromJSONBytes(rawBody)
	if serviceTier == nil {
		serviceTier = fromBody
	}
	if serviceTier == nil {
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
