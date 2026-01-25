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
)

const DefaultModelsDevURL = "https://models.dev/api.json"

var (
	ErrModelNotFound   = errors.New("模型库中未找到该 model_id")
	ErrModelAmbiguous  = errors.New("模型库中存在多个候选，请使用 provider/model_id 指定")
	ErrModelNoPricing  = errors.New("模型库中该模型缺少定价信息")
	defaultCacheTTL    = 10 * time.Minute
	defaultHTTPTimeout = 8 * time.Second
)

type AmbiguousModelError struct {
	ModelID   string
	Providers []string
}

func (e *AmbiguousModelError) Error() string {
	if e == nil {
		return ErrModelAmbiguous.Error()
	}
	if len(e.Providers) == 0 {
		return ErrModelAmbiguous.Error()
	}
	return fmt.Sprintf("%s（候选 provider：%s）", ErrModelAmbiguous.Error(), strings.Join(e.Providers, ", "))
}

type ModelsDevCatalogOptions struct {
	URL string
	TTL time.Duration

	HTTPClient *http.Client
}

type ModelsDevCatalog struct {
	url    string
	ttl    time.Duration
	client *http.Client

	mu         sync.Mutex
	cachedAt   time.Time
	cachedETag string
	providers  map[string]modelsDevProvider
}

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID   string         `json:"id"`
	Cost *modelsDevCost `json:"cost"`
	// 其他字段在本需求中不使用，避免过度绑定 schema。
}

type modelsDevCost struct {
	Input     decimal.Decimal `json:"input"`
	Output    decimal.Decimal `json:"output"`
	CacheRead decimal.Decimal `json:"cache_read"`
}

type candidateMatch struct {
	providerID string
	model      modelsDevModel
}

type LookupResult struct {
	Source string

	// OwnedBy 是 Realms 的展示用归属方（用于图标映射与 /v1/models owned_by）。
	OwnedBy string

	// ModelID 是查询时使用的 model_id（通常等于 managed_models.public_id）。
	ModelID string

	InputUSDPer1M       decimal.Decimal
	OutputUSDPer1M      decimal.Decimal
	CacheInputUSDPer1M  decimal.Decimal
	CacheOutputUSDPer1M decimal.Decimal
}

func NewModelsDevCatalog(opts ModelsDevCatalogOptions) *ModelsDevCatalog {
	url := strings.TrimSpace(opts.URL)
	if url == "" {
		url = DefaultModelsDevURL
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &ModelsDevCatalog{
		url:    url,
		ttl:    ttl,
		client: client,
	}
}

func (c *ModelsDevCatalog) Lookup(ctx context.Context, modelID string) (LookupResult, error) {
	id := strings.TrimSpace(modelID)
	if id == "" {
		return LookupResult{}, errors.New("model_id 不能为空")
	}
	providers, err := c.getProviders(ctx)
	if err != nil {
		return LookupResult{}, err
	}

	// 支持显式 provider/model：例如 openai/gpt-4o（会映射到 provider=openai, model=gpt-4o）。
	if strings.Contains(id, "/") {
		providerID, subID, ok := strings.Cut(id, "/")
		providerID = strings.TrimSpace(providerID)
		subID = strings.TrimSpace(subID)
		if ok && providerID != "" && subID != "" {
			if p, ok := providers[providerID]; ok && p.Models != nil {
				if m, ok := p.Models[subID]; ok {
					res, ok, err := buildResultFromProviderModel("models.dev", id, m, providerID)
					if err != nil {
						return LookupResult{}, err
					}
					if ok {
						return res, nil
					}
				}
			}
		}
	}

	// 兜底处理 openrouter 风格：model_id 中包含 "/" 且数据源中存在 openrouter 同名 key（例如 moonshotai/kimi-k2）。
	if strings.Contains(id, "/") {
		if p, ok := providers["openrouter"]; ok && p.Models != nil {
			if m, ok := p.Models[id]; ok {
				res, ok, err := buildResultFromProviderModel("models.dev", id, m, ownedByFromCompositeID(id))
				if err != nil {
					return LookupResult{}, err
				}
				if ok {
					return res, nil
				}
			}
		}
	}

	var matches []candidateMatch
	for pid, p := range providers {
		if p.Models == nil {
			continue
		}
		m, ok := p.Models[id]
		if !ok {
			continue
		}
		if m.Cost == nil {
			continue
		}
		matches = append(matches, candidateMatch{providerID: pid, model: m})
	}
	if len(matches) == 0 {
		return LookupResult{}, ErrModelNotFound
	}
	if len(matches) > 1 {
		if picked, ok := pickPreferredMatch(id, matches); ok {
			res, ok, err := buildResultFromProviderModel("models.dev", id, picked.model, picked.providerID)
			if err != nil {
				return LookupResult{}, err
			}
			if ok {
				return res, nil
			}
		}
		ps := make([]string, 0, len(matches))
		for _, it := range matches {
			ps = append(ps, it.providerID)
		}
		sort.Strings(ps)
		return LookupResult{}, &AmbiguousModelError{ModelID: id, Providers: ps}
	}
	res, ok, err := buildResultFromProviderModel("models.dev", id, matches[0].model, matches[0].providerID)
	if err != nil {
		return LookupResult{}, err
	}
	if !ok {
		return LookupResult{}, ErrModelNoPricing
	}
	return res, nil
}

func ownedByFromCompositeID(modelID string) string {
	// 例如 moonshotai/kimi-k2 -> owned_by=moonshotai
	owner, _, ok := strings.Cut(strings.TrimSpace(modelID), "/")
	if !ok {
		return ""
	}
	return strings.TrimSpace(owner)
}

func pickPreferredMatch(modelID string, matches []candidateMatch) (candidateMatch, bool) {
	norm := normalizeModelIDForPick(modelID)
	if norm == "" || len(matches) == 0 {
		return candidateMatch{}, false
	}

	preferred := preferredProvidersForModelID(norm)
	if len(preferred) > 0 {
		for _, want := range preferred {
			for _, m := range matches {
				if m.providerID == want {
					return m, true
				}
			}
		}
	}
	return candidateMatch{}, false
}

func normalizeModelIDForPick(modelID string) string {
	s := strings.ToLower(strings.TrimSpace(modelID))
	if s == "" {
		return ""
	}
	// 兼容常见前缀样式：只保留字母数字与常用分隔符（-._/:），避免复杂匹配。
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == '/' || r == ':':
			b.WriteRune(r)
		default:
		}
	}
	return b.String()
}

