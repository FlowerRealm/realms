package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/modellibrary"
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

type fakeModelLibraryCatalog struct {
	lookup  func(ctx context.Context, modelID string) (modellibrary.LookupResult, error)
	suggest func(ctx context.Context, q string, limit int) ([]modellibrary.SuggestResult, error)
}

func (f fakeModelLibraryCatalog) Lookup(ctx context.Context, modelID string) (modellibrary.LookupResult, error) {
	if f.lookup == nil {
		return modellibrary.LookupResult{}, errors.New("lookup not implemented")
	}
	return f.lookup(ctx, modelID)
}

func (f fakeModelLibraryCatalog) Suggest(ctx context.Context, q string, limit int) ([]modellibrary.SuggestResult, error) {
	if f.suggest == nil {
		return nil, errors.New("suggest not implemented")
	}
	return f.suggest(ctx, q, limit)
}

func TestAdminModelLibraryLookupHandler_UsesOpenRouterCatalog(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()

	oldCatalog := modelLibraryCatalogImpl
	modelLibraryCatalogImpl = fakeModelLibraryCatalog{
		lookup: func(ctx context.Context, modelID string) (modellibrary.LookupResult, error) {
			if modelID != "openai/gpt-5.4" {
				t.Fatalf("Lookup modelID=%q", modelID)
			}
			return modellibrary.LookupResult{
				Source:              "openrouter",
				OwnedBy:             "openai",
				ModelID:             modelID,
				InputUSDPer1M:       mustDecimal(t, "2.5"),
				OutputUSDPer1M:      mustDecimal(t, "15"),
				CacheInputUSDPer1M:  mustDecimal(t, "0.25"),
				CacheOutputUSDPer1M: mustDecimal(t, "0.25"),
			}, nil
		},
	}
	t.Cleanup(func() {
		modelLibraryCatalogImpl = oldCatalog
	})

	engine, sessionCookie, userID := setupRootSession(t, st)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/models/library-lookup", strings.NewReader(`{"model_id":"openai/gpt-5.4"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			OwnedBy             string `json:"owned_by"`
			InputUSDPer1M       string `json:"input_usd_per_1m"`
			OutputUSDPer1M      string `json:"output_usd_per_1m"`
			CacheInputUSDPer1M  string `json:"cache_input_usd_per_1m"`
			CacheOutputUSDPer1M string `json:"cache_output_usd_per_1m"`
			Source              string `json:"source"`
			IconURL             string `json:"icon_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if got.Message != "已从 OpenRouter 填充" {
		t.Fatalf("message=%q", got.Message)
	}
	if got.Data.Source != "openrouter" {
		t.Fatalf("source=%q", got.Data.Source)
	}
	if got.Data.OwnedBy != "openai" {
		t.Fatalf("owned_by=%q", got.Data.OwnedBy)
	}
	if got.Data.InputUSDPer1M != "2.5" {
		t.Fatalf("input=%q", got.Data.InputUSDPer1M)
	}
	if got.Data.OutputUSDPer1M != "15" {
		t.Fatalf("output=%q", got.Data.OutputUSDPer1M)
	}
	if got.Data.IconURL == "" {
		t.Fatal("expected icon_url")
	}
}

func TestAdminModelLibrarySuggestHandler_ReturnsOpenRouterMatches(t *testing.T) {
	st, cleanup := newTestSQLiteStore(t)
	defer cleanup()

	oldCatalog := modelLibraryCatalogImpl
	modelLibraryCatalogImpl = fakeModelLibraryCatalog{
		suggest: func(ctx context.Context, q string, limit int) ([]modellibrary.SuggestResult, error) {
			if q != "gpt" {
				t.Fatalf("Suggest q=%q", q)
			}
			if limit != 2 {
				t.Fatalf("Suggest limit=%d", limit)
			}
			return []modellibrary.SuggestResult{
				{
					ModelID: "openai/gpt-5.4",
					Name:    "OpenAI: GPT-5.4",
					OwnedBy: "openai",
				},
				{
					ModelID: "openai/gpt-5.4-mini",
					Name:    "OpenAI: GPT-5.4 Mini",
					OwnedBy: "openai",
				},
			}, nil
		},
	}
	t.Cleanup(func() {
		modelLibraryCatalogImpl = oldCatalog
	})

	engine, sessionCookie, userID := setupRootSession(t, st)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/models/library-suggest?q=gpt&limit=2", nil)
	req.Header.Set("Cookie", sessionCookie)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			OwnedBy string `json:"owned_by"`
			IconURL string `json:"icon_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success, got message=%q", got.Message)
	}
	if len(got.Data) != 2 {
		t.Fatalf("len(data)=%d", len(got.Data))
	}
	if got.Data[0].ID != "openai/gpt-5.4" {
		t.Fatalf("first model=%q", got.Data[0].ID)
	}
	if got.Data[0].Name != "OpenAI: GPT-5.4" {
		t.Fatalf("first name=%q", got.Data[0].Name)
	}
	if got.Data[0].OwnedBy != "openai" {
		t.Fatalf("first owned_by=%q", got.Data[0].OwnedBy)
	}
	if got.Data[0].IconURL == "" {
		t.Fatal("first icon_url empty")
	}
	if got.Data[1].ID != "openai/gpt-5.4-mini" {
		t.Fatalf("second model=%q", got.Data[1].ID)
	}
}
