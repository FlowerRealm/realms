package store

import (
	"container/list"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	defaultTokenAuthCacheSize      = 50000
	defaultTokenAuthCacheTTLMillis = 2000
)

type tokenAuthCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	ll         list.List
	items      map[[32]byte]*list.Element
}

type tokenAuthCacheEntry struct {
	key             [32]byte
	auth            TokenAuth
	expiresAt       time.Time
	lastUsedWriteAt time.Time
}

func newTokenAuthCacheFromEnv() *tokenAuthCache {
	size := defaultTokenAuthCacheSize
	if v := os.Getenv("REALMS_TOKEN_AUTH_CACHE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			size = n
		}
	}
	ttl := time.Duration(defaultTokenAuthCacheTTLMillis) * time.Millisecond
	if v := os.Getenv("REALMS_TOKEN_AUTH_CACHE_TTL_MILLIS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			ttl = time.Duration(n) * time.Millisecond
		}
	}
	if size <= 0 || ttl <= 0 {
		return nil
	}
	return &tokenAuthCache{
		ttl:        ttl,
		maxEntries: size,
		items:      make(map[[32]byte]*list.Element, size),
	}
}

func tokenHashKey(tokenHash []byte) (key [32]byte, ok bool) {
	if len(tokenHash) != 32 {
		return [32]byte{}, false
	}
	copy(key[:], tokenHash)
	return key, true
}

func cloneTokenAuth(in TokenAuth) TokenAuth {
	out := in
	if len(in.Groups) > 0 {
		out.Groups = append([]string(nil), in.Groups...)
	}
	return out
}

func (c *tokenAuthCache) get(now time.Time, key [32]byte) (auth TokenAuth, lastUsedWriteAt time.Time, ok bool) {
	if c == nil {
		return TokenAuth{}, time.Time{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return TokenAuth{}, time.Time{}, false
	}
	ent := el.Value.(*tokenAuthCacheEntry)
	if now.After(ent.expiresAt) {
		c.ll.Remove(el)
		delete(c.items, key)
		return TokenAuth{}, time.Time{}, false
	}
	c.ll.MoveToFront(el)
	return cloneTokenAuth(ent.auth), ent.lastUsedWriteAt, true
}

func (c *tokenAuthCache) set(now time.Time, key [32]byte, auth TokenAuth, lastUsedWriteAt time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		ent := el.Value.(*tokenAuthCacheEntry)
		ent.auth = cloneTokenAuth(auth)
		ent.expiresAt = now.Add(c.ttl)
		ent.lastUsedWriteAt = lastUsedWriteAt
		c.ll.MoveToFront(el)
		return
	}
	ent := &tokenAuthCacheEntry{
		key:             key,
		auth:            cloneTokenAuth(auth),
		expiresAt:       now.Add(c.ttl),
		lastUsedWriteAt: lastUsedWriteAt,
	}
	el := c.ll.PushFront(ent)
	c.items[key] = el

	for c.maxEntries > 0 && c.ll.Len() > c.maxEntries {
		back := c.ll.Back()
		if back == nil {
			break
		}
		be := back.Value.(*tokenAuthCacheEntry)
		delete(c.items, be.key)
		c.ll.Remove(back)
	}
}

func (c *tokenAuthCache) touchLastUsedWriteAt(key [32]byte, t time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return
	}
	ent := el.Value.(*tokenAuthCacheEntry)
	ent.lastUsedWriteAt = t
}

func (c *tokenAuthCache) purgeKey(key [32]byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return
	}
	c.ll.Remove(el)
	delete(c.items, key)
}

func (c *tokenAuthCache) purgeTokenID(tokenID int64) {
	if c == nil || tokenID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, el := range c.items {
		ent := el.Value.(*tokenAuthCacheEntry)
		if ent.auth.TokenID != tokenID {
			continue
		}
		c.ll.Remove(el)
		delete(c.items, key)
	}
}

func (c *tokenAuthCache) purgeAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.ll.Init()
	clear(c.items)
	c.mu.Unlock()
}
