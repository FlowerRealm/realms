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

	lastSweepAt time.Time
	sweepEvery  time.Duration
}

func newCodexSessionRouteCache() *codexSessionRouteCache {
	return &codexSessionRouteCache{
		data:       make(map[string]codexLastSuccessRoute),
		sweepEvery: 1 * time.Minute,
	}
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

	if c.sweepEvery > 0 && (c.lastSweepAt.IsZero() || now.Sub(c.lastSweepAt) >= c.sweepEvery) {
		for k, v := range c.data {
			if !v.expiresAt.IsZero() && now.After(v.expiresAt) {
				delete(c.data, k)
			}
		}
		c.lastSweepAt = now
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
	if now := time.Now(); c.sweepEvery > 0 && (c.lastSweepAt.IsZero() || now.Sub(c.lastSweepAt) >= c.sweepEvery) {
		for k, v := range c.data {
			if !v.expiresAt.IsZero() && now.After(v.expiresAt) {
				delete(c.data, k)
			}
		}
		c.lastSweepAt = now
	}
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
