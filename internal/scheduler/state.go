// Package scheduler 管理单实例运行态：亲和/会话绑定/RPM/冷却/失败统计（默认仅内存）。
package scheduler

import (
	"sort"
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
	bStats   RuntimeBindingStats

	rpm map[string][]time.Time

	tokens map[string][]tokenEvent

	credentialSessions map[string]int
	lastBindingSweep   time.Time

	credentialCooldown map[string]time.Time

	channelFails map[int64]int
	credFails    map[string]int

	channelBanUntil  map[int64]time.Time
	channelBanStreak map[int64]int

	channelProbeDueAt      map[int64]time.Time
	channelProbeClaimUntil map[int64]time.Time

	channelPointerID      int64
	channelPointerRing    []int64
	channelPointerIndex   map[int64]int
	channelPointerMovedAt time.Time
	channelPointerReason  string
	channelPointerPinned  bool

	channelPointerChangeHook func(ChannelPointerSnapshot)
}

type ChannelPointerSnapshot struct {
	ChannelID int64
	MovedAt   time.Time
	Reason    string
	Pinned    bool
}

func NewState() *State {
	return &State{
		binding:                make(map[string]bindingEntry),
		affinity:               make(map[string]affinityEntry),
		rpm:                    make(map[string][]time.Time),
		tokens:                 make(map[string][]tokenEvent),
		credentialSessions:     make(map[string]int),
		credentialCooldown:     make(map[string]time.Time),
		channelFails:           make(map[int64]int),
		credFails:              make(map[string]int),
		channelBanUntil:        make(map[int64]time.Time),
		channelBanStreak:       make(map[int64]int),
		channelProbeDueAt:      make(map[int64]time.Time),
		channelProbeClaimUntil: make(map[int64]time.Time),
		channelPointerIndex:    make(map[int64]int),
	}
}

func (s *State) SetChannelPointerChangeHook(h func(ChannelPointerSnapshot)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelPointerChangeHook = h
}

func (s *State) channelPointerSnapshotLocked() ChannelPointerSnapshot {
	return ChannelPointerSnapshot{
		ChannelID: s.channelPointerID,
		MovedAt:   s.channelPointerMovedAt,
		Reason:    s.channelPointerReason,
		Pinned:    s.channelPointerPinned,
	}
}

