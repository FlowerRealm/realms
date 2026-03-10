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
)

type ManagedModelPricing struct {
	ServiceTier         string
	PricingKind         string
	EffectiveServiceTier string
	HighContextApplied  bool
	HighContextThresholdTokens int64
	HighContextTriggerInputTokens int64
	InputUSDPer1M       decimal.Decimal
	OutputUSDPer1M      decimal.Decimal
	CacheInputUSDPer1M  decimal.Decimal
	CacheOutputUSDPer1M decimal.Decimal
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

func ResolveManagedModelPricing(m ManagedModel, serviceTier string, inputTokensTotal *int64) (ManagedModelPricing, error) {
	requestedTier := NormalizeServiceTier(serviceTier)
	pricing := ManagedModelPricing{
		ServiceTier:          requestedTier,
		PricingKind:          "base",
		EffectiveServiceTier: requestedTier,
		InputUSDPer1M:        m.InputUSDPer1M.Truncate(USDScale),
		OutputUSDPer1M:       m.OutputUSDPer1M.Truncate(USDScale),
		CacheInputUSDPer1M:   m.CacheInputUSDPer1M.Truncate(USDScale),
		CacheOutputUSDPer1M:  m.CacheOutputUSDPer1M.Truncate(USDScale),
	}
	switch requestedTier {
	case "", "default", "auto", "flex":
		pricing.EffectiveServiceTier = requestedTier
	case "priority":
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

	triggered := false
	var totalInput int64
	if inputTokensTotal != nil && *inputTokensTotal > 0 {
		totalInput = *inputTokensTotal
	}
	if m.HighContextPricing != nil {
		pricing.HighContextThresholdTokens = m.HighContextPricing.ThresholdInputTokens
		pricing.HighContextTriggerInputTokens = totalInput
		if totalInput > m.HighContextPricing.ThresholdInputTokens {
			triggered = true
		}
	}
	if !triggered {
		return pricing, nil
	}

	hc := m.HighContextPricing
	if hc == nil {
		return pricing, nil
	}
	pricing.HighContextApplied = true
	pricing.PricingKind = "high_context"
	if hc.ServiceTierPolicy == ManagedModelHighContextServiceTierPolicyForceStandard {
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
