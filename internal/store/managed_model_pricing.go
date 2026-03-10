package store

import (
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	ErrManagedModelServiceTierUnsupported = errors.New("模型不支持 fast mode")
	ErrManagedModelPriorityPricingMissing = errors.New("模型 fast mode 定价缺失")
	ErrPriorityServiceTierUnsupported     = ErrManagedModelServiceTierUnsupported
	ErrPriorityPricingMissing             = ErrManagedModelPriorityPricingMissing
	defaultFastModePriceMultiplier        = decimal.RequireFromString("2")
)

type ManagedModelPricing struct {
	ServiceTier                   string
	PricingKind                   string
	EffectiveServiceTier          string
	HighContextApplied            bool
	HighContextThresholdTokens    int64
	HighContextTriggerInputTokens int64
	InputUSDPer1M                 decimal.Decimal
	OutputUSDPer1M                decimal.Decimal
	CacheInputUSDPer1M            decimal.Decimal
	CacheOutputUSDPer1M           decimal.Decimal
}

func NormalizeServiceTier(raw string) string {
	tier := strings.ToLower(strings.TrimSpace(raw))
	if tier == "fast" {
		return "priority"
	}
	return tier
}

func NormalizeOptionalServiceTier(raw *string) *string {
	if raw == nil {
		return nil
	}
	tier := NormalizeServiceTier(*raw)
	if tier == "" {
		return nil
	}
	return &tier
}

func IsPriorityServiceTier(raw string) bool {
	return NormalizeServiceTier(raw) == "priority"
}

func applyDefaultManagedModelPriorityPricing(m *ManagedModel) error {
	if m == nil || !m.PriorityPricingEnabled {
		return nil
	}
	if m.PriorityInputUSDPer1M == nil {
		v := m.InputUSDPer1M.Truncate(USDScale).Mul(defaultFastModePriceMultiplier).Truncate(USDScale)
		m.PriorityInputUSDPer1M = &v
	}
	if m.PriorityOutputUSDPer1M == nil {
		v := m.OutputUSDPer1M.Truncate(USDScale).Mul(defaultFastModePriceMultiplier).Truncate(USDScale)
		m.PriorityOutputUSDPer1M = &v
	}
	if m.PriorityCacheInputUSDPer1M == nil {
		v := m.CacheInputUSDPer1M.Truncate(USDScale).Mul(defaultFastModePriceMultiplier).Truncate(USDScale)
		m.PriorityCacheInputUSDPer1M = &v
	}
	return nil
}

func ResolveManagedModelPricing(m ManagedModel, serviceTier string, inputTokensTotal *int64) (ManagedModelPricing, error) {
	if err := applyDefaultManagedModelPriorityPricing(&m); err != nil {
		return ManagedModelPricing{}, err
	}
	requestedTier := NormalizeServiceTier(serviceTier)
	pricing := ManagedModelPricing{
		ServiceTier:                requestedTier,
		PricingKind:                "base",
		EffectiveServiceTier:       requestedTier,
		InputUSDPer1M:              m.InputUSDPer1M.Truncate(USDScale),
		OutputUSDPer1M:             m.OutputUSDPer1M.Truncate(USDScale),
		CacheInputUSDPer1M:         m.CacheInputUSDPer1M.Truncate(USDScale),
		CacheOutputUSDPer1M:        m.CacheOutputUSDPer1M.Truncate(USDScale),
		HighContextThresholdTokens: 0,
	}

	triggered := false
	var totalInput int64
	hc := m.HighContextPricing
	if inputTokensTotal != nil && *inputTokensTotal > 0 {
		totalInput = *inputTokensTotal
	}
	if hc != nil {
		pricing.HighContextThresholdTokens = hc.ThresholdInputTokens
		pricing.HighContextTriggerInputTokens = totalInput
		if totalInput > hc.ThresholdInputTokens {
			triggered = true
		}
	}
	forceStandardTriggered := triggered && hc != nil && hc.ServiceTierPolicy == ManagedModelHighContextServiceTierPolicyForceStandard

	switch requestedTier {
	case "", "default", "auto", "flex":
		pricing.EffectiveServiceTier = requestedTier
	case "priority":
		if forceStandardTriggered {
			pricing.EffectiveServiceTier = "default"
			break
		}
		if !m.PriorityPricingEnabled {
			return ManagedModelPricing{}, ErrManagedModelServiceTierUnsupported
		}
		if m.PriorityInputUSDPer1M == nil || m.PriorityOutputUSDPer1M == nil {
			return ManagedModelPricing{}, ErrManagedModelPriorityPricingMissing
		}
		pricing.PricingKind = "priority"
		pricing.InputUSDPer1M = m.PriorityInputUSDPer1M.Truncate(USDScale)
		pricing.OutputUSDPer1M = m.PriorityOutputUSDPer1M.Truncate(USDScale)
		if m.PriorityCacheInputUSDPer1M != nil {
			pricing.CacheInputUSDPer1M = m.PriorityCacheInputUSDPer1M.Truncate(USDScale)
		}
	default:
		pricing.EffectiveServiceTier = requestedTier
	}
	if !triggered {
		return pricing, nil
	}

	if hc == nil {
		return pricing, nil
	}
	pricing.HighContextApplied = true
	pricing.PricingKind = "high_context"
	if forceStandardTriggered {
		pricing.EffectiveServiceTier = "default"
		pricing.CacheInputUSDPer1M = m.CacheInputUSDPer1M.Truncate(USDScale)
		pricing.CacheOutputUSDPer1M = m.CacheOutputUSDPer1M.Truncate(USDScale)
	}
	pricing.InputUSDPer1M = hc.InputUSDPer1M.Truncate(USDScale)
	pricing.OutputUSDPer1M = hc.OutputUSDPer1M.Truncate(USDScale)
	if hc.CacheInputUSDPer1M != nil {
		pricing.CacheInputUSDPer1M = hc.CacheInputUSDPer1M.Truncate(USDScale)
	}
	if hc.CacheOutputUSDPer1M != nil {
		pricing.CacheOutputUSDPer1M = hc.CacheOutputUSDPer1M.Truncate(USDScale)
	}
	return pricing, nil
}
