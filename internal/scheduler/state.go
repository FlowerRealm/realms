// Package scheduler 管理单实例运行态：亲和/RPM/冷却/失败统计（默认仅内存）。
package scheduler

import (
	"sync"
	"time"
)

type affinityEntry struct {
	channelID int64
	expiresAt time.Time
}

type tokenEvent struct {
	time   time.Time
	tokens int
}

type State struct {
	mu sync.Mutex

	affinity map[string]affinityEntry

	rpm map[string][]time.Time

	tokens map[string][]tokenEvent

	credentialCooldown map[string]time.Time
	endpointCooldown   map[int64]time.Time

	endpointFails map[int64]int
	channelFails  map[int64]int
	credFails     map[string]int

	channelBanUntil  map[int64]time.Time
	channelBanStreak map[int64]int

	channelProbeDueAt      map[int64]time.Time
	channelProbeClaimUntil map[int64]time.Time
}

func NewState() *State {
	return &State{
		affinity:               make(map[string]affinityEntry),
		rpm:                    make(map[string][]time.Time),
		tokens:                 make(map[string][]tokenEvent),
		credentialCooldown:     make(map[string]time.Time),
		endpointCooldown:       make(map[int64]time.Time),
		endpointFails:          make(map[int64]int),
		channelFails:           make(map[int64]int),
		credFails:              make(map[string]int),
		channelBanUntil:        make(map[int64]time.Time),
		channelBanStreak:       make(map[int64]int),
		channelProbeDueAt:      make(map[int64]time.Time),
		channelProbeClaimUntil: make(map[int64]time.Time),
	}
}

func (s *State) affinityKey(userID int64) string {
	return itoa64(userID)
}

func (s *State) SetAffinity(userID, channelID int64, expiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.affinity[s.affinityKey(userID)] = affinityEntry{channelID: channelID, expiresAt: expiresAt}
}

func (s *State) GetAffinity(userID int64, now time.Time) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.affinityKey(userID)
	e, ok := s.affinity[key]
	if !ok {
		return 0, false
	}
	if now.After(e.expiresAt) {
		delete(s.affinity, key)
		return 0, false
	}
	return e.channelID, true
}

func (s *State) RecordRPM(credentialKey string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rpm[credentialKey] = append(s.rpm[credentialKey], t)
}

func (s *State) RPM(credentialKey string, now time.Time, window time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.rpm[credentialKey]
	if len(events) == 0 {
		return 0
	}
	cutoff := now.Add(-window)
	var kept []time.Time
	for _, t := range events {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.rpm[credentialKey] = kept
	return len(kept)
}

func (s *State) RecordTokens(credentialKey string, t time.Time, tokens int) {
	if credentialKey == "" || tokens <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[credentialKey] = append(s.tokens[credentialKey], tokenEvent{time: t, tokens: tokens})
}

func (s *State) SetCredentialCooling(credentialKey string, until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentialCooldown[credentialKey] = until
}

func (s *State) IsCredentialCooling(credentialKey string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.credentialCooldown[credentialKey]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(s.credentialCooldown, credentialKey)
		return false
	}
	return true
}

func (s *State) SetEndpointCooling(endpointID int64, until time.Time) {
	if endpointID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpointCooldown[endpointID] = until
}

func (s *State) IsEndpointCooling(endpointID int64, now time.Time) bool {
	if endpointID == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.endpointCooldown[endpointID]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(s.endpointCooldown, endpointID)
		return false
	}
	return true
}

func (s *State) ClearEndpointCooldown(endpointID int64) {
	if endpointID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.endpointCooldown, endpointID)
}

func (s *State) RecordEndpointResult(endpointID int64, success bool) {
	if endpointID == 0 || success {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpointFails[endpointID]++
}

func (s *State) EndpointFailScore(endpointID int64) int {
	if endpointID == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.endpointFails[endpointID]
}

func (s *State) ResetEndpointFailScore(endpointID int64) {
	if endpointID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.endpointFails, endpointID)
}

