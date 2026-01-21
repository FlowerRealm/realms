package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type pricingImportParseResult struct {
	items  []store.ManagedModelPricingUpsert
	failed map[string]string
}

func (s *Server) ImportModelPricing(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "表单解析失败")
				return
			}
			http.Error(w, "表单解析失败", http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "表单解析失败")
				return
			}
			http.Error(w, "表单解析失败", http.StatusBadRequest)
			return
		}
	}

	returnTo := safeAdminReturnTo(r.FormValue("return_to"), "/admin/models")
	raw, err := readPricingImportPayload(r, 4<<20)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	parsed, err := parsePricingImportJSON(raw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "JSON 解析失败："+err.Error())
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("JSON 解析失败："+err.Error()), http.StatusFound)
		return
	}

	upsertRes, err := s.st.UpsertManagedModelPricing(r.Context(), parsed.items)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "导入失败")
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("导入失败"), http.StatusFound)
		return
	}

	notice := fmt.Sprintf("导入完成：新增 %d，更新 %d，无变化 %d，失败 %d。", len(upsertRes.Added), len(upsertRes.Updated), len(upsertRes.Unchanged), len(parsed.failed))
	if len(parsed.failed) > 0 {
		notice = notice + " 失败示例：" + summarizeFailedItems(parsed.failed, 3)
	}

	if isAjax(r) {
		ajaxOK(w, notice)
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape(notice), http.StatusFound)
}

func readPricingImportPayload(r *http.Request, maxBytes int64) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("请求无效")
	}

	// 文件优先。
	if r.MultipartForm != nil {
		f, _, err := r.FormFile("pricing_file")
		if err == nil && f != nil {
			defer f.Close()
			b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
			if err != nil {
				return nil, fmt.Errorf("读取文件失败")
			}
			if int64(len(b)) > maxBytes {
				return nil, fmt.Errorf("JSON 文件过大")
			}
			if len(bytes.TrimSpace(b)) == 0 {
				return nil, fmt.Errorf("JSON 文件为空")
			}
			return b, nil
		}
	}

	raw := strings.TrimSpace(r.FormValue("pricing_json"))
	if raw == "" {
		return nil, fmt.Errorf("请上传 JSON 文件或粘贴 JSON 内容")
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("JSON 内容过大")
	}
	return []byte(raw), nil
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
	hasUSD := hasAnyKey(m, "input_usd_per_1m", "output_usd_per_1m", "cache_usd_per_1m")
	hasToken := hasAnyKey(m, "input_cost_per_token", "output_cost_per_token", "cache_read_input_cost_per_token", "cache_read_cost_per_token")

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
		cacheUSD, err := parseDecimalFieldOptional(m, "cache_usd_per_1m")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "cache_usd_per_1m 不合法"
		}
		if inUSD.IsNegative() || outUSD.IsNegative() || cacheUSD.IsNegative() {
			return store.ManagedModelPricingUpsert{}, false, "定价不能为负数"
		}
		return store.ManagedModelPricingUpsert{
			InputUSDPer1M:  inUSD,
			OutputUSDPer1M: outUSD,
			CacheUSDPer1M:  cacheUSD,
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

	cacheTok, err := parseDecimalFieldOptional(m, "cache_read_input_cost_per_token")
	if err != nil {
		return store.ManagedModelPricingUpsert{}, false, "cache_read_input_cost_per_token 不合法"
	}
	if cacheTok.IsZero() {
		cacheTok, err = parseDecimalFieldOptional(m, "cache_read_cost_per_token")
		if err != nil {
			return store.ManagedModelPricingUpsert{}, false, "cache_read_cost_per_token 不合法"
		}
	}

	million := decimal.NewFromInt(1_000_000)
	inUSD := inTok.Mul(million)
	outUSD := outTok.Mul(million)
	cacheUSD := cacheTok.Mul(million)
	if inUSD.IsNegative() || outUSD.IsNegative() || cacheUSD.IsNegative() {
		return store.ManagedModelPricingUpsert{}, false, "定价不能为负数"
	}
	return store.ManagedModelPricingUpsert{
		InputUSDPer1M:  inUSD,
		OutputUSDPer1M: outUSD,
		CacheUSDPer1M:  cacheUSD,
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

