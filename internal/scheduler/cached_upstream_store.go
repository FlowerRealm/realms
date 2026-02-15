package scheduler

import (
	"context"
	"sync"
	"time"

	"realms/internal/store"
)

type cachedUpstreamStore struct {
	inner UpstreamStore
	ttl   time.Duration

	mu sync.Mutex

	channels            cacheEntry[store.UpstreamChannel]
	endpointsByChannel  map[int64]cacheEntry[store.UpstreamEndpoint]
	openAICredsByEP     map[int64]cacheEntry[store.OpenAICompatibleCredential]
	anthropicCredsByEP  map[int64]cacheEntry[store.AnthropicCredential]
	codexAccountsByEP   map[int64]cacheEntry[store.CodexOAuthAccount]
}

type cacheEntry[T any] struct {
	expiresAt time.Time
	value     []T
}

func NewCachedUpstreamStore(inner UpstreamStore, ttl time.Duration) UpstreamStore {
	if inner == nil {
		return inner
	}
	if ttl <= 0 {
		return inner
	}
	return &cachedUpstreamStore{
		inner:             inner,
		ttl:               ttl,
		endpointsByChannel: make(map[int64]cacheEntry[store.UpstreamEndpoint]),
		openAICredsByEP:    make(map[int64]cacheEntry[store.OpenAICompatibleCredential]),
		anthropicCredsByEP: make(map[int64]cacheEntry[store.AnthropicCredential]),
		codexAccountsByEP:  make(map[int64]cacheEntry[store.CodexOAuthAccount]),
	}
}

func (c *cachedUpstreamStore) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.channels = cacheEntry[store.UpstreamChannel]{}
	clear(c.endpointsByChannel)
	clear(c.openAICredsByEP)
	clear(c.anthropicCredsByEP)
	clear(c.codexAccountsByEP)
	c.mu.Unlock()
}

func (c *cachedUpstreamStore) ListUpstreamChannels(ctx context.Context) ([]store.UpstreamChannel, error) {
	now := time.Now()
	c.mu.Lock()
	if !c.channels.expiresAt.IsZero() && now.Before(c.channels.expiresAt) && c.channels.value != nil {
		out := append([]store.UpstreamChannel(nil), c.channels.value...)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	rows, err := c.inner.ListUpstreamChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := append([]store.UpstreamChannel(nil), rows...)

	c.mu.Lock()
	c.channels = cacheEntry[store.UpstreamChannel]{expiresAt: now.Add(c.ttl), value: out}
	c.mu.Unlock()
	return append([]store.UpstreamChannel(nil), out...), nil
}

func (c *cachedUpstreamStore) ListUpstreamEndpointsByChannel(ctx context.Context, channelID int64) ([]store.UpstreamEndpoint, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.endpointsByChannel[channelID]; ok {
		if !entry.expiresAt.IsZero() && now.Before(entry.expiresAt) && entry.value != nil {
			out := append([]store.UpstreamEndpoint(nil), entry.value...)
			c.mu.Unlock()
			return out, nil
		}
	}
	c.mu.Unlock()

	rows, err := c.inner.ListUpstreamEndpointsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := append([]store.UpstreamEndpoint(nil), rows...)

	c.mu.Lock()
	c.endpointsByChannel[channelID] = cacheEntry[store.UpstreamEndpoint]{expiresAt: now.Add(c.ttl), value: out}
	c.mu.Unlock()
	return append([]store.UpstreamEndpoint(nil), out...), nil
}

func (c *cachedUpstreamStore) ListOpenAICompatibleCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.openAICredsByEP[endpointID]; ok {
		if !entry.expiresAt.IsZero() && now.Before(entry.expiresAt) && entry.value != nil {
			out := append([]store.OpenAICompatibleCredential(nil), entry.value...)
			c.mu.Unlock()
			return out, nil
		}
	}
	c.mu.Unlock()

	rows, err := c.inner.ListOpenAICompatibleCredentialsByEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	out := append([]store.OpenAICompatibleCredential(nil), rows...)

	c.mu.Lock()
	c.openAICredsByEP[endpointID] = cacheEntry[store.OpenAICompatibleCredential]{expiresAt: now.Add(c.ttl), value: out}
	c.mu.Unlock()
	return append([]store.OpenAICompatibleCredential(nil), out...), nil
}

func (c *cachedUpstreamStore) ListAnthropicCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.AnthropicCredential, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.anthropicCredsByEP[endpointID]; ok {
		if !entry.expiresAt.IsZero() && now.Before(entry.expiresAt) && entry.value != nil {
			out := append([]store.AnthropicCredential(nil), entry.value...)
			c.mu.Unlock()
			return out, nil
		}
	}
	c.mu.Unlock()

	rows, err := c.inner.ListAnthropicCredentialsByEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	out := append([]store.AnthropicCredential(nil), rows...)

	c.mu.Lock()
	c.anthropicCredsByEP[endpointID] = cacheEntry[store.AnthropicCredential]{expiresAt: now.Add(c.ttl), value: out}
	c.mu.Unlock()
	return append([]store.AnthropicCredential(nil), out...), nil
}

func (c *cachedUpstreamStore) ListCodexOAuthAccountsByEndpoint(ctx context.Context, endpointID int64) ([]store.CodexOAuthAccount, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.codexAccountsByEP[endpointID]; ok {
		if !entry.expiresAt.IsZero() && now.Before(entry.expiresAt) && entry.value != nil {
			out := append([]store.CodexOAuthAccount(nil), entry.value...)
			c.mu.Unlock()
			return out, nil
		}
	}
	c.mu.Unlock()

	rows, err := c.inner.ListCodexOAuthAccountsByEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	out := append([]store.CodexOAuthAccount(nil), rows...)

	c.mu.Lock()
	c.codexAccountsByEP[endpointID] = cacheEntry[store.CodexOAuthAccount]{expiresAt: now.Add(c.ttl), value: out}
	c.mu.Unlock()
	return append([]store.CodexOAuthAccount(nil), out...), nil
}

