package modellibrary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestOpenRouterCatalog_Lookup_ConvertsPerTokenPricing(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":   "openai/gpt-5.4",
					"name": "OpenAI: GPT-5.4",
					"pricing": map[string]any{
						"prompt":           "0.0000025",
						"completion":       "0.000015",
						"input_cache_read": "0.00000025",
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(OpenRouterCatalogOptions{URL: srv.URL, TTL: time.Minute})
	res, err := c.Lookup(context.Background(), "openai/gpt-5.4")
	if err != nil {
		t.Fatalf("Lookup() err = %v", err)
	}
	if res.Source != "openrouter" {
		t.Fatalf("Source = %q, want %q", res.Source, "openrouter")
	}
	if res.OwnedBy != "openai" {
		t.Fatalf("OwnedBy = %q, want %q", res.OwnedBy, "openai")
	}
	if !res.InputUSDPer1M.Equal(decimal.RequireFromString("2.5")) {
		t.Fatalf("InputUSDPer1M = %s", res.InputUSDPer1M)
	}
	if !res.OutputUSDPer1M.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("OutputUSDPer1M = %s", res.OutputUSDPer1M)
	}
	if !res.CacheInputUSDPer1M.Equal(decimal.RequireFromString("0.25")) {
		t.Fatalf("CacheInputUSDPer1M = %s", res.CacheInputUSDPer1M)
	}
	if !res.CacheOutputUSDPer1M.Equal(decimal.RequireFromString("0.25")) {
		t.Fatalf("CacheOutputUSDPer1M = %s", res.CacheOutputUSDPer1M)
	}
}

func TestOpenRouterCatalog_Lookup_MissingPricing(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":      "openrouter/auto",
					"name":    "OpenRouter Auto",
					"pricing": map[string]any{"prompt": "-1", "completion": "-1"},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(OpenRouterCatalogOptions{URL: srv.URL, TTL: time.Minute})
	_, err := c.Lookup(context.Background(), "openrouter/auto")
	if err == nil {
		t.Fatal("Lookup() err = nil, want pricing error")
	}
	if err != ErrModelNoPricing {
		t.Fatalf("Lookup() err = %v, want %v", err, ErrModelNoPricing)
	}
}

