// Package scheduler 管理单实例运行态：亲和/会话绑定/RPM/冷却/失败统计（默认仅内存）。
package scheduler

import (
	"sync"
	"time"
)

type bindingEntry struct {
	sel       Selection
	expiresAt time.Time
}

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

	binding  map[string]bindingEntry
	affinity map[string]affinityEntry

	rpm map[string][]time.Time

	tokens map[string][]tokenEvent

	credentialSessions map[string]int
	lastBindingSweep   time.Time

	credentialCooldown map[string]time.Time

	channelFails map[int64]int
	credFails    map[string]int

	channelBanUntil  map[int64]time.Time
	channelBanStreak map[int64]int

	forcedChannelID    int64
	forcedChannelUntil time.Time

	lastSuccessSel Selection
	lastSuccessAt  time.Time
}

func NewState() *State {
	return &State{
		binding:            make(map[string]bindingEntry),
		affinity:           make(map[string]affinityEntry),
		rpm:                make(map[string][]time.Time),
		tokens:             make(map[string][]tokenEvent),
		credentialSessions: make(map[string]int),
		credentialCooldown: make(map[string]time.Time),
		channelFails:       make(map[int64]int),
		credFails:          make(map[string]int),
		channelBanUntil:    make(map[int64]time.Time),
		channelBanStreak:   make(map[int64]int),
	}
}

func (s *State) SetForcedChannel(channelID int64, until time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if channelID <= 0 || until.IsZero() || time.Now().After(until) {
		s.forcedChannelID = 0
		s.forcedChannelUntil = time.Time{}
		return
	}
	s.forcedChannelID = channelID
	s.forcedChannelUntil = until
}

func (s *State) ForcedChannel(now time.Time) (int64, time.Time, bool) {
	if s == nil {
		return 0, time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.forcedChannelID <= 0 || s.forcedChannelUntil.IsZero() {
		return 0, time.Time{}, false
	}
	if now.After(s.forcedChannelUntil) {
		s.forcedChannelID = 0
		s.forcedChannelUntil = time.Time{}
		return 0, time.Time{}, false
	}
	return s.forcedChannelID, s.forcedChannelUntil, true
}

func (s *State) ClearForcedChannel() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forcedChannelID = 0
	s.forcedChannelUntil = time.Time{}
}

func (s *State) RecordLastSuccess(sel Selection, at time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSuccessSel = sel
	s.lastSuccessAt = at
}

func (s *State) LastSuccess() (Selection, time.Time, bool) {
	if s == nil {
		return Selection{}, time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastSuccessAt.IsZero() {
		return Selection{}, time.Time{}, false
	}
	return s.lastSuccessSel, s.lastSuccessAt, true
}

func (s *State) bindingKey(userID int64, routeKeyHash string) string {
	return itoa64(userID) + ":" + routeKeyHash
}

func (s *State) affinityKey(userID int64) string {
	return itoa64(userID)
}

func (s *State) GetBinding(userID int64, routeKeyHash string) (Selection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.bindingKey(userID, routeKeyHash)
	e, ok := s.binding[key]
	if !ok {
		return Selection{}, false
	}
	if time.Now().After(e.expiresAt) {
		s.decCredentialSessionsLocked(e.sel.CredentialKey())
		delete(s.binding, key)
		return Selection{}, false
	}
	return e.sel, true
}

func (s *State) SetBinding(userID int64, routeKeyHash string, sel Selection, expiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.bindingKey(userID, routeKeyHash)
	now := time.Now()
	prev, ok := s.binding[key]
	if ok && now.After(prev.expiresAt) {
		s.decCredentialSessionsLocked(prev.sel.CredentialKey())
		ok = false
	}
	if ok {
		prevKey := prev.sel.CredentialKey()
		nextKey := sel.CredentialKey()
		if prevKey != nextKey {
			s.decCredentialSessionsLocked(prevKey)
			s.incCredentialSessionsLocked(nextKey)
		}
	} else {
		s.incCredentialSessionsLocked(sel.CredentialKey())
	}
	s.binding[key] = bindingEntry{sel: sel, expiresAt: expiresAt}
}

func (s *State) ClearBinding(userID int64, routeKeyHash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.bindingKey(userID, routeKeyHash)
	e, ok := s.binding[key]
	if ok && !time.Now().After(e.expiresAt) {
		s.decCredentialSessionsLocked(e.sel.CredentialKey())
	}
	delete(s.binding, key)
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

func (s *State) TPM(credentialKey string, now time.Time, window time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.tokens[credentialKey]
	if len(events) == 0 {
		return 0
	}
	cutoff := now.Add(-window)
	total := 0
	var kept []tokenEvent
	for _, e := range events {
		if e.time.After(cutoff) {
			kept = append(kept, e)
			total += e.tokens
		}
	}
	s.tokens[credentialKey] = kept
	return total
}

func (s *State) CredentialSessions(credentialKey string, now time.Time) int {
	if credentialKey == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maybeSweepBindingsLocked(now)
	return s.credentialSessions[credentialKey]
}

func (s *State) maybeSweepBindingsLocked(now time.Time) {
	if !s.lastBindingSweep.IsZero() && now.Sub(s.lastBindingSweep) < 10*time.Second {
		return
	}
	s.lastBindingSweep = now
	for k, e := range s.binding {
		if now.After(e.expiresAt) {
			s.decCredentialSessionsLocked(e.sel.CredentialKey())
			delete(s.binding, k)
		}
	}
}

func (s *State) incCredentialSessionsLocked(credentialKey string) {
	if credentialKey == "" {
		return
	}
	s.credentialSessions[credentialKey]++
}

func (s *State) decCredentialSessionsLocked(credentialKey string) {
	if credentialKey == "" {
		return
	}
	if s.credentialSessions[credentialKey] > 0 {
		s.credentialSessions[credentialKey]--
	}
	if s.credentialSessions[credentialKey] == 0 {
		delete(s.credentialSessions, credentialKey)
	}
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
		return false
	}
	return true
}

func (s *State) ClearChannelBan(channelID int64) {
	if channelID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channelBanUntil, channelID)
	delete(s.channelBanStreak, channelID)
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
	s.channelBanUntil[channelID] = newUntil
	return newUntil
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
