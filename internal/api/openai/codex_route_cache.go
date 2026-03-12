package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"realms/internal/auth"
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
	channelID     int64
	credentialKey string
	routeGroup    string
	expiresAt     time.Time
}

func (r codexLastSuccessRoute) differs(sel scheduler.Selection) bool {
	if r.channelID <= 0 {
		return false
	}
	if strings.TrimSpace(r.credentialKey) == "" {
		return r.channelID != sel.ChannelID
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

func (c *codexSessionRouteCache) sweepLocked(now time.Time) {
	if c == nil || c.sweepEvery <= 0 {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	if !c.lastSweepAt.IsZero() && now.Sub(c.lastSweepAt) < c.sweepEvery {
		return
	}
	for k, v := range c.data {
		if !v.expiresAt.IsZero() && now.After(v.expiresAt) {
			delete(c.data, k)
		}
	}
	c.lastSweepAt = now
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

	c.sweepLocked(now)

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

	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweepLocked(time.Now())
	c.data[key] = codexLastSuccessRoute{
		channelID:     sel.ChannelID,
		credentialKey: credKey,
		routeGroup:    strings.TrimSpace(sel.RouteGroup),
		expiresAt:     expiresAt,
	}
}

func (c *codexSessionRouteCache) SetRoute(key string, route codexLastSuccessRoute) {
	if c == nil || strings.TrimSpace(key) == "" {
		return
	}
	if route.channelID <= 0 || route.expiresAt.IsZero() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweepLocked(time.Now())
	c.data[key] = route
}

func (c *codexSessionRouteCache) Delete(key string) {
	if c == nil || strings.TrimSpace(key) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

func codexStickyBindingKey(userID int64, stickyRouteKeyHash string) string {
	if userID <= 0 {
		return ""
	}
	stickyRouteKeyHash = strings.TrimSpace(stickyRouteKeyHash)
	if stickyRouteKeyHash == "" {
		return ""
	}
	return fmt.Sprintf("%d|%s", userID, stickyRouteKeyHash)
}

func (h *Handler) getCodexLastSuccessRoute(userID int64, stickyRouteKeyHash string, now time.Time) (codexLastSuccessRoute, bool) {
	if h == nil || h.codexRouteCache == nil {
		return codexLastSuccessRoute{}, false
	}
	key := codexStickyBindingKey(userID, stickyRouteKeyHash)
	if key == "" {
		return codexLastSuccessRoute{}, false
	}
	return h.codexRouteCache.Get(key, now)
}

func (h *Handler) rememberCodexLastSuccessRoute(r *http.Request, sel scheduler.Selection) {
	if h == nil || h.codexRouteCache == nil || r == nil {
		return
	}
	stickyHash := strings.TrimSpace(getCodexStickyRouteKeyHash(r.Context()))
	if stickyHash == "" {
		return
	}
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID <= 0 {
		return
	}
	key := codexStickyBindingKey(p.UserID, stickyHash)
	if key == "" {
		return
	}
	h.codexRouteCache.Set(key, sel, time.Now().Add(codexSessionTTL()))
	h.upsertCodexStickyBindingBestEffort(r.Context(), p.UserID, stickyHash, sel)
}

func (h *Handler) clearCodexLastSuccessRoute(userID int64, stickyRouteKeyHash string) {
	if h == nil || h.codexRouteCache == nil {
		return
	}
	key := codexStickyBindingKey(userID, stickyRouteKeyHash)
	if key == "" {
		return
	}
	h.codexRouteCache.Delete(key)
}