func TestOpenRouterCatalog_Suggest_SortsByMatchQuality(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":   "openai/gpt-5.4",
					"name": "OpenAI: GPT-5.4",
					"pricing": map[string]any{
						"prompt":     "0.0000025",
						"completion": "0.000015",
					},
				},
				{
					"id":   "openai/gpt-5.4-mini",
					"name": "OpenAI: GPT-5.4 Mini",
					"pricing": map[string]any{
						"prompt":     "0.000001",
						"completion": "0.000004",
					},
				},
				{
					"id":   "anthropic/claude-4.5-sonnet",
					"name": "Claude GPT competitor",
					"pricing": map[string]any{
						"prompt":     "0.000003",
						"completion": "0.000015",
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(OpenRouterCatalogOptions{URL: srv.URL, TTL: time.Minute})
	got, err := c.Suggest(context.Background(), "gpt-5.4", 10)
	if err != nil {
		t.Fatalf("Suggest() err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(Suggest()) = %d, want 2", len(got))
	}
	if got[0].ModelID != "openai/gpt-5.4" {
		t.Fatalf("first model = %q, want exact match", got[0].ModelID)
	}
	if got[1].ModelID != "openai/gpt-5.4-mini" {
		t.Fatalf("second model = %q, want prefix match", got[1].ModelID)
	}
}

func TestOpenRouterCatalog_Suggest_IncludesModelsWithoutPricing(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":      "openrouter/auto",
					"name":    "OpenRouter Auto",
					"pricing": map[string]any{"prompt": "-1", "completion": "-1"},
				},
				{
					"id":   "openai/gpt-5.4",
					"name": "OpenAI: GPT-5.4",
					"pricing": map[string]any{
						"prompt":     "0.0000025",
						"completion": "0.000015",
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(OpenRouterCatalogOptions{URL: srv.URL, TTL: time.Minute})
	got, err := c.Suggest(context.Background(), "open", 10)
	if err != nil {
		t.Fatalf("Suggest() err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(Suggest()) = %d, want 2", len(got))
	}
	if got[0].ModelID != "openai/gpt-5.4" {
		t.Fatalf("first model = %q, want %q", got[0].ModelID, "openai/gpt-5.4")
	}
	if got[1].ModelID != "openrouter/auto" {
		t.Fatalf("second model = %q, want %q", got[1].ModelID, "openrouter/auto")
	}
}

func TestEnrichLookupResult_OpenRouterHighContextFallback(t *testing.T) {
	oldFetch := fetchURLFunc
	t.Cleanup(func() { fetchURLFunc = oldFetch })
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		if !strings.Contains(url, "openrouter.ai/openai/o3/pricing") {
			t.Fatalf("unexpected url: %s", url)
		}
		return `{"pricing_json":{"openai_responses:high_context_threshold":"272000","openai_responses:prompt_tokens_high_context":"5e-6","openai_responses:completion_tokens_high_context":"22.5e-6","openai_responses:cached_prompt_tokens_high_context":"0.5e-6"}}`, nil
	}

	res, err := enrichLookupResult(context.Background(), LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "openai/o3",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	})
	if err != nil {
		t.Fatalf("enrichLookupResult() err = %v", err)
	}
	if res.HighContextPricing == nil {
		t.Fatal("expected high_context_pricing")
	}
	if res.HighContextPricing.ThresholdInputTokens != 272000 {
		t.Fatalf("threshold=%d, want 272000", res.HighContextPricing.ThresholdInputTokens)
	}
	if !res.HighContextPricing.InputUSDPer1M.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("input=%s, want 5", res.HighContextPricing.InputUSDPer1M)
	}
	if res.SourceDetail != "models.dev" {
		t.Fatalf("source_detail=%q, want models.dev", res.SourceDetail)
	}
	if res.HighContextPricing.SourceDetail != "openrouter_pricing_page" {
		t.Fatalf("high_context source_detail=%q, want openrouter_pricing_page", res.HighContextPricing.SourceDetail)
	}
}

func TestEnrichLookupResult_OpenAIOfficialHighContextPricing(t *testing.T) {
	oldFetch := fetchURLFunc
	t.Cleanup(func() { fetchURLFunc = oldFetch })
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		switch url {
		case "https://developers.openai.com/api/docs/pricing/":
			return `<table><tr><td>gpt-5.4 (&gt;272K context length)</td></tr></table>`, nil
		case "https://developers.openai.com/api/docs/guides/latest-model/":
			return `Requests above 272K tokens is automatically processed at standard rates.`, nil
		default:
			t.Fatalf("unexpected url: %s", url)
			return "", nil
		}
	}

	res, err := enrichLookupResult(context.Background(), LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	})
	if err != nil {
		t.Fatalf("enrichLookupResult() err = %v", err)
	}
	if res.HighContextPricing == nil {
		t.Fatal("expected high_context_pricing")
	}
	if res.HighContextPricing.ServiceTierPolicy != "force_standard" {
		t.Fatalf("service_tier_policy=%q, want force_standard", res.HighContextPricing.ServiceTierPolicy)
	}
	if !res.HighContextPricing.OutputUSDPer1M.Equal(decimal.RequireFromString("22.5")) {
		t.Fatalf("output=%s, want 22.5", res.HighContextPricing.OutputUSDPer1M)
	}
	if res.SourceDetail != "models.dev" {
		t.Fatalf("source_detail=%q, want models.dev", res.SourceDetail)
	}
	if res.HighContextPricing.SourceDetail != "openai_official_pricing_docs" {
		t.Fatalf("high_context source_detail=%q, want openai_official_pricing_docs", res.HighContextPricing.SourceDetail)
	}
}

