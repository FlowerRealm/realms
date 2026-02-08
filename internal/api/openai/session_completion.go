package openai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
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

func (h *Handler) touchBindingFromPromptCacheKey(userID int64, sel scheduler.Selection, promptCacheKey string) {
	if h == nil || h.sched == nil || userID <= 0 {
		return
	}
	normalized := normalizeCodexSessionID(promptCacheKey)
	if normalized == "" {
		return
	}
	h.sched.TouchBinding(userID, h.sched.RouteKeyHash(normalized), sel)
}

func extractPromptCacheKeyFromRawBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return extractPromptCacheKey(payload)
}

func extractPromptCacheKey(v any) string {
	return extractPromptCacheKeyDepth(v, 8)
}

func extractPromptCacheKeyDepth(v any, depth int) string {
	if depth <= 0 || v == nil {
		return ""
	}
	switch vv := v.(type) {
	case map[string]any:
		if s := normalizeCodexSessionID(stringFromAny(vv["prompt_cache_key"])); s != "" {
			return s
		}
		for _, child := range vv {
			if s := extractPromptCacheKeyDepth(child, depth-1); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range vv {
			if s := extractPromptCacheKeyDepth(child, depth-1); s != "" {
				return s
			}
		}
	}
	return ""
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
