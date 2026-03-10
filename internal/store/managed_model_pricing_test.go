package store

import (
	"testing"

	"github.com/shopspring/decimal"
)

func testDecimal(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}
	return d
}

func testDecimalPtr(v string) *decimal.Decimal {
	d := testDecimal(v)
	return &d
}

func TestResolveManagedModelPricingPriority(t *testing.T) {
	m := ManagedModel{
		InputUSDPer1M:              testDecimal("1"),
		OutputUSDPer1M:             testDecimal("2"),
		CacheInputUSDPer1M:         testDecimal("0.5"),
		CacheOutputUSDPer1M:        testDecimal("0.25"),
		PriorityPricingEnabled:     true,
		PriorityInputUSDPer1M:      testDecimalPtr("10"),
		PriorityOutputUSDPer1M:     testDecimalPtr("20"),
		PriorityCacheInputUSDPer1M: testDecimalPtr("5"),
	}

	pricing, err := ResolveManagedModelPricing(m, "fast", nil)
	if err != nil {
		t.Fatalf("ResolveManagedModelPricing: %v", err)
	}
	if pricing.ServiceTier != "priority" {
		t.Fatalf("service_tier=%q, want priority", pricing.ServiceTier)
	}
	if !pricing.InputUSDPer1M.Equal(testDecimal("10")) {
		t.Fatalf("input=%s, want 10", pricing.InputUSDPer1M)
	}
	if !pricing.OutputUSDPer1M.Equal(testDecimal("20")) {
		t.Fatalf("output=%s, want 20", pricing.OutputUSDPer1M)
	}
	if !pricing.CacheInputUSDPer1M.Equal(testDecimal("5")) {
		t.Fatalf("cache_input=%s, want 5", pricing.CacheInputUSDPer1M)
	}
	if !pricing.CacheOutputUSDPer1M.Equal(testDecimal("0.25")) {
		t.Fatalf("cache_output=%s, want 0.25", pricing.CacheOutputUSDPer1M)
	}
}

func TestResolveManagedModelPricingPriorityUnsupported(t *testing.T) {
	_, err := ResolveManagedModelPricing(ManagedModel{}, "priority", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrManagedModelServiceTierUnsupported {
		t.Fatalf("err=%v, want %v", err, ErrManagedModelServiceTierUnsupported)
	}
}

func TestResolveManagedModelPricingPriorityMissingRequiredPrices(t *testing.T) {
	m := ManagedModel{PriorityPricingEnabled: true}
	_, err := ResolveManagedModelPricing(m, "priority", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrManagedModelPriorityPricingMissing {
		t.Fatalf("err=%v, want %v", err, ErrManagedModelPriorityPricingMissing)
	}
}

func TestResolveManagedModelPricingHighContextAppliedAboveThreshold(t *testing.T) {
	inputTokens := int64(272001)
	m := ManagedModel{
		InputUSDPer1M:       testDecimal("1"),
		OutputUSDPer1M:      testDecimal("2"),
		CacheInputUSDPer1M:  testDecimal("0.5"),
		CacheOutputUSDPer1M: testDecimal("0.25"),
		HighContextPricing: &ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    ManagedModelHighContextServiceTierPolicyInherit,
			InputUSDPer1M:        testDecimal("5"),
			OutputUSDPer1M:       testDecimal("22.5"),
			CacheInputUSDPer1M:   testDecimalPtr("0.5"),
			CacheOutputUSDPer1M:  testDecimalPtr("0.4"),
		},
	}

	pricing, err := ResolveManagedModelPricing(m, "", &inputTokens)
	if err != nil {
		t.Fatalf("ResolveManagedModelPricing: %v", err)
	}
	if !pricing.HighContextApplied {
		t.Fatal("expected high_context_applied=true")
	}
	if pricing.PricingKind != "high_context" {
		t.Fatalf("pricing_kind=%q, want high_context", pricing.PricingKind)
	}
	if !pricing.InputUSDPer1M.Equal(testDecimal("5")) {
		t.Fatalf("input=%s, want 5", pricing.InputUSDPer1M)
	}
	if !pricing.OutputUSDPer1M.Equal(testDecimal("22.5")) {
		t.Fatalf("output=%s, want 22.5", pricing.OutputUSDPer1M)
	}
	if !pricing.CacheOutputUSDPer1M.Equal(testDecimal("0.4")) {
		t.Fatalf("cache_output=%s, want 0.4", pricing.CacheOutputUSDPer1M)
	}
	if pricing.HighContextThresholdTokens != 272000 {
		t.Fatalf("threshold=%d, want 272000", pricing.HighContextThresholdTokens)
	}
	if pricing.HighContextTriggerInputTokens != inputTokens {
		t.Fatalf("trigger_input=%d, want %d", pricing.HighContextTriggerInputTokens, inputTokens)
	}
}

func TestResolveManagedModelPricingHighContextNotAppliedAtThreshold(t *testing.T) {
	inputTokens := int64(272000)
	m := ManagedModel{
		InputUSDPer1M:       testDecimal("1"),
		OutputUSDPer1M:      testDecimal("2"),
		CacheInputUSDPer1M:  testDecimal("0.5"),
		CacheOutputUSDPer1M: testDecimal("0.25"),
		HighContextPricing: &ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    ManagedModelHighContextServiceTierPolicyInherit,
			InputUSDPer1M:        testDecimal("5"),
			OutputUSDPer1M:       testDecimal("22.5"),
		},
	}

	pricing, err := ResolveManagedModelPricing(m, "", &inputTokens)
	if err != nil {
		t.Fatalf("ResolveManagedModelPricing: %v", err)
	}
	if pricing.HighContextApplied {
		t.Fatal("expected high_context_applied=false")
	}
	if pricing.PricingKind != "base" {
		t.Fatalf("pricing_kind=%q, want base", pricing.PricingKind)
	}
	if !pricing.InputUSDPer1M.Equal(testDecimal("1")) {
		t.Fatalf("input=%s, want 1", pricing.InputUSDPer1M)
	}
}