func TestEnrichLookupResult_PreservesBasePricingWhenEnrichmentUnavailable(t *testing.T) {
	oldFetch := fetchURLFunc
	t.Cleanup(func() { fetchURLFunc = oldFetch })
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		switch url {
		case "https://developers.openai.com/api/docs/pricing/":
			return `<table><tr><td>gpt-4o</td></tr></table>`, nil
		case "https://developers.openai.com/api/docs/guides/latest-model/":
			return `Requests above 272K tokens is automatically processed at standard rates.`, nil
		}
		if strings.Contains(url, "openrouter.ai/openai/gpt-5.4/pricing") {
			return "", errors.New("openrouter unavailable")
		}
		t.Fatalf("unexpected url: %s", url)
		return "", nil
	}

	res, err := enrichLookupResult(context.Background(), LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	})
	if err != nil {
		t.Fatalf("enrichLookupResult() err = %v", err)
	}
	if res.HighContextPricing != nil {
		t.Fatal("expected no high_context_pricing when enrichment is unavailable")
	}
	if res.SourceDetail != "models.dev" {
		t.Fatalf("source_detail=%q, want models.dev", res.SourceDetail)
	}
}

func TestEnrichLookupResult_HighContextCacheHit(t *testing.T) {
	oldFetch := fetchURLFunc
	oldTTL := highContextLookupCacheTTL
	t.Cleanup(func() {
		fetchURLFunc = oldFetch
		highContextLookupCacheTTL = oldTTL
	})
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	highContextLookupCacheTTL = time.Minute

	var fetchCount int32
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		atomic.AddInt32(&fetchCount, 1)
		switch url {
		case "https://developers.openai.com/api/docs/pricing/":
			return `<table><tr><td>gpt-5.4 (&gt;272K context length)</td></tr></table>`, nil
		case "https://developers.openai.com/api/docs/guides/latest-model/":
			return `Requests above 272K tokens is automatically processed at standard rates.`, nil
		default:
			t.Fatalf("unexpected url: %s", url)
			return "", nil
		}
	}

	in := LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	}

	first, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("first enrichLookupResult() err = %v", err)
	}
	second, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("second enrichLookupResult() err = %v", err)
	}
	if first.HighContextPricing == nil || second.HighContextPricing == nil {
		t.Fatal("expected cached high_context_pricing")
	}
	if got := atomic.LoadInt32(&fetchCount); got != 2 {
		t.Fatalf("fetch count = %d, want 2", got)
	}
}

func TestEnrichLookupResult_DoesNotCacheErrors(t *testing.T) {
	oldFetch := fetchURLFunc
	oldTTL := highContextLookupCacheTTL
	t.Cleanup(func() {
		fetchURLFunc = oldFetch
		highContextLookupCacheTTL = oldTTL
	})
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	highContextLookupCacheTTL = time.Minute

	var fetchCount int32
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		atomic.AddInt32(&fetchCount, 1)
		if strings.HasPrefix(url, "https://developers.openai.com/") {
			return "", errors.New("official docs unavailable")
		}
		if strings.Contains(url, "openrouter.ai/openai/gpt-5.4/pricing") {
			return "", errors.New("openrouter pricing unavailable")
		}
		t.Fatalf("unexpected url: %s", url)
		return "", nil
	}

	in := LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	}

	first, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("first enrichLookupResult() err = %v", err)
	}
	second, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("second enrichLookupResult() err = %v", err)
	}
	if first.HighContextPricing != nil || second.HighContextPricing != nil {
		t.Fatal("expected no high_context_pricing when both lookups fail")
	}
	if got := atomic.LoadInt32(&fetchCount); got != 4 {
		t.Fatalf("fetch count = %d, want 4", got)
	}
}

