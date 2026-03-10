package modellibrary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

const DefaultOpenRouterModelsURL = "https://openrouter.ai/api/v1/models"

var (
	ErrModelNotFound   = errors.New("OpenRouter 中未找到该 model_id")
	ErrModelNoPricing  = errors.New("OpenRouter 中该模型缺少定价信息")
	defaultCacheTTL    = 10 * time.Minute
	defaultHTTPTimeout = 8 * time.Second
	usdPerMillion      = decimal.RequireFromString("1000000")
)

type OpenRouterCatalogOptions struct {
	URL string
	TTL time.Duration

	HTTPClient *http.Client
}

type OpenRouterCatalog struct {
	url    string
	ttl    time.Duration
	client *http.Client

	mu         sync.Mutex
	cachedAt   time.Time
	cachedETag string
	models     []openRouterModel
}

type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Pricing openRouterModelPricing `json:"pricing"`
}

type openRouterModelPricing struct {
	Prompt         string `json:"prompt"`
	Completion     string `json:"completion"`
	InputCacheRead string `json:"input_cache_read"`
}

type LookupResult struct {
	Source       string
	SourceDetail string

	OwnedBy string
	ModelID string

	InputUSDPer1M       decimal.Decimal
	OutputUSDPer1M      decimal.Decimal
	CacheInputUSDPer1M  decimal.Decimal
	CacheOutputUSDPer1M decimal.Decimal
	HighContextPricing  *store.ManagedModelHighContextPricing
}

type SuggestResult struct {
	ModelID string
	Name    string
	OwnedBy string
}

func NewModelsDevCatalog(opts OpenRouterCatalogOptions) *OpenRouterCatalog {
	url := strings.TrimSpace(opts.URL)
	if url == "" {
		url = DefaultOpenRouterModelsURL
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &OpenRouterCatalog{
		url:    url,
		ttl:    ttl,
		client: client,
	}
}

func (c *OpenRouterCatalog) Lookup(ctx context.Context, modelID string) (LookupResult, error) {
	id := strings.TrimSpace(modelID)
	if id == "" {
		return LookupResult{}, errors.New("model_id 不能为空")
	}
	models, err := c.getModels(ctx)
	if err != nil {
		return LookupResult{}, err
	}
	for _, model := range models {
		if strings.TrimSpace(model.ID) != id {
			continue
		}
		res, err := buildLookupResult("openrouter", model)
		if err != nil {
			return LookupResult{}, err
		}
		return enrichLookupResult(ctx, res)
	}
	return LookupResult{}, ErrModelNotFound
}

func (c *OpenRouterCatalog) Suggest(ctx context.Context, q string, limit int) ([]SuggestResult, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return []SuggestResult{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	models, err := c.getModels(ctx)
	if err != nil {
		return nil, err
	}

	type scoredModel struct {
		model openRouterModel
		score int
	}
	needle := strings.ToLower(query)
	matches := make([]scoredModel, 0, limit)
	for _, model := range models {
		score := scoreOpenRouterModel(model, needle)
		if score < 0 {
			continue
		}
		matches = append(matches, scoredModel{model: model, score: score})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		left := strings.TrimSpace(matches[i].model.ID)
		right := strings.TrimSpace(matches[j].model.ID)
		return left < right
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]SuggestResult, 0, len(matches))
	for _, item := range matches {
		out = append(out, buildSuggestResult(item.model))
	}
	return out, nil
}

func scoreOpenRouterModel(model openRouterModel, needle string) int {
	id := strings.ToLower(strings.TrimSpace(model.ID))
	name := strings.ToLower(strings.TrimSpace(model.Name))
	switch {
	case id == needle:
		return 0
	case strings.HasPrefix(id, needle):
		return 1
	case strings.Contains(id, needle):
		return 2
	case name != "" && strings.HasPrefix(name, needle):
		return 3
	case name != "" && strings.Contains(name, needle):
		return 4
	default:
		return -1
	}
}

func ownedByFromCompositeID(modelID string) string {
	owner, _, ok := strings.Cut(strings.TrimSpace(modelID), "/")
	if !ok {
		return ""
	}
	return strings.TrimSpace(owner)
}

func buildLookupResult(source string, model openRouterModel) (LookupResult, error) {
	pricing, ok, err := buildPricing(model.Pricing)
	if err != nil {
		return LookupResult{}, err
	}
	if !ok {
		return LookupResult{}, ErrModelNoPricing
	}
	ownedBy := ownedByFromCompositeID(model.ID)
	if ownedBy == "" {
		return LookupResult{}, errors.New("OpenRouter 模型数据缺少归属方信息")
	}
	return LookupResult{
		Source:              source,
		SourceDetail:        source,
		OwnedBy:             ownedBy,
		ModelID:             strings.TrimSpace(model.ID),
		InputUSDPer1M:       pricing.input,
		OutputUSDPer1M:      pricing.output,
		CacheInputUSDPer1M:  pricing.cacheRead,
		CacheOutputUSDPer1M: pricing.cacheRead,
	}, nil
}

func buildSuggestResult(model openRouterModel) SuggestResult {
	return SuggestResult{
		ModelID: strings.TrimSpace(model.ID),
		Name:    strings.TrimSpace(model.Name),
		OwnedBy: ownedByFromCompositeID(model.ID),
	}
}

type normalizedPricing struct {
	input     decimal.Decimal
	output    decimal.Decimal
	cacheRead decimal.Decimal
}

func buildPricing(pricing openRouterModelPricing) (normalizedPricing, bool, error) {
	input, inputSet, err := parsePricePerToken(pricing.Prompt)
	if err != nil {
		return normalizedPricing{}, false, fmt.Errorf("OpenRouter prompt 定价不合法: %w", err)
	}
	output, outputSet, err := parsePricePerToken(pricing.Completion)
	if err != nil {
		return normalizedPricing{}, false, fmt.Errorf("OpenRouter completion 定价不合法: %w", err)
	}
	cacheRead, cacheSet, err := parsePricePerToken(pricing.InputCacheRead)
	if err != nil {
		return normalizedPricing{}, false, fmt.Errorf("OpenRouter input_cache_read 定价不合法: %w", err)
	}
	if !inputSet && !outputSet && !cacheSet {
		return normalizedPricing{}, false, nil
	}
	if !cacheSet {
		cacheRead = decimal.Zero
	}
	return normalizedPricing{
		input:     input,
		output:    output,
		cacheRead: cacheRead,
	}, true, nil
}

func parsePricePerToken(raw string) (decimal.Decimal, bool, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return decimal.Zero, false, nil
	}
	if s == "-1" {
		return decimal.Zero, false, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, false, err
	}
	return d.Mul(usdPerMillion), true, nil
}

