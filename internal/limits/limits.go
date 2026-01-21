// Package limits 提供单实例的最小护栏：并发/SSE 连接/上游凭据并发保护。
package limits

import "sync"

type TokenLimits struct {
	maxInflight int
	maxSSE      int

	mu       sync.Mutex
	inflight map[int64]int
	sse      map[int64]int
}

func NewTokenLimits(maxInflight, maxSSE int) *TokenLimits {
	if maxInflight <= 0 {
		maxInflight = 1
	}
	if maxSSE < 0 {
		maxSSE = 0
	}
	return &TokenLimits{
		maxInflight: maxInflight,
		maxSSE:      maxSSE,
		inflight:    make(map[int64]int),
		sse:         make(map[int64]int),
	}
}

func (l *TokenLimits) AcquireInflight(tokenID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[tokenID] >= l.maxInflight {
		return false
	}
	l.inflight[tokenID]++
	return true
}

func (l *TokenLimits) ReleaseInflight(tokenID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[tokenID] > 0 {
		l.inflight[tokenID]--
	}
	if l.inflight[tokenID] == 0 {
		delete(l.inflight, tokenID)
	}
}

func (l *TokenLimits) AcquireSSE(tokenID int64) bool {
	if l.maxSSE == 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sse[tokenID] >= l.maxSSE {
		return false
	}
	l.sse[tokenID]++
	return true
}

func (l *TokenLimits) ReleaseSSE(tokenID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sse[tokenID] > 0 {
		l.sse[tokenID]--
	}
	if l.sse[tokenID] == 0 {
		delete(l.sse, tokenID)
	}
}

type CredentialLimits struct {
	maxInflight int

	mu       sync.Mutex
	inflight map[string]int
}

func NewCredentialLimits(maxInflight int) *CredentialLimits {
	if maxInflight <= 0 {
		maxInflight = 1
	}
	return &CredentialLimits{
		maxInflight: maxInflight,
		inflight:    make(map[string]int),
	}
}

func (l *CredentialLimits) Acquire(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[key] >= l.maxInflight {
		return false
	}
	l.inflight[key]++
	return true
}

func (l *CredentialLimits) Release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[key] > 0 {
		l.inflight[key]--
	}
	if l.inflight[key] == 0 {
		delete(l.inflight, key)
	}
}

type ChannelLimits struct {
	mu       sync.Mutex
	inflight map[int64]int
}

func NewChannelLimits() *ChannelLimits {
	return &ChannelLimits{
		inflight: make(map[int64]int),
	}
}

func (l *ChannelLimits) Acquire(channelID int64, limit *int) bool {
	if channelID == 0 {
		return true
	}
	if limit == nil || *limit <= 0 {
		return true
	}
	max := *limit
	if max <= 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[channelID] >= max {
		return false
	}
	l.inflight[channelID]++
	return true
}

func (l *ChannelLimits) Release(channelID int64) {
	if channelID == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[channelID] > 0 {
		l.inflight[channelID]--
	}
	if l.inflight[channelID] == 0 {
		delete(l.inflight, channelID)
	}
}
