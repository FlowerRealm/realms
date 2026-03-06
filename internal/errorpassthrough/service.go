package errorpassthrough

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"realms/internal/store"
)

const (
	defaultCacheTTL  = 10 * time.Second
	maxBodyMatchSize = 8 << 10
)

// RuleSource 提供错误透传规则数据来源（通常是 store.Store）。
type RuleSource interface {
	ListErrorPassthroughRules(ctx context.Context) ([]store.ErrorPassthroughRule, error)
}

// Service 负责缓存并匹配错误透传规则。
type Service struct {
	source RuleSource
	ttl    time.Duration
	now    func() time.Time

	mu        sync.RWMutex
	expiresAt time.Time
	rules     []compiledRule
}

type compiledRule struct {
	raw store.ErrorPassthroughRule

	errorCodeSet map[int]struct{}
	keywords     []string
	platforms    map[string]struct{}
}

func NewService(source RuleSource, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	return &Service{
		source: source,
		ttl:    ttl,
		now:    time.Now,
	}
}

// Match 返回命中后的响应参数：
// - status: 下游状态码
// - message: 下游错误消息
// - skipMonitoring: 是否建议跳过监控/审计
// - matched: 是否命中规则
func (s *Service) Match(platform string, upstreamStatus int, body []byte) (status int, message string, skipMonitoring bool, matched bool) {
	rules := s.loadRules(context.Background())
	if len(rules) == 0 {
		return 0, "", false, false
	}

	platform = strings.ToLower(strings.TrimSpace(platform))
	bodyLower := ""
	bodyLowerReady := false

	for _, rule := range rules {
		if !rule.raw.Enabled {
			continue
		}
		if !platformMatches(rule, platform) {
			continue
		}
		if !ruleMatches(rule, upstreamStatus, body, &bodyLower, &bodyLowerReady) {
			continue
		}

		respCode := upstreamStatus
		if !rule.raw.PassthroughCode && rule.raw.ResponseCode != nil && *rule.raw.ResponseCode > 0 {
			respCode = *rule.raw.ResponseCode
		}
		if respCode <= 0 {
			respCode = http.StatusBadGateway
		}

		msg := extractUpstreamMessage(body)
		if !rule.raw.PassthroughBody && rule.raw.CustomMessage != nil {
			custom := strings.TrimSpace(*rule.raw.CustomMessage)
			if custom != "" {
				msg = custom
			}
		}
		if msg == "" {
			msg = http.StatusText(respCode)
		}
		if msg == "" {
			msg = "上游请求失败"
		}
		return respCode, msg, rule.raw.SkipMonitoring, true
	}

	return 0, "", false, false
}

func (s *Service) loadRules(ctx context.Context) []compiledRule {
	if s == nil || s.source == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := s.now()

	s.mu.RLock()
	if !s.expiresAt.IsZero() && now.Before(s.expiresAt) {
		out := s.rules
		s.mu.RUnlock()
		return out
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	now = s.now()
	if !s.expiresAt.IsZero() && now.Before(s.expiresAt) {
		return s.rules
	}

	rows, err := s.source.ListErrorPassthroughRules(ctx)
	if err != nil {
		// 保持上一次可用缓存，避免数据库短暂抖动导致行为震荡。
		if len(s.rules) > 0 {
			s.expiresAt = now.Add(time.Second)
			return s.rules
		}
		return nil
	}

	s.rules = compile(rows)
	s.expiresAt = now.Add(s.ttl)
	return s.rules
}

func compile(in []store.ErrorPassthroughRule) []compiledRule {
	if len(in) == 0 {
		return nil
	}
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Priority == in[j].Priority {
			return in[i].ID < in[j].ID
		}
		return in[i].Priority < in[j].Priority
	})

	out := make([]compiledRule, 0, len(in))
	for _, r := range in {
		cr := compiledRule{raw: r}
		if len(r.ErrorCodes) > 0 {
			cr.errorCodeSet = make(map[int]struct{}, len(r.ErrorCodes))
			for _, code := range r.ErrorCodes {
				cr.errorCodeSet[code] = struct{}{}
			}
		}
		if len(r.Keywords) > 0 {
			cr.keywords = make([]string, 0, len(r.Keywords))
			for _, kw := range r.Keywords {
				kw = strings.ToLower(strings.TrimSpace(kw))
				if kw == "" {
					continue
				}
				cr.keywords = append(cr.keywords, kw)
			}
		}
		if len(r.Platforms) > 0 {
			cr.platforms = make(map[string]struct{}, len(r.Platforms))
			for _, p := range r.Platforms {
				p = strings.ToLower(strings.TrimSpace(p))
				if p == "" {
					continue
				}
				cr.platforms[p] = struct{}{}
			}
		}
		out = append(out, cr)
	}
	return out
}

func platformMatches(rule compiledRule, platform string) bool {
	if len(rule.platforms) == 0 {
		return true
	}
	_, ok := rule.platforms[platform]
	return ok
}

func ruleMatches(rule compiledRule, statusCode int, body []byte, bodyLower *string, ready *bool) bool {
	hasCodes := len(rule.errorCodeSet) > 0
	hasKeywords := len(rule.keywords) > 0
	if !hasCodes && !hasKeywords {
		return false
	}

	codeMatched := true
	if hasCodes {
		_, codeMatched = rule.errorCodeSet[statusCode]
	}
	keywordMatched := true
	if hasKeywords {
		keywordMatched = containsKeyword(ensureBodyLower(body, bodyLower, ready), rule.keywords)
	}

	mode := strings.ToLower(strings.TrimSpace(rule.raw.MatchMode))
	if mode == "all" {
		if hasCodes && !codeMatched {
			return false
		}
		if hasKeywords && !keywordMatched {
			return false
		}
		return true
	}
	// 默认 any
	if hasCodes && codeMatched {
		return true
	}
	if hasKeywords && keywordMatched {
		return true
	}
	return false
}

func ensureBodyLower(body []byte, bodyLower *string, ready *bool) string {
	if *ready {
		return *bodyLower
	}
	if len(body) > maxBodyMatchSize {
		body = body[:maxBodyMatchSize]
	}
	*bodyLower = strings.ToLower(string(body))
	*ready = true
	return *bodyLower
}

func containsKeyword(bodyLower string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(bodyLower, kw) {
			return true
		}
	}
	return false
}

func extractUpstreamMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if s := extractMsgFromJSONMap(parsed); s != "" {
			return trimSummary(s)
		}
	}
	return trimSummary(string(body))
}

func extractMsgFromJSONMap(v map[string]any) string {
	if v == nil {
		return ""
	}
	for _, key := range []string{"detail", "message", "error_description"} {
		if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	if errObj, ok := v["error"].(map[string]any); ok {
		for _, key := range []string{"message", "error_description"} {
			if s, ok := errObj[key].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func trimSummary(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}
