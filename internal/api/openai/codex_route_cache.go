package openai

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"realms/internal/scheduler"
)

type codexStickyRouteKeyHashCtxKey struct{}

func withCodexStickyRouteKeyHash(ctx context.Context, stickyRouteKeyHash string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, codexStickyRouteKeyHashCtxKey{}, stickyRouteKeyHash)
}

func getCodexStickyRouteKeyHash(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(codexStickyRouteKeyHashCtxKey{}).(string)
	return v
}

type codexLastSuccessRoute struct {
	channelID      int64
	credentialKey  string
	expiresAt      time.Time
}

func (r codexLastSuccessRoute) differs(sel scheduler.Selection) bool {
	if r.channelID <= 0 || strings.TrimSpace(r.credentialKey) == "" {
		return false
	}
	return r.channelID != sel.ChannelID || r.credentialKey != sel.CredentialKey()
}

type codexSessionRouteCache struct {
	mu   sync.Mutex
	data map[string]codexLastSuccessRoute
}

func newCodexSessionRouteCache() *codexSessionRouteCache {
	return &codexSessionRouteCache{data: make(map[string]codexLastSuccessRoute)}
}

func (c *codexSessionRouteCache) Get(key string, now time.Time) (codexLastSuccessRoute, bool) {
	if c == nil || strings.TrimSpace(key) == "" {
		return codexLastSuccessRoute{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.data {
		if now.After(v.expiresAt) {
			delete(c.data, k)
		}
	}

	v, ok := c.data[key]
	if !ok {
		return codexLastSuccessRoute{}, false
	}
	if now.After(v.expiresAt) {
		delete(c.data, key)
		return codexLastSuccessRoute{}, false
	}
	return v, true
}

func (c *codexSessionRouteCache) Set(key string, sel scheduler.Selection, expiresAt time.Time) {
	if c == nil || strings.TrimSpace(key) == "" {
		return
	}
	if sel.ChannelID <= 0 {
		return
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(codexSessionTTL())
	}

	credKey := sel.CredentialKey()
	if strings.TrimSpace(credKey) == "" {
		return
	}

	c.mu.Lock()
	c.data[key] = codexLastSuccessRoute{
		channelID:     sel.ChannelID,
		credentialKey: credKey,
		expiresAt:     expiresAt,
	}
	c.mu.Unlock()
}

func (h *Handler) getCodexLastSuccessRoute(stickyRouteKeyHash string, now time.Time) (codexLastSuccessRoute, bool) {
	if h == nil || h.codexRouteCache == nil {
		return codexLastSuccessRoute{}, false
	}
	return h.codexRouteCache.Get(stickyRouteKeyHash, now)
}

func (h *Handler) rememberCodexLastSuccessRoute(r *http.Request, sel scheduler.Selection) {
	if h == nil || h.codexRouteCache == nil || r == nil {
		return
	}
	key := strings.TrimSpace(getCodexStickyRouteKeyHash(r.Context()))
	if key == "" {
		return
	}
	h.codexRouteCache.Set(key, sel, time.Now().Add(codexSessionTTL()))
}