func (c *OpenRouterCatalog) getModels(ctx context.Context) ([]openRouterModel, error) {
	now := time.Now()

	c.mu.Lock()
	cached := c.models
	cachedAt := c.cachedAt
	etag := c.cachedETag
	ttl := c.ttl
	c.mu.Unlock()

	if cached != nil && now.Sub(cachedAt) <= ttl {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "realms/1 (OpenRouter lookup)")
	if strings.TrimSpace(etag) != "" {
		req.Header.Set("If-None-Match", strings.TrimSpace(etag))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		c.mu.Lock()
		if c.models != nil {
			c.cachedAt = now
			out := c.models
			c.mu.Unlock()
			return out, nil
		}
		c.mu.Unlock()
		return nil, fmt.Errorf("OpenRouter 响应异常（304 但无本地缓存）")
	default:
		return nil, fmt.Errorf("OpenRouter 请求失败（HTTP %d）", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var payload openRouterModelsResponse
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("OpenRouter 返回空数据")
	}

	newETag := strings.TrimSpace(resp.Header.Get("ETag"))
	models := compactOpenRouterModels(payload.Data)

	c.mu.Lock()
	c.models = models
	c.cachedAt = now
	c.cachedETag = newETag
	c.mu.Unlock()
	return models, nil
}

func compactOpenRouterModels(items []openRouterModel) []openRouterModel {
	seen := make(map[string]openRouterModel, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		item.ID = id
		item.Name = strings.TrimSpace(item.Name)
		seen[id] = item
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]openRouterModel, 0, len(ids))
	for _, id := range ids {
		out = append(out, seen[id])
	}
	return out
}
