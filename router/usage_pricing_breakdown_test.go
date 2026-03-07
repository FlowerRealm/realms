package router

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func usageTestInt64(v int64) *int64 {
	return &v
}

func usageTestString(v string) *string {
	return &v
}

func usageTestDecimal(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}
	return d
}

func usageTestDecimalPtr(v string) *decimal.Decimal {
	d := usageTestDecimal(v)
	return &d
}

func TestBuildUsageEventPricingBreakdownPriorityUsesFastPricing(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()

	_, err := st.CreateManagedModel(context.Background(), store.ManagedModelCreate{
		PublicID:                   "gpt-fast",
		GroupName:                  "default",
		InputUSDPer1M:              usageTestDecimal("1"),
		OutputUSDPer1M:             usageTestDecimal("2"),
		CacheInputUSDPer1M:         usageTestDecimal("0.5"),
		CacheOutputUSDPer1M:        usageTestDecimal("0.25"),
		PriorityPricingEnabled:     true,
		PriorityInputUSDPer1M:      usageTestDecimalPtr("10"),
		PriorityOutputUSDPer1M:     usageTestDecimalPtr("20"),
		PriorityCacheInputUSDPer1M: usageTestDecimalPtr("5"),
		Status:                     1,
	})
	if err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	ev := store.UsageEvent{
		State:                  store.UsageStateCommitted,
		Model:                  usageTestString("gpt-fast"),
		ServiceTier:            usageTestString("fast"),
		InputTokens:            usageTestInt64(1000),
		CachedInputTokens:      usageTestInt64(200),
		OutputTokens:           usageTestInt64(500),
		CachedOutputTokens:     usageTestInt64(100),
		PriceMultiplier:        usageTestDecimal("1"),
		PriceMultiplierGroup:   usageTestDecimal("1"),
		PriceMultiplierPayment: usageTestDecimal("1"),
	}

	got, err := buildUsageEventPricingBreakdown(context.Background(), st, ev)
	if err != nil {
		t.Fatalf("buildUsageEventPricingBreakdown: %v", err)
	}
	if got.ServiceTier != "priority" {
		t.Fatalf("service_tier=%q, want priority", got.ServiceTier)
	}
	if !got.InputUSDPer1M.Equal(usageTestDecimal("10")) {
		t.Fatalf("input_usd_per_1m=%s, want 10", got.InputUSDPer1M)
	}
	if !got.OutputUSDPer1M.Equal(usageTestDecimal("20")) {
		t.Fatalf("output_usd_per_1m=%s, want 20", got.OutputUSDPer1M)
	}
	if !got.CacheInputUSDPer1M.Equal(usageTestDecimal("5")) {
		t.Fatalf("cache_input_usd_per_1m=%s, want 5", got.CacheInputUSDPer1M)
	}
	if !got.CacheOutputUSDPer1M.Equal(usageTestDecimal("0.25")) {
		t.Fatalf("cache_output_usd_per_1m=%s, want 0.25", got.CacheOutputUSDPer1M)
	}
}