func TestResolveManagedModelPricingHighContextForceStandardOverridesPriorityTier(t *testing.T) {
	inputTokens := int64(300000)
	m := ManagedModel{
		InputUSDPer1M:              testDecimal("1"),
		OutputUSDPer1M:             testDecimal("2"),
		CacheInputUSDPer1M:         testDecimal("0.25"),
		CacheOutputUSDPer1M:        testDecimal("0.1"),
		PriorityPricingEnabled:     true,
		PriorityInputUSDPer1M:      testDecimalPtr("10"),
		PriorityOutputUSDPer1M:     testDecimalPtr("20"),
		PriorityCacheInputUSDPer1M: testDecimalPtr("5"),
		HighContextPricing: &ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    ManagedModelHighContextServiceTierPolicyForceStandard,
			InputUSDPer1M:        testDecimal("5"),
			OutputUSDPer1M:       testDecimal("22.5"),
			CacheInputUSDPer1M:   testDecimalPtr("0.5"),
		},
	}

	pricing, err := ResolveManagedModelPricing(m, "fast", &inputTokens)
	if err != nil {
		t.Fatalf("ResolveManagedModelPricing: %v", err)
	}
	if pricing.ServiceTier != "priority" {
		t.Fatalf("service_tier=%q, want priority", pricing.ServiceTier)
	}
	if pricing.EffectiveServiceTier != "default" {
		t.Fatalf("effective_service_tier=%q, want default", pricing.EffectiveServiceTier)
	}
	if pricing.PricingKind != "high_context" {
		t.Fatalf("pricing_kind=%q, want high_context", pricing.PricingKind)
	}
	if !pricing.InputUSDPer1M.Equal(testDecimal("5")) {
		t.Fatalf("input=%s, want 5", pricing.InputUSDPer1M)
	}
	if !pricing.CacheInputUSDPer1M.Equal(testDecimal("0.5")) {
		t.Fatalf("cache_input=%s, want 0.5", pricing.CacheInputUSDPer1M)
	}
	if !pricing.CacheOutputUSDPer1M.Equal(testDecimal("0.1")) {
		t.Fatalf("cache_output=%s, want 0.1", pricing.CacheOutputUSDPer1M)
	}
}