func preferredProvidersForModelID(normID string) []string {
	switch {
	case containsAny(normID, "openai", "gpt", "dalle", "whisper", "textembedding", "textmoderation", "tts", "o1", "o3", "o4"):
		return []string{"openai"}
	case containsAny(normID, "anthropic", "claude"):
		return []string{"anthropic"}
	case containsAny(normID, "google", "gemini", "gemma", "vertex", "palm", "learnlm", "imagen", "veo"):
		// google-vertex 是常见发行渠道，但 Realms 的 owned_by 更偏向品牌；优先 google。
		return []string{"google", "google-vertex"}
	case containsAny(normID, "moonshot", "kimi"):
		return []string{"moonshotai"}
	case containsAny(normID, "zhipu", "chatglm", "glm", "cogview", "cogvideo"):
		return []string{"zhipuai"}
	case containsAny(normID, "qwen", "tongyi", "dashscope", "aliyun", "alibaba", "bailian"):
		// models.dev 的 provider 可能是 alibaba/alibaba-cn；这里优先 alibaba。
		return []string{"alibaba", "alibaba-cn"}
	case containsAny(normID, "deepseek"):
		return []string{"deepseek"}
	case containsAny(normID, "minimax", "abab"):
		return []string{"minimax"}
	case containsAny(normID, "wenxin", "ernie", "baidu"):
		return []string{"baidu"}
	case containsAny(normID, "spark", "xunfei", "iflytek"):
		return []string{"iflytek"}
	case containsAny(normID, "hunyuan", "tencent"):
		return []string{"tencent"}
	case containsAny(normID, "doubao", "bytedance", "volcengine"):
		return []string{"bytedance"}
	case containsAny(normID, "mistral"):
		return []string{"mistral"}
	case containsAny(normID, "cohere"):
		return []string{"cohere"}
	case containsAny(normID, "perplexity"):
		return []string{"perplexity"}
	case containsAny(normID, "xai"):
		return []string{"xai"}
	default:
		return nil
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func buildResultFromProviderModel(source string, queryID string, m modelsDevModel, ownedBy string) (LookupResult, bool, error) {
	if m.Cost == nil {
		return LookupResult{}, false, nil
	}
	if ownedBy == "" {
		return LookupResult{}, false, errors.New("模型库数据缺少归属方信息")
	}
	return LookupResult{
		Source:              source,
		OwnedBy:             ownedBy,
		ModelID:             strings.TrimSpace(queryID),
		InputUSDPer1M:       m.Cost.Input,
		OutputUSDPer1M:      m.Cost.Output,
		CacheInputUSDPer1M:  m.Cost.CacheRead,
		CacheOutputUSDPer1M: m.Cost.CacheRead,
	}, true, nil
}

func (c *ModelsDevCatalog) getProviders(ctx context.Context) (map[string]modelsDevProvider, error) {
	now := time.Now()

	c.mu.Lock()
	cached := c.providers
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
	req.Header.Set("User-Agent", "realms/1 (models.dev lookup)")
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
		if c.providers != nil {
			c.cachedAt = now
			out := c.providers
			c.mu.Unlock()
			return out, nil
		}
		c.mu.Unlock()
		// 兜底：本地无缓存但收到 304，视为异常，继续按失败处理。
		return nil, fmt.Errorf("models.dev 响应异常（304 但无本地缓存）")
	default:
		return nil, fmt.Errorf("models.dev 请求失败（HTTP %d）", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var providers map[string]modelsDevProvider
	if err := dec.Decode(&providers); err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("models.dev 返回空数据")
	}

	newETag := strings.TrimSpace(resp.Header.Get("ETag"))

	c.mu.Lock()
	c.providers = providers
	c.cachedAt = now
	c.cachedETag = newETag
	c.mu.Unlock()
	return providers, nil
}
