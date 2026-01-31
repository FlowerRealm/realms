package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/icons"
	"realms/internal/modellibrary"
	"realms/internal/store"
)

var modelsDevCatalog = modellibrary.NewModelsDevCatalog(modellibrary.ModelsDevCatalogOptions{})

type modelLibraryLookupRequest struct {
	ModelID string `json:"model_id"`
}

type modelLibraryLookupResult struct {
	OwnedBy             string `json:"owned_by"`
	InputUSDPer1M       string `json:"input_usd_per_1m"`
	OutputUSDPer1M      string `json:"output_usd_per_1m"`
	CacheInputUSDPer1M  string `json:"cache_input_usd_per_1m"`
	CacheOutputUSDPer1M string `json:"cache_output_usd_per_1m"`
	Source              string `json:"source"`
	IconURL             string `json:"icon_url"`
}

func adminModelLibraryLookupHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req modelLibraryLookupRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		modelID := strings.TrimSpace(req.ModelID)
		if modelID == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "model_id 不能为空"})
			return
		}

		res, err := modelsDevCatalog.Lookup(c.Request.Context(), modelID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		out := modelLibraryLookupResult{
			Source:              res.Source,
			OwnedBy:             res.OwnedBy,
			InputUSDPer1M:       formatUSDPlain(res.InputUSDPer1M),
			OutputUSDPer1M:      formatUSDPlain(res.OutputUSDPer1M),
			CacheInputUSDPer1M:  formatUSDPlain(res.CacheInputUSDPer1M),
			CacheOutputUSDPer1M: formatUSDPlain(res.CacheOutputUSDPer1M),
			IconURL:             icons.ModelIconURL(modelID, res.OwnedBy),
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已从模型库填充（models.dev）", "data": out})
	}
}

type importModelPricingRequest struct {
	PricingJSON string `json:"pricing_json"`
}

type importModelPricingResult struct {
	Added     []string          `json:"added"`
	Updated   []string          `json:"updated"`
	Unchanged []string          `json:"unchanged"`
	Failed    map[string]string `json:"failed"`
}

func adminImportModelPricingHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req importModelPricingRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		raw := strings.TrimSpace(req.PricingJSON)
		if raw == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "请粘贴 JSON 内容"})
			return
		}

		parsed, err := parsePricingImportJSON([]byte(raw))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "JSON 解析失败：" + err.Error()})
			return
		}

		upsertRes, err := opts.Store.UpsertManagedModelPricing(c.Request.Context(), parsed.items)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "导入失败"})
			return
		}

		msg := fmt.Sprintf("导入完成：新增 %d，更新 %d，无变化 %d，失败 %d。", len(upsertRes.Added), len(upsertRes.Updated), len(upsertRes.Unchanged), len(parsed.failed))
		if len(parsed.failed) > 0 {
			msg += " 失败示例：" + summarizeFailedItems(parsed.failed, 3)
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": msg,
			"data": importModelPricingResult{
				Added:     upsertRes.Added,
				Updated:   upsertRes.Updated,
				Unchanged: upsertRes.Unchanged,
				Failed:    parsed.failed,
			},
		})
	}
}

type pricingImportParseResult struct {
	items  []store.ManagedModelPricingUpsert
	failed map[string]string
}

func parsePricingImportJSON(b []byte) (pricingImportParseResult, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return pricingImportParseResult{}, fmt.Errorf("JSON 为空")
	}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var top any
	if err := dec.Decode(&top); err != nil {
		return pricingImportParseResult{}, err
	}

	out := pricingImportParseResult{
		failed: make(map[string]string),
	}

	addItem := func(modelID string, item store.ManagedModelPricingUpsert) {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			return
		}
		item.PublicID = modelID
		out.items = append(out.items, item)
	}

	switch v := top.(type) {
	case map[string]any:
		for modelID, raw := range v {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" {
				continue
			}
			m, ok := raw.(map[string]any)
			if !ok {
				out.failed[modelID] = "条目必须是对象"
				continue
			}
			item, ok, reason := parsePricingFromMap(m)
			if !ok {
				out.failed[modelID] = reason
				continue
			}
			addItem(modelID, item)
		}
	case []any:
		for i, raw := range v {
			m, ok := raw.(map[string]any)
			key := fmt.Sprintf("#%d", i+1)
			if !ok {
				out.failed[key] = "条目必须是对象"
				continue
			}
			modelID := pickString(m, "public_id", "model", "id", "name", "model_name")
			modelID = strings.TrimSpace(modelID)
			if modelID == "" {
				out.failed[key] = "缺少 model/public_id 字段"
				continue
			}
			item, ok, reason := parsePricingFromMap(m)
			if !ok {
				out.failed[modelID] = reason
				continue
			}
			addItem(modelID, item)
		}
	default:
		return pricingImportParseResult{}, fmt.Errorf("JSON 顶层必须是对象或数组")
	}

	// 合并同名模型：后者覆盖前者（更符合“导入”的直觉）。
	if len(out.items) == 0 {
		return out, nil
	}
	byID := make(map[string]store.ManagedModelPricingUpsert, len(out.items))
	for _, it := range out.items {
		id := strings.TrimSpace(it.PublicID)
		if id == "" {
			continue
		}
		byID[id] = it
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out.items = out.items[:0]
	for _, id := range ids {
		out.items = append(out.items, byID[id])
	}
	return out, nil
}

