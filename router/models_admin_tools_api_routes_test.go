package router

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func mustDecimal(t *testing.T, v string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(v)
	if err != nil {
		t.Fatalf("decimal parse: %v", err)
	}
	return d
}

func mustDecimalPtr(t *testing.T, v string) *decimal.Decimal {
	t.Helper()
	d := mustDecimal(t, v)
	return &d
}

func TestParsePricingImportJSON_PriorityFieldsOptional(t *testing.T) {
	parsed, err := parsePricingImportJSON([]byte(`{
		"gpt-fast": {
			"input_usd_per_1m": 1,
			"output_usd_per_1m": 2,
			"cache_input_usd_per_1m": 0.5,
			"cache_output_usd_per_1m": 0.25,
			"priority_pricing_enabled": true,
			"priority_input_usd_per_1m": 10,
			"priority_output_usd_per_1m": 20
		}
	}`))
	if err != nil {
		t.Fatalf("parsePricingImportJSON: %v", err)
	}
	if len(parsed.failed) != 0 {
		t.Fatalf("unexpected failed items: %+v", parsed.failed)
	}
	if len(parsed.items) != 1 {
		t.Fatalf("items=%d, want 1", len(parsed.items))
	}
	it := parsed.items[0]
	if it.PriorityPricingEnabled == nil || !*it.PriorityPricingEnabled {
		t.Fatalf("priority_pricing_enabled=%v, want true", it.PriorityPricingEnabled)
	}
	if it.PriorityInputUSDPer1M == nil || !it.PriorityInputUSDPer1M.Equal(mustDecimal(t, "10")) {
		t.Fatalf("priority_input=%v, want 10", it.PriorityInputUSDPer1M)
	}
	if it.PriorityOutputUSDPer1M == nil || !it.PriorityOutputUSDPer1M.Equal(mustDecimal(t, "20")) {
		t.Fatalf("priority_output=%v, want 20", it.PriorityOutputUSDPer1M)
	}
	if it.PriorityCacheInputUSDPer1M != nil {
		t.Fatalf("priority_cache_input=%v, want nil when omitted", it.PriorityCacheInputUSDPer1M)
	}
}

func TestParsePricingImportJSON_HighContextPricing(t *testing.T) {
	parsed, err := parsePricingImportJSON([]byte(`{
		"gpt-5.4": {
			"input_usd_per_1m": 2.5,
			"output_usd_per_1m": 15,
			"cache_input_usd_per_1m": 0.25,
			"cache_output_usd_per_1m": 0.25,
			"high_context_pricing": {
				"threshold_input_tokens": 272000,
				"service_tier_policy": "force_standard",
				"input_usd_per_1m": 5,
				"output_usd_per_1m": 22.5,
				"cache_input_usd_per_1m": 0.5,
				"source": "openai_official"
			}
		}
	}`))
	if err != nil {
		t.Fatalf("parsePricingImportJSON: %v", err)
	}
	if len(parsed.items) != 1 {
		t.Fatalf("items=%d, want 1", len(parsed.items))
	}
	hc := parsed.items[0].HighContextPricing
	if !parsed.items[0].HighContextPricingSpecified || hc == nil {
		t.Fatalf("high_context_pricing not parsed: %+v", parsed.items[0])
	}
	if hc.ThresholdInputTokens != 272000 {
		t.Fatalf("threshold=%d, want 272000", hc.ThresholdInputTokens)
	}
	if hc.ServiceTierPolicy != "force_standard" {
		t.Fatalf("service_tier_policy=%q", hc.ServiceTierPolicy)
	}
	if !hc.InputUSDPer1M.Equal(mustDecimal(t, "5")) {
		t.Fatalf("high_context input=%s, want 5", hc.InputUSDPer1M)
	}
}

func TestUpsertManagedModelPricing_DoesNotClearPriorityFieldsWhenOmitted(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()

	_, err := st.CreateManagedModel(context.Background(), store.ManagedModelCreate{
		PublicID:                   "gpt-fast",
		GroupName:                  "default",
		InputUSDPer1M:              mustDecimal(t, "1"),
		OutputUSDPer1M:             mustDecimal(t, "2"),
		CacheInputUSDPer1M:         mustDecimal(t, "0.5"),
		CacheOutputUSDPer1M:        mustDecimal(t, "0.25"),
		PriorityPricingEnabled:     true,
		PriorityInputUSDPer1M:      mustDecimalPtr(t, "10"),
		PriorityOutputUSDPer1M:     mustDecimalPtr(t, "20"),
		PriorityCacheInputUSDPer1M: mustDecimalPtr(t, "5"),
		Status:                     1,
	})
	if err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	_, err = st.UpsertManagedModelPricing(context.Background(), []store.ManagedModelPricingUpsert{{
		PublicID:             "gpt-fast",
		BasePricingSpecified: true,
		InputUSDPer1M:        mustDecimal(t, "1.1"),
		OutputUSDPer1M:       mustDecimal(t, "2.2"),
		CacheInputUSDPer1M:   mustDecimal(t, "0.55"),
		CacheOutputUSDPer1M:  mustDecimal(t, "0.3"),
	}})
	if err != nil {
		t.Fatalf("UpsertManagedModelPricing: %v", err)
	}

	got, err := st.GetManagedModelByPublicID(context.Background(), "gpt-fast")
	if err != nil {
		t.Fatalf("GetManagedModelByPublicID: %v", err)
	}
	if !got.PriorityPricingEnabled {
		t.Fatal("priority_pricing_enabled unexpectedly cleared")
	}
	if got.PriorityInputUSDPer1M == nil || !got.PriorityInputUSDPer1M.Equal(mustDecimal(t, "10")) {
		t.Fatalf("priority_input=%v, want 10", got.PriorityInputUSDPer1M)
	}
	if got.PriorityOutputUSDPer1M == nil || !got.PriorityOutputUSDPer1M.Equal(mustDecimal(t, "20")) {
		t.Fatalf("priority_output=%v, want 20", got.PriorityOutputUSDPer1M)
	}
	if got.PriorityCacheInputUSDPer1M == nil || !got.PriorityCacheInputUSDPer1M.Equal(mustDecimal(t, "5")) {
		t.Fatalf("priority_cache_input=%v, want 5", got.PriorityCacheInputUSDPer1M)
	}
	if !got.InputUSDPer1M.Equal(mustDecimal(t, "1.1")) {
		t.Fatalf("input=%s, want 1.1", got.InputUSDPer1M)
	}
	if !got.OutputUSDPer1M.Equal(mustDecimal(t, "2.2")) {
		t.Fatalf("output=%s, want 2.2", got.OutputUSDPer1M)
	}
}
