package modellibrary

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
