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

	pricing, err := ResolveManagedModelPricing(m, "fast")
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
	_, err := ResolveManagedModelPricing(ManagedModel{}, "priority")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrManagedModelServiceTierUnsupported {
		t.Fatalf("err=%v, want %v", err, ErrManagedModelServiceTierUnsupported)
	}
}

func TestResolveManagedModelPricingPriorityMissingRequiredPrices(t *testing.T) {
	m := ManagedModel{PriorityPricingEnabled: true}
	_, err := ResolveManagedModelPricing(m, "priority")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrManagedModelPriorityPricingMissing {
		t.Fatalf("err=%v, want %v", err, ErrManagedModelPriorityPricingMissing)
	}
}
