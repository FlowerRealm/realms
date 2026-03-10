package modellibrary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestModelsDevCatalog_Lookup_OpenAI(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"id":   "openai",
				"name": "OpenAI",
				"models": map[string]any{
					"gpt-4o": map[string]any{
						"id": "gpt-4o",
						"cost": map[string]any{
							"input":      2.5,
							"output":     10,
							"cache_read": 1.25,
						},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(ModelsDevCatalogOptions{
		URL: srv.URL,
		TTL: time.Minute,
	})

	res, err := c.Lookup(context.Background(), "gpt-4o")
	if err != nil {
		t.Fatalf("Lookup() err = %v", err)
	}
	if res.OwnedBy != "openai" {
		t.Fatalf("OwnedBy = %q, want %q", res.OwnedBy, "openai")
	}
	if !res.InputUSDPer1M.Equal(decimal.RequireFromString("2.5")) {
		t.Fatalf("InputUSDPer1M = %s", res.InputUSDPer1M)
	}
	if !res.OutputUSDPer1M.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("OutputUSDPer1M = %s", res.OutputUSDPer1M)
	}
	if !res.CacheInputUSDPer1M.Equal(decimal.RequireFromString("1.25")) {
		t.Fatalf("CacheInputUSDPer1M = %s", res.CacheInputUSDPer1M)
	}
	if !res.CacheOutputUSDPer1M.Equal(decimal.RequireFromString("1.25")) {
		t.Fatalf("CacheOutputUSDPer1M = %s", res.CacheOutputUSDPer1M)
	}
}

func TestModelsDevCatalog_Lookup_PreferOpenAIWhenAmbiguous(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"id": "openai",
				"models": map[string]any{
					"gpt-4o": map[string]any{
						"id": "gpt-4o",
						"cost": map[string]any{
							"input":      2.5,
							"output":     10,
							"cache_read": 1.25,
						},
					},
				},
			},
			"azure": map[string]any{
				"id": "azure",
				"models": map[string]any{
					"gpt-4o": map[string]any{
						"id": "gpt-4o",
						"cost": map[string]any{
							"input":  3,
							"output": 12,
						},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(ModelsDevCatalogOptions{URL: srv.URL, TTL: time.Minute})

	res, err := c.Lookup(context.Background(), "gpt-4o")
	if err != nil {
		t.Fatalf("Lookup() err = %v", err)
	}
	if res.OwnedBy != "openai" {
		t.Fatalf("OwnedBy = %q, want %q", res.OwnedBy, "openai")
	}
	if !res.InputUSDPer1M.Equal(decimal.RequireFromString("2.5")) {
		t.Fatalf("InputUSDPer1M = %s", res.InputUSDPer1M)
	}
	if !res.OutputUSDPer1M.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("OutputUSDPer1M = %s", res.OutputUSDPer1M)
	}
}

func TestModelsDevCatalog_Lookup_ExplicitProviderBeatsOpenRouter(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"id": "openai",
				"models": map[string]any{
					"gpt-4o": map[string]any{
						"id": "gpt-4o",
						"cost": map[string]any{
							"input":      2.5,
							"output":     10,
							"cache_read": 1.25,
						},
					},
				},
			},
			"openrouter": map[string]any{
				"id": "openrouter",
				"models": map[string]any{
					"openai/gpt-4o": map[string]any{
						"id": "openai/gpt-4o",
						"cost": map[string]any{
							"input":  100,
							"output": 200,
						},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(ModelsDevCatalogOptions{URL: srv.URL, TTL: time.Minute})

	res, err := c.Lookup(context.Background(), "openai/gpt-4o")
	if err != nil {
		t.Fatalf("Lookup() err = %v", err)
	}
	if res.OwnedBy != "openai" {
		t.Fatalf("OwnedBy = %q, want %q", res.OwnedBy, "openai")
	}
	if !res.InputUSDPer1M.Equal(decimal.RequireFromString("2.5")) {
		t.Fatalf("InputUSDPer1M = %s", res.InputUSDPer1M)
	}
	if !res.OutputUSDPer1M.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("OutputUSDPer1M = %s", res.OutputUSDPer1M)
	}
}

func TestModelsDevCatalog_Lookup_OpenRouterCompositeID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openrouter": map[string]any{
				"id":   "openrouter",
				"name": "OpenRouter",
				"models": map[string]any{
					"moonshotai/kimi-k2": map[string]any{
						"id": "moonshotai/kimi-k2",
						"cost": map[string]any{
							"input":  0.55,
							"output": 2.2,
						},
					},
				},
			},
			"moonshotai": map[string]any{
				"id":   "moonshotai",
				"name": "Moonshot",
				"models": map[string]any{
					"kimi-k2": map[string]any{
						"id":   "kimi-k2",
						"cost": nil,
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(ModelsDevCatalogOptions{
		URL: srv.URL,
		TTL: time.Minute,
	})

	res, err := c.Lookup(context.Background(), "moonshotai/kimi-k2")
	if err != nil {
		t.Fatalf("Lookup() err = %v", err)
	}
	if res.OwnedBy != "moonshotai" {
		t.Fatalf("OwnedBy = %q, want %q", res.OwnedBy, "moonshotai")
	}
	if !res.InputUSDPer1M.Equal(decimal.RequireFromString("0.55")) {
		t.Fatalf("InputUSDPer1M = %s", res.InputUSDPer1M)
	}
	if !res.OutputUSDPer1M.Equal(decimal.RequireFromString("2.2")) {
		t.Fatalf("OutputUSDPer1M = %s", res.OutputUSDPer1M)
	}
	if !res.CacheInputUSDPer1M.Equal(decimal.Zero) {
		t.Fatalf("CacheInputUSDPer1M = %s", res.CacheInputUSDPer1M)
	}
	if !res.CacheOutputUSDPer1M.Equal(decimal.Zero) {
		t.Fatalf("CacheOutputUSDPer1M = %s", res.CacheOutputUSDPer1M)
	}
}

func TestModelsDevCatalog_Lookup_Ambiguous(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"id": "openai",
				"models": map[string]any{
					"x": map[string]any{"id": "x", "cost": map[string]any{"input": 1, "output": 2}},
				},
			},
			"anthropic": map[string]any{
				"id": "anthropic",
				"models": map[string]any{
					"x": map[string]any{"id": "x", "cost": map[string]any{"input": 1, "output": 2}},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewModelsDevCatalog(ModelsDevCatalogOptions{URL: srv.URL, TTL: time.Minute})

	_, err := c.Lookup(context.Background(), "x")
	if err == nil {
		t.Fatalf("Lookup() err = nil, want ambiguous error")
	}
	var amb *AmbiguousModelError
	if !errors.As(err, &amb) {
		t.Fatalf("Lookup() err = %T, want *AmbiguousModelError", err)
	}
	if len(amb.Providers) != 2 {
		t.Fatalf("Providers = %v, want 2 items", amb.Providers)
	}
}

func TestEnrichLookupResult_OpenRouterHighContextFallback(t *testing.T) {
	oldFetch := fetchURLFunc
	t.Cleanup(func() { fetchURLFunc = oldFetch })
	fetchURLFunc = func(ctx context.Context, client *http.Client, url string) (string, error) {
		if strings.HasPrefix(url, "https://developers.openai.com/") {
			return "", errors.New("official docs unavailable")
		}
		if !strings.Contains(url, "openrouter.ai/openai/gpt-5.4/pricing") {
			t.Fatalf("unexpected url: %s", url)
		}
		return `{"pricing_json":{"openai_responses:high_context_threshold":"272000","openai_responses:prompt_tokens_high_context":"5e-6","openai_responses:completion_tokens_high_context":"22.5e-6","openai_responses:cached_prompt_tokens_high_context":"0.5e-6"}}`, nil
	}

	res := enrichLookupResult(context.Background(), LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "openai/gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	})
	if res.HighContextPricing == nil {
		t.Fatal("expected high_context_pricing")
	}
	if res.HighContextPricing.ThresholdInputTokens != 272000 {
		t.Fatalf("threshold=%d, want 272000", res.HighContextPricing.ThresholdInputTokens)
	}
	if !res.HighContextPricing.InputUSDPer1M.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("input=%s, want 5", res.HighContextPricing.InputUSDPer1M)
	}
	if res.SourceDetail != "openrouter_pricing_page" {
		t.Fatalf("source_detail=%q, want openrouter_pricing_page", res.SourceDetail)
	}
}

func TestEnrichLookupResult_OpenAIOfficialHighContextPricing(t *testing.T) {
	oldFetch := fetchURLFunc
	t.Cleanup(func() { fetchURLFunc = oldFetch })
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

	res := enrichLookupResult(context.Background(), LookupResult{
		Source:              "models.dev",
		SourceDetail:        "models.dev",
		OwnedBy:             "openai",
		ModelID:             "gpt-5.4",
		InputUSDPer1M:       decimal.RequireFromString("2.5"),
		OutputUSDPer1M:      decimal.RequireFromString("15"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0.25"),
	})
	if res.HighContextPricing == nil {
		t.Fatal("expected high_context_pricing")
	}
	if res.HighContextPricing.ServiceTierPolicy != "force_standard" {
		t.Fatalf("service_tier_policy=%q, want force_standard", res.HighContextPricing.ServiceTierPolicy)
	}
	if !res.HighContextPricing.OutputUSDPer1M.Equal(decimal.RequireFromString("22.5")) {
		t.Fatalf("output=%s, want 22.5", res.HighContextPricing.OutputUSDPer1M)
	}
	if res.SourceDetail != "openai_official_pricing_docs" {
		t.Fatalf("source_detail=%q, want openai_official_pricing_docs", res.SourceDetail)
	}
}