func (s *State) ChannelPointerSnapshot() ChannelPointerSnapshot {
	if s == nil {
		return ChannelPointerSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channelPointerSnapshotLocked()
}

func (s *State) SetChannelPointer(channelID int64) {
	if s == nil {
		return
	}
	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot

	s.mu.Lock()
	oldID := s.channelPointerID
	oldPinned := s.channelPointerPinned
	now := time.Now()
	if channelID <= 0 {
		if s.channelPointerID != 0 || s.channelPointerPinned {
			s.channelPointerMovedAt = now
			s.channelPointerReason = "clear"
		}
		s.channelPointerID = 0
		s.channelPointerPinned = false
	} else {
		s.channelPointerID = channelID
		s.channelPointerPinned = true
		// 若 ring 已存在但不包含该 channel，则把它追加进 ring，避免后续读取时被判定为 invalid 而自动回退。
		if len(s.channelPointerRing) > 0 {
			if _, ok := s.channelPointerIndex[s.channelPointerID]; !ok {
				s.channelPointerIndex[s.channelPointerID] = len(s.channelPointerRing)
				s.channelPointerRing = append(s.channelPointerRing, s.channelPointerID)
			}
		}
		s.channelPointerMovedAt = now
		s.channelPointerReason = "manual"
	}

	if oldID != s.channelPointerID || oldPinned != s.channelPointerPinned {
		hook = s.channelPointerChangeHook
		snap = s.channelPointerSnapshotLocked()
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
}

func (s *State) TouchChannelPointer(channelID int64, reason string) {
	if s == nil || channelID <= 0 {
		return
	}
	if reason == "" {
		reason = "route"
	}

	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot

	s.mu.Lock()
	oldID := s.channelPointerID
	now := time.Now()
	s.channelPointerID = channelID
	if len(s.channelPointerRing) > 0 {
		if _, ok := s.channelPointerIndex[s.channelPointerID]; !ok {
			s.channelPointerIndex[s.channelPointerID] = len(s.channelPointerRing)
			s.channelPointerRing = append(s.channelPointerRing, s.channelPointerID)
		}
	}
	s.channelPointerMovedAt = now
	s.channelPointerReason = reason

	if oldID != s.channelPointerID {
		hook = s.channelPointerChangeHook
		snap = s.channelPointerSnapshotLocked()
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
}

func (s *State) IsChannelPointerPinned() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channelPointerPinned
}

func (s *State) ChannelPointer(now time.Time) (int64, bool) {
	if s == nil {
		return 0, false
	}

	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot

	s.mu.Lock()
	id, _, _, ok, changed := s.channelPointerLocked(now)
	if changed {
		hook = s.channelPointerChangeHook
		snap = s.channelPointerSnapshotLocked()
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
	return id, ok
}

func (s *State) ChannelPointerInfo(now time.Time) (int64, time.Time, string, bool) {
	if s == nil {
		return 0, time.Time{}, "", false
	}

	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot

	s.mu.Lock()
	id, movedAt, reason, ok, changed := s.channelPointerLocked(now)
	if changed {
		hook = s.channelPointerChangeHook
		snap = s.channelPointerSnapshotLocked()
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
	return id, movedAt, reason, ok
}

func (s *State) channelPointerLocked(now time.Time) (int64, time.Time, string, bool, bool) {
	originalID := s.channelPointerID

	if len(s.channelPointerRing) > 0 {
		if _, ok := s.channelPointerIndex[s.channelPointerID]; !ok {
			s.channelPointerID = s.channelPointerRing[0]
			s.channelPointerMovedAt = now
			s.channelPointerReason = "invalid"
		}
		for i := 0; i < len(s.channelPointerRing); i++ {
			if s.channelPointerID <= 0 {
				break
			}
			until, ok := s.channelBanUntil[s.channelPointerID]
			if ok && now.Before(until) {
				s.advanceChannelPointerLocked(now, "ban")
				continue
			}
			break
		}
	}

	if s.channelPointerID <= 0 {
		return 0, time.Time{}, "", false, originalID != s.channelPointerID
	}
	return s.channelPointerID, s.channelPointerMovedAt, s.channelPointerReason, true, originalID != s.channelPointerID
}

func (s *State) ClearChannelPointer() {
	if s == nil {
		return
	}

	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot

	s.mu.Lock()
	oldID := s.channelPointerID
	oldPinned := s.channelPointerPinned
	if s.channelPointerID != 0 || s.channelPointerPinned {
		now := time.Now()
		s.channelPointerMovedAt = now
		s.channelPointerReason = "clear"
	}
	s.channelPointerID = 0
	s.channelPointerPinned = false

	if oldID != s.channelPointerID || oldPinned != s.channelPointerPinned {
		hook = s.channelPointerChangeHook
		snap = s.channelPointerSnapshotLocked()
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
}

func (s *State) ApplyChannelPointerSnapshot(snap ChannelPointerSnapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.channelPointerID = snap.ChannelID
	s.channelPointerPinned = snap.Pinned
	s.channelPointerMovedAt = snap.MovedAt
	s.channelPointerReason = snap.Reason

	if s.channelPointerID > 0 && len(s.channelPointerRing) > 0 {
		if _, ok := s.channelPointerIndex[s.channelPointerID]; !ok {
			s.channelPointerIndex[s.channelPointerID] = len(s.channelPointerRing)
			s.channelPointerRing = append(s.channelPointerRing, s.channelPointerID)
		}
	}
}

func (s *State) SetChannelPointerRing(ring []int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.channelPointerRing = append(s.channelPointerRing[:0], ring...)
	clear(s.channelPointerIndex)
	for i, id := range s.channelPointerRing {
		if id <= 0 {
			continue
		}
		if _, ok := s.channelPointerIndex[id]; ok {
			continue
		}
		s.channelPointerIndex[id] = i
	}
	// 指针可能指向“非 default ring” 的 channel（例如手动 pin 到某个不在默认组的 channel）。
	// 为保证指针实时生效，这里确保 ring 中至少包含当前指针 channel。
	if s.channelPointerID > 0 {
		if _, ok := s.channelPointerIndex[s.channelPointerID]; !ok {
			s.channelPointerIndex[s.channelPointerID] = len(s.channelPointerRing)
			s.channelPointerRing = append(s.channelPointerRing, s.channelPointerID)
		}
	}
}

func (s *State) ChannelPointerRing() []int64 {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.channelPointerRing) == 0 {
		return nil
	}
	return append([]int64(nil), s.channelPointerRing...)
}

func (s *State) advanceChannelPointerLocked(now time.Time, reason string) bool {
	if s.channelPointerID <= 0 || len(s.channelPointerRing) == 0 {
		return false
	}
	startIdx, ok := s.channelPointerIndex[s.channelPointerID]
	if !ok {
		s.channelPointerID = s.channelPointerRing[0]
		s.channelPointerMovedAt = now
		s.channelPointerReason = "invalid"
		return true
	}
	for step := 1; step <= len(s.channelPointerRing); step++ {
		idx := (startIdx + step) % len(s.channelPointerRing)
		nextID := s.channelPointerRing[idx]
		if nextID <= 0 {
			continue
		}
		until, ok := s.channelBanUntil[nextID]
		if ok && now.Before(until) {
			continue
		}
		s.channelPointerID = nextID
		s.channelPointerMovedAt = now
		s.channelPointerReason = reason
		return true
	}
	return false
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
		s.bStats.Clears++
		s.bStats.ClearExpired++
		return Selection{}, false
	}
	return e.sel, true
}

func (s *State) HasBinding(userID int64, routeKeyHash string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.bindingKey(userID, routeKeyHash)
	e, ok := s.binding[key]
	if !ok {
		return false
	}
	if now.After(e.expiresAt) {
		s.decCredentialSessionsLocked(e.sel.CredentialKey())
		delete(s.binding, key)
		s.bStats.Clears++
		s.bStats.ClearExpired++
		return false
	}
	return true
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

func (s *State) RecordBindingMemoryHit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.MemoryHits++
}

func (s *State) RecordBindingStoreHit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.StoreHits++
}

func (s *State) RecordBindingMiss() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.Misses++
}

func (s *State) RecordBindingSet(source string, refreshed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.Sets++
	switch source {
	case "select":
		s.bStats.SetBySelect++
	case "touch":
		s.bStats.SetByTouch++
	case "store_restore":
		s.bStats.SetByStoreRestore++
	}
	if refreshed {
		s.bStats.Refreshes++
	}
}

func (s *State) RecordBindingClear(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.Clears++
	switch reason {
	case "manual":
		s.bStats.ClearManual++
	case "ineligible":
		s.bStats.ClearIneligible++
	case "probe_pending":
		s.bStats.ClearProbePending++
	case "parse_error":
		s.bStats.ClearParseError++
	}
}

func (s *State) RecordBindingStoreReadError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.StoreReadErrors++
}