func TestEnrichLookupResult_OpenAISoftFailureFallsBackToOpenRouter(t *testing.T) {
	oldFetch := fetchURLFunc
	t.Cleanup(func() { fetchURLFunc = oldFetch })
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)

	var openRouterFetches int32
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		if strings.HasPrefix(url, "https://developers.openai.com/") {
			return "", errors.New("temporary upstream outage")
		}
		if !strings.Contains(url, "openrouter.ai/openai/gpt-5.4/pricing") {
			t.Fatalf("unexpected url: %s", url)
		}
		atomic.AddInt32(&openRouterFetches, 1)
		return `{"pricing_json":{"openai_responses:high_context_threshold":"272000","openai_responses:prompt_tokens_high_context":"5e-6","openai_responses:completion_tokens_high_context":"22.5e-6","openai_responses:cached_prompt_tokens_high_context":"0.5e-6"}}`, nil
	}

	res, err := enrichLookupResult(context.Background(), LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	})
	if err != nil {
		t.Fatalf("enrichLookupResult() err = %v", err)
	}
	if res.HighContextPricing == nil {
		t.Fatal("expected fallback high_context_pricing")
	}
	if res.SourceDetail != "models.dev" {
		t.Fatalf("source_detail=%q, want models.dev", res.SourceDetail)
	}
	if res.HighContextPricing.SourceDetail != "openrouter_pricing_page" {
		t.Fatalf("high_context source_detail=%q, want openrouter_pricing_page", res.HighContextPricing.SourceDetail)
	}
	if got := atomic.LoadInt32(&openRouterFetches); got != 1 {
		t.Fatalf("openrouter fetch count = %d, want 1", got)
	}
}

func TestEnrichLookupResult_CachesEmptyResultsWithShortTTL(t *testing.T) {
	oldFetch := fetchURLFunc
	oldTTL := highContextLookupCacheTTL
	oldNegativeTTL := highContextLookupNegativeCacheTTL
	t.Cleanup(func() {
		fetchURLFunc = oldFetch
		highContextLookupCacheTTL = oldTTL
		highContextLookupNegativeCacheTTL = oldNegativeTTL
	})
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	highContextLookupCacheTTL = time.Minute
	highContextLookupNegativeCacheTTL = time.Minute

	var fetchCount int32
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		atomic.AddInt32(&fetchCount, 1)
		if !strings.Contains(url, "openrouter.ai/moonshotai/kimi-k2/pricing") {
			t.Fatalf("unexpected url: %s", url)
		}
		return `{"pricing_json":{}}`, nil
	}

	in := LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "moonshotai",
		ModelID:             "moonshotai/kimi-k2",
		InputUSDPer1M:       decimal.RequireFromString("0.55"),
		OutputUSDPer1M:      decimal.RequireFromString("2.2"),
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
	}

	first, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("first enrichLookupResult() err = %v", err)
	}
	second, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("second enrichLookupResult() err = %v", err)
	}
	if first.HighContextPricing != nil || second.HighContextPricing != nil {
		t.Fatal("expected no high_context_pricing")
	}
	if got := atomic.LoadInt32(&fetchCount); got != 1 {
		t.Fatalf("fetch count = %d, want 1", got)
	}
}

func TestEnrichLookupResult_DoesNotNegativeCacheWhenOfficialLookupFails(t *testing.T) {
	oldFetch := fetchURLFunc
	oldTTL := highContextLookupCacheTTL
	oldNegativeTTL := highContextLookupNegativeCacheTTL
	t.Cleanup(func() {
		fetchURLFunc = oldFetch
		highContextLookupCacheTTL = oldTTL
		highContextLookupNegativeCacheTTL = oldNegativeTTL
	})
	resetHighContextLookupCache()
	t.Cleanup(resetHighContextLookupCache)
	highContextLookupCacheTTL = time.Minute
	highContextLookupNegativeCacheTTL = time.Minute

	var fetchCount int32
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		atomic.AddInt32(&fetchCount, 1)
		if strings.HasPrefix(url, "https://developers.openai.com/") {
			return "", errors.New("official docs unavailable")
		}
		if !strings.Contains(url, "openrouter.ai/openai/gpt-5.4/pricing") {
			t.Fatalf("unexpected url: %s", url)
		}
		return `{"pricing_json":{}}`, nil
	}

	in := LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	}

	first, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("first enrichLookupResult() err = %v", err)
	}
	second, err := enrichLookupResult(context.Background(), in)
	if err != nil {
		t.Fatalf("second enrichLookupResult() err = %v", err)
	}
	if first.HighContextPricing != nil || second.HighContextPricing != nil {
		t.Fatal("expected no high_context_pricing")
	}
	if got := atomic.LoadInt32(&fetchCount); got != 4 {
		t.Fatalf("fetch count = %d, want 4", got)
	}
}
