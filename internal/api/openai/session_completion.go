package openai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"realms/internal/auth"
	"realms/internal/scheduler"
)

const (
	codexSessionTTLDefaultSeconds = 300
	codexSessionIDMaxLen          = 1024
)

type codexAutoSessionEntry struct {
	sessionID string
	expiresAt time.Time
}

var codexAutoSessionCache = struct {
	mu   sync.Mutex
	data map[string]codexAutoSessionEntry
}{
	data: make(map[string]codexAutoSessionEntry),
}

var metadataSessionIDRegex = regexp.MustCompile(`(?i)session_([a-f0-9-]{36})`)

func completeCodexSessionIdentifiers(payload map[string]any, r *http.Request, p auth.Principal) (bool, string, bool) {
	if payload == nil || r == nil || !isCodexResponsesPayload(payload) {
		return false, "", false
	}

	bodyPromptCacheKey := normalizeCodexSessionID(stringFromAny(payload["prompt_cache_key"]))
	bodySessionID := normalizeCodexSessionID(stringFromAny(payload["session_id"]))
	metadataSessionID := normalizeCodexSessionID(extractMetadataSessionID(payload))
	headerSessionID := normalizeCodexSessionID(extractSessionIDFromHeaders(r.Header))

	sessionID := firstNonEmpty(bodyPromptCacheKey, bodySessionID, metadataSessionID, headerSessionID)
	generated := false
	if sessionID == "" {
		sessionID = getOrCreateCodexAutoSessionID(buildCodexSessionFingerprint(payload, r, p))
		generated = true
	}
	if sessionID == "" {
		return false, "", false
	}

	changed := false
	if bodyPromptCacheKey == "" {
		payload["prompt_cache_key"] = sessionID
		changed = true
	}
	if strings.TrimSpace(r.Header.Get("Session_id")) == "" {
		r.Header.Set("Session_id", sessionID)
		changed = true
	}
	if strings.TrimSpace(r.Header.Get("X-Session-Id")) == "" {
		r.Header.Set("X-Session-Id", sessionID)
		changed = true
	}
	return changed, sessionID, generated
}

func (h *Handler) touchBindingFromRouteKey(userID int64, sel scheduler.Selection, routeKey string) {
	if h == nil || h.sched == nil || userID <= 0 {
		return
	}
	normalized := normalizeCodexSessionID(routeKey)
	if normalized == "" {
		return
	}
	h.sched.TouchBinding(userID, h.sched.RouteKeyHash(normalized), sel)
}

func extractRouteKeyFromStructuredData(v any) string {
	return extractRouteKeyFromStructuredDataDepth(v, 8)
}

