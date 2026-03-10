package modellibrary

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

var defaultModelLibraryHTTPClient = &http.Client{Timeout: 10 * time.Second}
var fetchURLFunc = fetchURL

func enrichLookupResult(ctx context.Context, in LookupResult) LookupResult {
	out := in
	if hc, detail, err := lookupOpenAIHighContextPricing(ctx, in.ModelID, in); err == nil && hc != nil {
		out.HighContextPricing = hc
		out.SourceDetail = detail
		return out
	}
	if hc, detail, err := lookupOpenRouterHighContextPricing(ctx, in.ModelID, in); err == nil && hc != nil {
		out.HighContextPricing = hc
		out.SourceDetail = detail
	}
	return out
}

func lookupOpenAIHighContextPricing(ctx context.Context, modelID string, base LookupResult) (*store.ManagedModelHighContextPricing, string, error) {
	if strings.TrimSpace(base.OwnedBy) != "openai" {
		return nil, "", nil
	}
	norm := strings.TrimSpace(modelID)
	if idx := strings.Index(norm, "/"); idx >= 0 {
		norm = strings.TrimSpace(norm[idx+1:])
	}
	if norm != "gpt-5.4" && norm != "gpt-5.4-pro" {
		return nil, "", nil
	}
	body, err := fetchURLFunc(ctx, defaultModelLibraryHTTPClient, "https://developers.openai.com/api/docs/pricing/")
	if err != nil {
		return nil, "", err
	}
	guide, err := fetchURLFunc(ctx, defaultModelLibraryHTTPClient, "https://developers.openai.com/api/docs/guides/latest-model/")
	if err != nil {
		return nil, "", err
	}
	policy := store.ManagedModelHighContextServiceTierPolicyInherit
	if strings.Contains(guide, "above 272K tokens is automatically processed at standard rates") {
		policy = store.ManagedModelHighContextServiceTierPolicyForceStandard
	}
	switch norm {
	case "gpt-5.4":
		if !strings.Contains(body, "gpt-5.4 (&gt;272K context length)") {
			return nil, "", errors.New("openai pricing page missing gpt-5.4 high context row")
		}
		return &store.ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            store.ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    policy,
			InputUSDPer1M:        decimal.RequireFromString("5"),
			OutputUSDPer1M:       decimal.RequireFromString("22.5"),
			CacheInputUSDPer1M:   decimalPtr(decimal.RequireFromString("0.5")),
			Source:               "openai_official",
			SourceDetail:         "openai_official_pricing_docs",
		}, "openai_official_pricing_docs", nil
	case "gpt-5.4-pro":
		if !strings.Contains(body, "gpt-5.4-pro (&gt;272K context length)") {
			return nil, "", errors.New("openai pricing page missing gpt-5.4-pro high context row")
		}
		return &store.ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            store.ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    policy,
			InputUSDPer1M:        decimal.RequireFromString("60"),
			OutputUSDPer1M:       decimal.RequireFromString("270"),
			Source:               "openai_official",
			SourceDetail:         "openai_official_pricing_docs",
		}, "openai_official_pricing_docs", nil
	default:
		return nil, "", nil
	}
}

func lookupOpenRouterHighContextPricing(ctx context.Context, modelID string, base LookupResult) (*store.ManagedModelHighContextPricing, string, error) {
	pageModelID := strings.TrimSpace(modelID)
	if pageModelID == "" {
		return nil, "", nil
	}
	if !strings.Contains(pageModelID, "/") {
		owner := strings.TrimSpace(base.OwnedBy)
		if owner == "" {
			return nil, "", nil
		}
		pageModelID = owner + "/" + pageModelID
	}
	body, err := fetchURLFunc(ctx, defaultModelLibraryHTTPClient, "https://openrouter.ai/"+pageModelID+"/pricing")
	if err != nil {
		return nil, "", err
	}
	threshold, ok := parseOpenRouterInt(body, `high_context_threshold":"([0-9]+)"`)
	if !ok || threshold <= 0 {
		return nil, "", nil
	}
	inPrice, ok := parseOpenRouterDecimal(body, `prompt_tokens_high_context":"([0-9.eE+-]+)"`)
	if !ok {
		return nil, "", nil
	}
	outPrice, ok := parseOpenRouterDecimal(body, `completion_tokens_high_context":"([0-9.eE+-]+)"`)
	if !ok {
		return nil, "", nil
	}
	cachePrice, hasCache := parseOpenRouterDecimal(body, `cached_prompt_tokens_high_context":"([0-9.eE+-]+)"`)
	hc := &store.ManagedModelHighContextPricing{
		ThresholdInputTokens: threshold,
		AppliesTo:            store.ManagedModelHighContextAppliesToFullRequest,
		ServiceTierPolicy:    store.ManagedModelHighContextServiceTierPolicyInherit,
		InputUSDPer1M:        inPrice.Mul(decimal.NewFromInt(1_000_000)).Truncate(store.USDScale),
		OutputUSDPer1M:       outPrice.Mul(decimal.NewFromInt(1_000_000)).Truncate(store.USDScale),
		Source:               "openrouter",
		SourceDetail:         "openrouter_pricing_page",
	}
	if hasCache {
		cacheUSD := cachePrice.Mul(decimal.NewFromInt(1_000_000)).Truncate(store.USDScale)
		hc.CacheInputUSDPer1M = decimalPtr(cacheUSD)
	}
	return hc, "openrouter_pricing_page", nil
}

func fetchURL(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "realms/1 (model library lookup)")
	req.Header.Set("Accept", "text/html,application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("upstream fetch failed")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseOpenRouterInt(body string, pattern string) (int64, bool) {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	return n, err == nil
}

func parseOpenRouterDecimal(body string, pattern string) (decimal.Decimal, bool) {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(m[1])
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

func decimalPtr(d decimal.Decimal) *decimal.Decimal {
	out := d
	return &out
}
