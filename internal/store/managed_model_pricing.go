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

func ResolveManagedModelPricing(m ManagedModel, serviceTier string) (ManagedModelPricing, error) {
	pricing := ManagedModelPricing{
		ServiceTier:         NormalizeServiceTier(serviceTier),
		InputUSDPer1M:       m.InputUSDPer1M.Truncate(USDScale),
		OutputUSDPer1M:      m.OutputUSDPer1M.Truncate(USDScale),
		CacheInputUSDPer1M:  m.CacheInputUSDPer1M.Truncate(USDScale),
		CacheOutputUSDPer1M: m.CacheOutputUSDPer1M.Truncate(USDScale),
	}
	if pricing.ServiceTier != "priority" {
		return pricing, nil
	}
	if !m.PriorityPricingEnabled {
		return ManagedModelPricing{}, ErrManagedModelServiceTierUnsupported
	}
	if m.PriorityInputUSDPer1M == nil || m.PriorityOutputUSDPer1M == nil {
		return ManagedModelPricing{}, ErrManagedModelPriorityPricingMissing
	}
	pricing.InputUSDPer1M = m.PriorityInputUSDPer1M.Truncate(USDScale)
	pricing.OutputUSDPer1M = m.PriorityOutputUSDPer1M.Truncate(USDScale)
	if m.PriorityCacheInputUSDPer1M != nil {
		pricing.CacheInputUSDPer1M = m.PriorityCacheInputUSDPer1M.Truncate(USDScale)
	}
	return pricing, nil
}