func (s *State) RecordBindingStoreWriteError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.StoreWriteErrors++
}

func (s *State) RecordBindingStoreDeleteError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bStats.StoreDeleteErrors++
}

func (s *State) BindingStatsSnapshot() RuntimeBindingStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bStats
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
		if _, ok := s.channelProbeDueAt[channelID]; !ok {
			s.channelProbeDueAt[channelID] = now
		}
		delete(s.channelProbeClaimUntil, channelID)
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
	delete(s.channelProbeDueAt, channelID)
	delete(s.channelProbeClaimUntil, channelID)
}

func (s *State) BanChannel(channelID int64, now time.Time, base time.Duration) time.Time {
	if channelID == 0 || base <= 0 {
		return now
	}
	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot
	changed := false

	s.mu.Lock()

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
		s.mu.Unlock()
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
	if s.channelPointerID == channelID {
		before := s.channelPointerID
		s.advanceChannelPointerLocked(now, "ban")
		changed = before != s.channelPointerID
		if changed {
			hook = s.channelPointerChangeHook
			snap = s.channelPointerSnapshotLocked()
		}
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
	return newUntil
}

func (s *State) BanChannelImmediate(channelID int64, now time.Time, base time.Duration) time.Time {
	if channelID == 0 || base <= 0 {
		return now
	}
	var hook func(ChannelPointerSnapshot)
	var snap ChannelPointerSnapshot
	changed := false

	s.mu.Lock()

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
	if s.channelPointerID == channelID {
		before := s.channelPointerID
		s.advanceChannelPointerLocked(now, "ban")
		changed = before != s.channelPointerID
		if changed {
			hook = s.channelPointerChangeHook
			snap = s.channelPointerSnapshotLocked()
		}
	}
	s.mu.Unlock()

	if hook != nil {
		hook(snap)
	}
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

func (s *State) ListProbeDueChannels(now time.Time, limit int) []int64 {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	type item struct {
		id int64
		at time.Time
	}
	ready := make([]item, 0, len(s.channelProbeDueAt))
	for channelID, dueAt := range s.channelProbeDueAt {
		if until, ok := s.channelProbeClaimUntil[channelID]; ok {
			if now.After(until) {
				delete(s.channelProbeClaimUntil, channelID)
			} else {
				continue
			}
		}
		ready = append(ready, item{id: channelID, at: dueAt})
	}
	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i].at.Equal(ready[j].at) {
			return ready[i].id < ready[j].id
		}
		return ready[i].at.Before(ready[j].at)
	})
	if len(ready) < limit {
		limit = len(ready)
	}
	out := make([]int64, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, ready[i].id)
	}
	return out
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