func parsePricingFromMap(m map[string]any) (store.ManagedModelPricingUpsert, bool, string) {
	hasUSD := hasAnyKey(m, "input_usd_per_1m", "output_usd_per_1m", "cache_input_usd_per_1m", "cache_output_usd_per_1m")
	hasToken := hasAnyKey(m, "input_cost_per_token", "output_cost_per_token", "cache_read_input_cost_per_token", "cache_read_output_cost_per_token", "cache_read_cost_per_token")

	if !hasUSD && !hasToken {
		return store.ManagedModelPricingUpsert{}, false, "缺少定价字段（usd_per_1m 或 cost_per_token）"
	}

	if hasUSD {
		inUSD, err := parseDecimalFieldOptional(m, "input_usd_per_1m")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "input_usd_per_1m 不合法"
		}
		outUSD, err := parseDecimalFieldOptional(m, "output_usd_per_1m")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "output_usd_per_1m 不合法"
		}
		cacheInUSD, err := parseDecimalFieldOptional(m, "cache_input_usd_per_1m")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "cache_input_usd_per_1m 不合法"
		}
		cacheOutUSD, err := parseDecimalFieldOptional(m, "cache_output_usd_per_1m")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "cache_output_usd_per_1m 不合法"
		}
		if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
			return store.ManagedModelPricingUpsert{}, false, "定价不能为负数"
		}
		return store.ManagedModelPricingUpsert{
			InputUSDPer1M:       inUSD,
			OutputUSDPer1M:      outUSD,
			CacheInputUSDPer1M:  cacheInUSD,
			CacheOutputUSDPer1M: cacheOutUSD,
		}, true, ""
	}

	// LiteLLM 常见格式：cost_per_token -> usd_per_1m
	inTok, err := parseDecimalFieldOptional(m, "input_cost_per_token")
	if err != nil {
		return store.ManagedModelPricingUpsert{}, false, "input_cost_per_token 不合法"
	}
	outTok, err := parseDecimalFieldOptional(m, "output_cost_per_token")
	if err != nil {
		return store.ManagedModelPricingUpsert{}, false, "output_cost_per_token 不合法"
	}

	cacheInTok, err := parseDecimalFieldOptional(m, "cache_read_input_cost_per_token")
	if err != nil {
		return store.ManagedModelPricingUpsert{}, false, "cache_read_input_cost_per_token 不合法"
	}
	cacheOutTok, err := parseDecimalFieldOptional(m, "cache_read_output_cost_per_token")
	if err != nil {
		return store.ManagedModelPricingUpsert{}, false, "cache_read_output_cost_per_token 不合法"
	}

	// LiteLLM 常见兜底字段：cache_read_cost_per_token（若缺少 input/output 维度，则视为同价）。
	if cacheInTok.IsZero() || cacheOutTok.IsZero() {
		cacheTok, err := parseDecimalFieldOptional(m, "cache_read_cost_per_token")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "cache_read_cost_per_token 不合法"
		}
		if cacheInTok.IsZero() {
			cacheInTok = cacheTok
		}
		if cacheOutTok.IsZero() {
			cacheOutTok = cacheTok
		}
	}

	million := decimal.NewFromInt(1_000_000)
	inUSD := inTok.Mul(million)
	outUSD := outTok.Mul(million)
	cacheInUSD := cacheInTok.Mul(million)
	cacheOutUSD := cacheOutTok.Mul(million)
	if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
		return store.ManagedModelPricingUpsert{}, false, "定价不能为负数"
	}
	return store.ManagedModelPricingUpsert{
		InputUSDPer1M:       inUSD,
		OutputUSDPer1M:      outUSD,
		CacheInputUSDPer1M:  cacheInUSD,
		CacheOutputUSDPer1M: cacheOutUSD,
	}, true, ""
}

func parseDecimalFieldOptional(m map[string]any, key string) (decimal.Decimal, error) {
	if m == nil {
		return decimal.Zero, nil
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return decimal.Zero, nil
	}
	d, ok := parseDecimalAny(raw)
	if !ok {
		return decimal.Zero, fmt.Errorf("invalid")
	}
	return d, nil
}

func parseDecimalAny(v any) (decimal.Decimal, bool) {
	switch vv := v.(type) {
	case json.Number:
		d, err := decimal.NewFromString(vv.String())
		if err != nil {
			return decimal.Zero, false
		}
		return d, true
	case float64:
		if vv != vv || vv > 1e100 || vv < -1e100 {
			return decimal.Zero, false
		}
		return decimal.NewFromFloat(vv), true
	case int:
		return decimal.NewFromInt(int64(vv)), true
	case int64:
		return decimal.NewFromInt(vv), true
	case string:
		s := strings.TrimSpace(vv)
		if s == "" {
			return decimal.Zero, false
		}
		d, err := decimal.NewFromString(s)
		if err != nil {
			return decimal.Zero, false
		}
		return d, true
	default:
		return decimal.Zero, false
	}
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	if m == nil {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func pickString(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch vv := v.(type) {
		case string:
			s := strings.TrimSpace(vv)
			if s != "" {
				return s
			}
		case json.Number:
			s := strings.TrimSpace(vv.String())
			if s != "" {
				return s
			}
		default:
		}
	}
	return ""
}

func summarizeFailedItems(failed map[string]string, maxItems int) string {
	if len(failed) == 0 || maxItems <= 0 {
		return ""
	}
	keys := make([]string, 0, len(failed))
	for k := range failed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > maxItems {
		keys = keys[:maxItems]
	}
	var parts []string
	for _, k := range keys {
		v := strings.TrimSpace(failed[k])
		if v == "" {
			v = "失败"
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", k, v))
	}
	return strings.Join(parts, "，")
}