func extractRouteKeyFromStructuredDataDepth(v any, depth int) string {
	if depth <= 0 || v == nil {
		return ""
	}
	switch vv := v.(type) {
	case map[string]any:
		for _, key := range []string{"prompt_cache_key", "session_id", "conversation_id", "previous_response_id"} {
			if s := normalizeCodexSessionID(stringFromAny(vv[key])); s != "" {
				return s
			}
		}
		for _, child := range vv {
			if s := extractRouteKeyFromStructuredDataDepth(child, depth-1); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range vv {
			if s := extractRouteKeyFromStructuredDataDepth(child, depth-1); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractSessionIDFromMetadataUserID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	metaAny, ok := payload["metadata"]
	if !ok {
		return ""
	}
	meta, ok := metaAny.(map[string]any)
	if !ok {
		return ""
	}
	userID := strings.TrimSpace(stringFromAny(meta["user_id"]))
	if userID == "" {
		return ""
	}
	if match := metadataSessionIDRegex.FindStringSubmatch(userID); len(match) > 1 {
		return strings.ToLower(strings.TrimSpace(match[1]))
	}
	return ""
}

func deriveRouteKeyFromConversationPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if cacheable := extractCacheableConversationText(payload); cacheable != "" {
		return routeKeyHashFromText(cacheable)
	}

	var combined strings.Builder
	if systemText := extractSystemText(payload["system"]); systemText != "" {
		_, _ = combined.WriteString(systemText)
	}
	if messages, ok := payload["messages"].([]any); ok {
		for _, message := range messages {
			if msg, ok := message.(map[string]any); ok {
				if text := extractTextContent(msg["content"]); text != "" {
					_, _ = combined.WriteString(text)
				}
			}
		}
	}
	if inputs, ok := payload["input"].([]any); ok {
		for _, item := range inputs {
			if msg, ok := item.(map[string]any); ok {
				if text := extractTextContent(msg["content"]); text != "" {
					_, _ = combined.WriteString(text)
				}
			}
		}
	}
	if combined.Len() == 0 {
		return ""
	}
	return routeKeyHashFromText(combined.String())
}

func extractCacheableConversationText(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	var systemCacheable strings.Builder
	if parts, ok := payload["system"].([]any); ok {
		for _, part := range parts {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			cc, ok := partMap["cache_control"].(map[string]any)
			if !ok || !strings.EqualFold(stringFromAny(cc["type"]), "ephemeral") {
				continue
			}
			if text := strings.TrimSpace(stringFromAny(partMap["text"])); text != "" {
				_, _ = systemCacheable.WriteString(text)
			}
		}
	}

	for _, field := range []string{"messages", "input"} {
		items, ok := payload[field].([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			msg, ok := item.(map[string]any)
			if !ok {
				continue
			}
			content, ok := msg["content"].([]any)
			if !ok {
				continue
			}
			for _, part := range content {
				partMap, ok := part.(map[string]any)
				if !ok {
					continue
				}
				cc, ok := partMap["cache_control"].(map[string]any)
				if !ok || !strings.EqualFold(stringFromAny(cc["type"]), "ephemeral") {
					continue
				}
				return extractTextContent(msg["content"])
			}
		}
	}

	return systemCacheable.String()
}

func extractSystemText(system any) string {
	switch value := system.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		var parts []string
		for _, item := range value {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text := strings.TrimSpace(stringFromAny(m["text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func extractTextContent(content any) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		var parts []string
		for _, item := range value {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			contentType := strings.ToLower(strings.TrimSpace(stringFromAny(m["type"])))
			if contentType != "" && contentType != "text" && contentType != "input_text" {
				continue
			}
			if text := strings.TrimSpace(stringFromAny(m["text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func routeKeyHashFromText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	// 对齐 sub2api：使用前 16 字节（32 hex）作为会话键。
	return hex.EncodeToString(sum[:16])
}

func isCodexResponsesPayload(payload map[string]any) bool {
	_, ok := payload["input"].([]any)
	return ok
}

func extractMetadataSessionID(payload map[string]any) string {
	metaAny, ok := payload["metadata"]
	if !ok {
		return ""
	}
	meta, ok := metaAny.(map[string]any)
	if !ok {
		return ""
	}
	return stringFromAny(meta["session_id"])
}

func extractSessionIDFromHeaders(h http.Header) string {
	for _, key := range []string{"Session_id", "Session-Id", "X-Session-Id", "session_id", "session-id"} {
		if v := strings.TrimSpace(h.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func buildCodexSessionFingerprint(payload map[string]any, r *http.Request, p auth.Principal) string {
	userID := strconv.FormatInt(p.UserID, 10)
	tokenID := ""
	if p.TokenID != nil {
		tokenID = strconv.FormatInt(*p.TokenID, 10)
	}
	userAgent := strings.TrimSpace(r.Header.Get("User-Agent"))
	ip := clientIPFromHeaders(r.Header)
	model := strings.TrimSpace(stringFromAny(payload["model"]))

	fingerprint := strings.Join([]string{userID, tokenID, userAgent, ip, model}, "|")
	sum := sha256.Sum256([]byte(fingerprint))
	return hex.EncodeToString(sum[:])
}

func clientIPFromHeaders(h http.Header) string {
	if forwarded := strings.TrimSpace(h.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	return strings.TrimSpace(h.Get("X-Real-Ip"))
}

func getOrCreateCodexAutoSessionID(fingerprint string) string {
	now := time.Now()
	ttl := codexSessionTTL()

	codexAutoSessionCache.mu.Lock()
	defer codexAutoSessionCache.mu.Unlock()

	for key, entry := range codexAutoSessionCache.data {
		if now.After(entry.expiresAt) {
			delete(codexAutoSessionCache.data, key)
		}
	}

	if entry, ok := codexAutoSessionCache.data[fingerprint]; ok && now.Before(entry.expiresAt) {
		return entry.sessionID
	}

	sessionID := newCodexAutoSessionID()
	if sessionID == "" {
		return ""
	}
	codexAutoSessionCache.data[fingerprint] = codexAutoSessionEntry{
		sessionID: sessionID,
		expiresAt: now.Add(ttl),
	}
	return sessionID
}

func newCodexAutoSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return "rk_auto_" + hex.EncodeToString(b)
}

func codexSessionTTL() time.Duration {
	seconds := codexSessionTTLDefaultSeconds
	if raw := strings.TrimSpace(os.Getenv("REALMS_CODEX_SESSION_TTL_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			seconds = parsed
		}
	}
	return time.Duration(seconds) * time.Second
}

func normalizeCodexSessionID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > codexSessionIDMaxLen {
		return ""
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