func (s *State) RecordChannelResult(channelID int64, success bool) {
	if success {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelFails[channelID]++
}

func (s *State) RecordCredentialResult(credentialKey string, success bool) {
	if success {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credFails[credentialKey]++
}

func (s *State) ChannelFailScore(channelID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channelFails[channelID]
}

func (s *State) IsChannelBanned(channelID int64, now time.Time) bool {
	if channelID == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.channelBanUntil[channelID]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(s.channelBanUntil, channelID)
		if _, ok := s.channelProbeDueAt[channelID]; !ok {
			s.channelProbeDueAt[channelID] = now
		}
		delete(s.channelProbeClaimUntil, channelID)
		return false
	}
	return true
}

func (s *State) ChannelBanUntil(channelID int64, now time.Time) (time.Time, bool) {
	if channelID == 0 {
		return time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.channelBanUntil[channelID]
	if !ok {
		return time.Time{}, false
	}
	if now.After(until) {
		delete(s.channelBanUntil, channelID)
		if _, ok := s.channelProbeDueAt[channelID]; !ok {
			s.channelProbeDueAt[channelID] = now
		}
		delete(s.channelProbeClaimUntil, channelID)
		return time.Time{}, false
	}
	return until, true
}

func (s *State) ClearChannelBan(channelID int64) {
	if channelID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channelBanUntil, channelID)
	delete(s.channelBanStreak, channelID)
	delete(s.channelProbeDueAt, channelID)
	delete(s.channelProbeClaimUntil, channelID)
}

func (s *State) BanChannel(channelID int64, now time.Time, base time.Duration) time.Time {
	if channelID == 0 || base <= 0 {
		return now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	streak := s.channelBanStreak[channelID] + 1
	// 溢出保护：避免不受控的指数/线性增长导致 duration 过大。
	if streak > 20 {
		streak = 20
	}
	s.channelBanStreak[channelID] = streak

	// 避免“单次可重试失败”就把整个渠道 ban 掉：优先让同渠道的其他 credential 有机会接管。
	// 连续失败达到阈值后才进入 ban。
	if streak < 2 {
		delete(s.channelBanUntil, channelID)
		return now
	}

	start := now
	if until, ok := s.channelBanUntil[channelID]; ok && until.After(now) {
		start = until
	}
	inc := base * time.Duration(streak)
	newUntil := start.Add(inc)
	if newUntil.Before(start) {
		newUntil = start.Add(24 * time.Hour)
	}
	maxUntil := now.Add(10 * time.Minute)
	if newUntil.After(maxUntil) {
		newUntil = maxUntil
	}
	s.channelBanUntil[channelID] = newUntil
	return newUntil
}

func (s *State) BanChannelImmediate(channelID int64, now time.Time, base time.Duration) time.Time {
	if channelID == 0 || base <= 0 {
		return now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	streak := s.channelBanStreak[channelID] + 1
	// 溢出保护：避免不受控的指数/线性增长导致 duration 过大。
	if streak > 20 {
		streak = 20
	}
	s.channelBanStreak[channelID] = streak

	start := now
	if until, ok := s.channelBanUntil[channelID]; ok && until.After(now) {
		start = until
	}
	inc := base * time.Duration(streak)
	newUntil := start.Add(inc)
	if newUntil.Before(start) {
		newUntil = start.Add(24 * time.Hour)
	}
	maxUntil := now.Add(10 * time.Minute)
	if newUntil.After(maxUntil) {
		newUntil = maxUntil
	}
	s.channelBanUntil[channelID] = newUntil
	return newUntil
}

func (s *State) IsChannelProbePending(channelID int64, now time.Time) bool {
	if s == nil || channelID == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channelProbeDueAt[channelID]; !ok {
		return false
	}
	if until, ok := s.channelProbeClaimUntil[channelID]; ok {
		if now.After(until) {
			delete(s.channelProbeClaimUntil, channelID)
		} else {
			return false
		}
	}
	return true
}

func (s *State) IsChannelProbeDue(channelID int64) bool {
	if s == nil || channelID == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.channelProbeDueAt[channelID]
	return ok
}

func (s *State) TryClaimChannelProbe(channelID int64, now time.Time, ttl time.Duration) bool {
	if s == nil || channelID == 0 {
		return false
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channelProbeDueAt[channelID]; !ok {
		return false
	}
	if until, ok := s.channelProbeClaimUntil[channelID]; ok {
		if now.After(until) {
			delete(s.channelProbeClaimUntil, channelID)
		} else {
			return false
		}
	}
	until := now.Add(ttl)
	if until.Before(now) {
		until = now.Add(30 * time.Second)
	}
	s.channelProbeClaimUntil[channelID] = until
	return true
}

func (s *State) ReleaseChannelProbeClaim(channelID int64) {
	if s == nil || channelID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channelProbeClaimUntil, channelID)
}

func (s *State) ClearChannelProbe(channelID int64) {
	if s == nil || channelID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channelProbeDueAt, channelID)
	delete(s.channelProbeClaimUntil, channelID)
}

func (s *State) ResetChannelFailScore(channelID int64) {
	if s == nil || channelID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channelFails, channelID)
}

func (s *State) SweepExpiredChannelBans(now time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for channelID, until := range s.channelBanUntil {
		if now.After(until) {
			delete(s.channelBanUntil, channelID)
			if _, ok := s.channelProbeDueAt[channelID]; !ok {
				s.channelProbeDueAt[channelID] = now
			}
			delete(s.channelProbeClaimUntil, channelID)
		}
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [32]byte
	i := len(b)
	x := n
	if x < 0 {
		x = -x
	}
	for x > 0 {
		i--
		b[i] = byte('0' + x%10)
		x /= 10
	}
	if n < 0 {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
