// Package scheduler 实现三层选择：Channel → Endpoint → Credential，并提供最小的亲和/粘性/冷却能力。
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"realms/internal/store"
)

type CredentialType string

const (
	CredentialTypeOpenAI    CredentialType = "openai_compatible"
	CredentialTypeCodex     CredentialType = "codex_oauth"
	CredentialTypeAnthropic CredentialType = "anthropic"
)

type Selection struct {
	ChannelID     int64
	ChannelType   string
	ChannelGroups string
	RouteGroup    string

	AllowServiceTier       bool
	DisableStore           bool
	AllowSafetyIdentifier  bool
	OpenAIOrganization     *string
	AutoBan                bool
	ForceFormat            bool
	ThinkingToContent      bool
	PassThroughBodyEnabled bool
	Proxy                  string
	SystemPrompt           string
	SystemPromptOverride   bool
	CacheTTLPreference     string
	ParamOverride          string
	HeaderOverride         string
	StatusCodeMapping      string
	ModelSuffixPreserve    string
	RequestBodyBlacklist   string
	RequestBodyWhitelist   string

	EndpointID int64
	BaseURL    string

	CredentialType CredentialType
	CredentialID   int64
}

func (s Selection) CredentialKey() string {
	return fmt.Sprintf("%s:%d", s.CredentialType, s.CredentialID)
}

type Result struct {
	Success    bool
	Retriable  bool
	StatusCode int
	ErrorClass string
	// CooldownUntil 用于上层传入精确的冷却截止时间（例如上游返回 resets_at）。
	// 为空时按调度器默认策略计算。
	CooldownUntil *time.Time
}

type Scheduler struct {
	st UpstreamStore

	state         *State
	bindingStore  BindingStore
	groupPointers ChannelGroupPointerStore
	affinityTTL   time.Duration
	bindingTTL    time.Duration
	rpmWindow     time.Duration
	cooldownBase  time.Duration
	probeClaimTTL time.Duration

	groupPointerPersistMu   sync.Mutex
	groupPointerPersistLast map[int64]groupPointerPersistState
	groupPointerSyncMu      sync.Mutex
	groupPointerSync        map[int64]groupPointerSyncState
}

type Constraints struct {
	RequireChannelType string
	RequireChannelID   int64
	AllowGroups        map[string]struct{}
	AllowGroupOrder    []string
	AllowChannelIDs    map[int64]struct{}
}

type UpstreamStore interface {
	ListUpstreamChannels(ctx context.Context) ([]store.UpstreamChannel, error)
	ListUpstreamEndpointsByChannel(ctx context.Context, channelID int64) ([]store.UpstreamEndpoint, error)
	ListOpenAICompatibleCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error)
	ListAnthropicCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.AnthropicCredential, error)
	ListCodexOAuthAccountsByEndpoint(ctx context.Context, endpointID int64) ([]store.CodexOAuthAccount, error)
}

type BindingStore interface {
	GetSessionBindingPayload(ctx context.Context, userID int64, routeKeyHash string, now time.Time) (string, bool, error)
	UpsertSessionBindingPayload(ctx context.Context, userID int64, routeKeyHash string, payload string, expiresAt time.Time) error
	DeleteSessionBinding(ctx context.Context, userID int64, routeKeyHash string) error
}

type ChannelGroupPointerStore interface {
	GetChannelGroupPointer(ctx context.Context, groupID int64) (store.ChannelGroupPointer, bool, error)
	UpsertChannelGroupPointer(ctx context.Context, in store.ChannelGroupPointer) error
}

const (
	bindingSetSourceSelect      = "select"
	bindingSetSourceTouch       = "touch"
	bindingSetSourceStoreWarmup = "store_restore"

	bindingClearReasonManual       = "manual"
	bindingClearReasonIneligible   = "ineligible"
	bindingClearReasonProbePending = "probe_pending"
	bindingClearReasonParseError   = "parse_error"
)

func New(st UpstreamStore) *Scheduler {
	s := &Scheduler{
		st:                      st,
		state:                   NewState(),
		affinityTTL:             30 * time.Minute,
		bindingTTL:              1 * time.Hour,
		rpmWindow:               60 * time.Second,
		cooldownBase:            30 * time.Second,
		probeClaimTTL:           30 * time.Second,
		groupPointerPersistLast: make(map[int64]groupPointerPersistState),
		groupPointerSync:        make(map[int64]groupPointerSyncState),
	}
	return s
}

func (s *Scheduler) SetBindingStore(bs BindingStore) {
	if s == nil {
		return
	}
	s.bindingStore = bs
}

func (s *Scheduler) SetGroupPointerStore(ps ChannelGroupPointerStore) {
	if s == nil {
		return
	}
	s.groupPointers = ps
}

// InvalidateUpstreamSnapshot 清空上游配置快照缓存（若启用）。
func (s *Scheduler) InvalidateUpstreamSnapshot() {
	if s == nil || s.st == nil {
		return
	}
	type invalidator interface {
		InvalidateAll()
	}
	if inv, ok := s.st.(invalidator); ok {
		inv.InvalidateAll()
	}
}

type groupPointerPersistState struct {
	channelID int64
	pinned    bool
}

type groupPointerSyncState struct {
	rec         store.ChannelGroupPointer
	ok          bool
	lastRefresh time.Time
}

func (s *Scheduler) maybeSyncChannelGroupPointerFromStore(ctx context.Context, groupID int64) (store.ChannelGroupPointer, bool) {
	if s == nil || s.groupPointers == nil || groupID <= 0 {
		return store.ChannelGroupPointer{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()

	s.groupPointerSyncMu.Lock()
	entry := s.groupPointerSync[groupID]
	if !entry.lastRefresh.IsZero() && now.Sub(entry.lastRefresh) < 1*time.Second {
		s.groupPointerSyncMu.Unlock()
		return entry.rec, entry.ok
	}
	entry.lastRefresh = now
	prevRec := entry.rec
	prevOK := entry.ok
	s.groupPointerSync[groupID] = entry
	s.groupPointerSyncMu.Unlock()

	rec, ok, err := s.groupPointers.GetChannelGroupPointer(ctx, groupID)
	if err != nil {
		return prevRec, prevOK
	}
	if !ok {
		rec = store.ChannelGroupPointer{GroupID: groupID}
	}

	s.groupPointerSyncMu.Lock()
	s.groupPointerSync[groupID] = groupPointerSyncState{
		rec:         rec,
		ok:          ok,
		lastRefresh: now,
	}
	s.groupPointerSyncMu.Unlock()

	s.groupPointerPersistMu.Lock()
	s.groupPointerPersistLast[groupID] = groupPointerPersistState{
		channelID: rec.ChannelID,
		pinned:    rec.Pinned,
	}
	s.groupPointerPersistMu.Unlock()

	return rec, ok
}

func (s *Scheduler) upsertChannelGroupPointer(in store.ChannelGroupPointer) {
	if s == nil || s.groupPointers == nil || in.GroupID <= 0 {
		return
	}

	s.groupPointerPersistMu.Lock()
	last := s.groupPointerPersistLast[in.GroupID]
	if in.ChannelID == last.channelID && in.Pinned == last.pinned {
		s.groupPointerPersistMu.Unlock()
		return
	}
	s.groupPointerPersistLast[in.GroupID] = groupPointerPersistState{
		channelID: in.ChannelID,
		pinned:    in.Pinned,
	}
	s.groupPointerPersistMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	_ = s.groupPointers.UpsertChannelGroupPointer(ctx, in)
	cancel()

	s.groupPointerSyncMu.Lock()
	entry := s.groupPointerSync[in.GroupID]
	entry.rec = in
	entry.ok = true
	entry.lastRefresh = time.Now()
	s.groupPointerSync[in.GroupID] = entry
	s.groupPointerSyncMu.Unlock()
}

func (s *Scheduler) setChannelGroupPointer(groupID int64, channelID int64, pinned bool, reason string) {
	if s == nil || s.groupPointers == nil || groupID <= 0 {
		return
	}
	if reason == "" {
		reason = "route"
	}
	now := time.Now()
	ms := now.UnixMilli()
	s.upsertChannelGroupPointer(store.ChannelGroupPointer{
		GroupID:       groupID,
		ChannelID:     channelID,
		Pinned:        pinned,
		MovedAtUnixMS: ms,
		Reason:        reason,
	})
}

func (s *Scheduler) touchChannelGroupPointer(ctx context.Context, groupID int64, channelID int64, reason string) {
	if s == nil || s.groupPointers == nil || groupID <= 0 || channelID <= 0 {
		return
	}
	if reason == "" {
		reason = "route"
	}

	rec, ok := s.maybeSyncChannelGroupPointerFromStore(ctx, groupID)
	pinned := false
	if ok {
		pinned = rec.Pinned
	}
	if ok && rec.ChannelID == channelID {
		return
	}
	s.setChannelGroupPointer(groupID, channelID, pinned, reason)
}

func (s *Scheduler) ClearChannelBan(channelID int64) {
	if s == nil || s.state == nil {
		return
	}
	s.state.ClearChannelBan(channelID)
}

func (s *Scheduler) RouteKeyHash(routeKey string) string {
	if routeKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(routeKey))
	return hex.EncodeToString(sum[:])
}

func (s *Scheduler) TouchBinding(userID int64, routeKeyHash string, sel Selection) {
	if routeKeyHash == "" {
		return
	}
	s.setBinding(context.Background(), userID, routeKeyHash, sel, bindingSetSourceTouch)
}

func (s *Scheduler) SelectWithConstraints(ctx context.Context, userID int64, routeKeyHash string, cons Constraints) (Selection, error) {
	now := time.Now()

	// 1) 会话粘性：命中绑定则优先
	if routeKeyHash != "" {
		if sel, ok := s.getBinding(ctx, userID, routeKeyHash); ok {
			credKey := sel.CredentialKey()
			if selectionMatchesConstraints(sel, cons) &&
				!s.state.IsChannelBanned(sel.ChannelID, now) &&
				!s.state.IsCredentialCooling(credKey, now) &&
				s.state.ChannelFailScore(sel.ChannelID) == 0 {
				// 若该 channel 处于“封禁到期待探测”，先抢占 probe，避免并发探测风暴。
				if s.state.IsChannelProbeDue(sel.ChannelID) && !s.state.TryClaimChannelProbe(sel.ChannelID, now, s.probeClaimTTL) {
					// 已绑定但不可用：清理绑定，避免 session 永久占用导致 limits 失真。
					s.clearBinding(ctx, userID, routeKeyHash, bindingClearReasonProbePending)
				} else {
					// 命中成功后 touch 续期（TTL）。
					s.setBinding(ctx, userID, routeKeyHash, sel, bindingSetSourceTouch)
					s.state.RecordRPM(credKey, now)
					return sel, nil
				}
			}
			// 已绑定但不可用：清理绑定，避免 session 永久占用导致 limits 失真。
			s.clearBinding(ctx, userID, routeKeyHash, bindingClearReasonIneligible)
		}
	}

	// 2) 选择 channel：promotion > affinity > priority > fallback
	channels, err := s.st.ListUpstreamChannels(ctx)
	if err != nil {
		return Selection{}, err
	}
	var candidates []store.UpstreamChannel
	for _, ch := range channels {
		if ch.Status != 1 {
			continue
		}
		if s.state.IsChannelBanned(ch.ID, now) {
			continue
		}
		if ch.Type != store.UpstreamTypeOpenAICompatible && ch.Type != store.UpstreamTypeCodexOAuth && ch.Type != store.UpstreamTypeAnthropic {
			continue
		}
		if cons.RequireChannelType != "" && ch.Type != cons.RequireChannelType {
			continue
		}
		if cons.RequireChannelID != 0 && ch.ID != cons.RequireChannelID {
			continue
		}
		if cons.AllowGroups != nil && !channelInAnyGroup(ch.Groups, cons.AllowGroups) {
			continue
		}
		if cons.AllowChannelIDs != nil {
			if _, ok := cons.AllowChannelIDs[ch.ID]; !ok {
				continue
			}
		}
		candidates = append(candidates, ch)
	}
	if len(candidates) == 0 {
		return Selection{}, errors.New("未配置可用上游 channel")
	}

	affinityChannelID, affinityOK := s.state.GetAffinity(userID, now)
	if affinityOK && s.state.ChannelFailScore(affinityChannelID) > 0 {
		affinityOK = false
	}
	ordered := orderChannels(candidates, affinityChannelID, affinityOK, func(channelID int64) bool {
		return s.state.IsChannelProbePending(channelID, now)
	}, s.state.ChannelFailScore)

	// 3) 选择 endpoint + credential
	for _, ch := range ordered {
		claimedProbe := false
		if s.state.IsChannelProbeDue(ch.ID) {
			if !s.state.TryClaimChannelProbe(ch.ID, now, s.probeClaimTTL) {
				continue
			}
			claimedProbe = true
		}
		endpoints, err := s.st.ListUpstreamEndpointsByChannel(ctx, ch.ID)
		if err != nil {
			if claimedProbe {
				s.state.ReleaseChannelProbeClaim(ch.ID)
			}
			return Selection{}, err
		}
		var eps []store.UpstreamEndpoint
		for _, e := range endpoints {
			if e.Status != 1 {
				continue
			}
			eps = append(eps, e)
		}
		sort.SliceStable(eps, func(i, j int) bool {
			if eps[i].Priority != eps[j].Priority {
				return eps[i].Priority > eps[j].Priority
			}
			return eps[i].ID > eps[j].ID
		})
		for _, ep := range eps {
			sel, ok, err := s.selectCredential(ctx, ch, ep, now)
			if err != nil {
				if claimedProbe {
					s.state.ReleaseChannelProbeClaim(ch.ID)
				}
				return Selection{}, err
			}
			if ok {
				s.state.RecordRPM(sel.CredentialKey(), now)
				if routeKeyHash != "" {
					s.setBinding(ctx, userID, routeKeyHash, sel, bindingSetSourceSelect)
				}
				s.state.SetAffinity(userID, ch.ID, now.Add(s.affinityTTL))
				return sel, nil
			}
		}
		if claimedProbe {
			s.state.ReleaseChannelProbeClaim(ch.ID)
		}
	}
	return Selection{}, errors.New("未找到可用上游 credential/account")
}

func (s *Scheduler) getBinding(ctx context.Context, userID int64, routeKeyHash string) (Selection, bool) {
	if routeKeyHash == "" {
		return Selection{}, false
	}
	if sel, ok := s.state.GetBinding(userID, routeKeyHash); ok {
		s.state.RecordBindingMemoryHit()
		return sel, true
	}
	s.state.RecordBindingMiss()
	if s.bindingStore == nil {
		return Selection{}, false
	}
	payload, ok, err := s.bindingStore.GetSessionBindingPayload(ctx, userID, routeKeyHash, time.Now())
	if err != nil {
		s.state.RecordBindingStoreReadError()
		return Selection{}, false
	}
	if !ok {
		return Selection{}, false
	}
	var sel Selection
	if err := json.Unmarshal([]byte(payload), &sel); err != nil {
		s.state.RecordBindingClear(bindingClearReasonParseError)
		if err := s.bindingStore.DeleteSessionBinding(ctx, userID, routeKeyHash); err != nil {
			s.state.RecordBindingStoreDeleteError()
		}
		return Selection{}, false
	}
	s.state.SetBinding(userID, routeKeyHash, sel, time.Now().Add(s.bindingTTL))
	s.state.RecordBindingStoreHit()
	s.state.RecordBindingSet(bindingSetSourceStoreWarmup, false)
	return sel, true
}

func (s *Scheduler) setBinding(ctx context.Context, userID int64, routeKeyHash string, sel Selection, source string) {
	if routeKeyHash == "" {
		return
	}
	refreshed := s.state.HasBinding(userID, routeKeyHash, time.Now())
	expiresAt := time.Now().Add(s.bindingTTL)
	s.state.SetBinding(userID, routeKeyHash, sel, expiresAt)
	s.state.RecordBindingSet(source, refreshed)
	if s.bindingStore == nil {
		return
	}
	// 热路径优化：会话命中时的 touch 不需要每次都写入 bindingStore（高并发下会形成写热点）。
	// 允许 store 绑定过期仅影响跨实例粘性/亲和，不影响正确性。
	if source == bindingSetSourceTouch && refreshed {
		return
	}
	raw, err := json.Marshal(sel)
	if err != nil {
		return
	}
	if err := s.bindingStore.UpsertSessionBindingPayload(ctx, userID, routeKeyHash, string(raw), expiresAt); err != nil {
		s.state.RecordBindingStoreWriteError()
	}
}

func (s *Scheduler) clearBinding(ctx context.Context, userID int64, routeKeyHash string, reason string) {
	if routeKeyHash == "" {
		return
	}
	s.state.ClearBinding(userID, routeKeyHash)
	s.state.RecordBindingClear(reason)
	if s.bindingStore == nil {
		return
	}
	if err := s.bindingStore.DeleteSessionBinding(ctx, userID, routeKeyHash); err != nil {
		s.state.RecordBindingStoreDeleteError()
	}
}

func selectionMatchesConstraints(sel Selection, c Constraints) bool {
	if c.RequireChannelType != "" && sel.ChannelType != c.RequireChannelType {
		return false
	}
	if c.RequireChannelID != 0 && sel.ChannelID != c.RequireChannelID {
		return false
	}
	if c.AllowGroups != nil && !channelInAnyGroup(sel.ChannelGroups, c.AllowGroups) {
		return false
	}
	if c.AllowChannelIDs != nil {
		if _, ok := c.AllowChannelIDs[sel.ChannelID]; !ok {
			return false
		}
	}
	return true
}

func (s *Scheduler) selectCredential(ctx context.Context, ch store.UpstreamChannel, ep store.UpstreamEndpoint, now time.Time) (Selection, bool, error) {
	switch ch.Type {
	case store.UpstreamTypeOpenAICompatible:
		creds, err := s.st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var ids []int64
		for _, c := range creds {
			if c.Status != 1 {
				continue
			}
			key := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, c.ID)
			if s.state.IsCredentialCooling(key, now) {
				continue
			}
			ids = append(ids, c.ID)
		}
		if len(ids) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(ids, func(i, j int) bool {
			ki := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, ids[i])
			kj := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, ids[j])
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ids[i] > ids[j]
		})
		return Selection{
			ChannelID:              ch.ID,
			ChannelType:            ch.Type,
			ChannelGroups:          ch.Groups,
			AllowServiceTier:       ch.AllowServiceTier,
			DisableStore:           ch.DisableStore,
			AllowSafetyIdentifier:  ch.AllowSafetyIdentifier,
			OpenAIOrganization:     ch.OpenAIOrganization,
			AutoBan:                ch.AutoBan,
			ForceFormat:            ch.Setting.ForceFormat,
			ThinkingToContent:      ch.Setting.ThinkingToContent,
			PassThroughBodyEnabled: ch.Setting.PassThroughBodyEnabled,
			Proxy:                  ch.Setting.Proxy,
			SystemPrompt:           ch.Setting.SystemPrompt,
			SystemPromptOverride:   ch.Setting.SystemPromptOverride,
			CacheTTLPreference:     ch.Setting.CacheTTLPreference,
			ParamOverride:          ch.ParamOverride,
			HeaderOverride:         ch.HeaderOverride,
			StatusCodeMapping:      ch.StatusCodeMapping,
			ModelSuffixPreserve:    ch.ModelSuffixPreserve,
			RequestBodyBlacklist:   ch.RequestBodyBlacklist,
			RequestBodyWhitelist:   ch.RequestBodyWhitelist,
			EndpointID:             ep.ID,
			BaseURL:                ep.BaseURL,
			CredentialType:         CredentialTypeOpenAI,
			CredentialID:           ids[0],
		}, true, nil
	case store.UpstreamTypeAnthropic:
		creds, err := s.st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var ids []int64
		for _, c := range creds {
			if c.Status != 1 {
				continue
			}
			key := fmt.Sprintf("%s:%d", CredentialTypeAnthropic, c.ID)
			if s.state.IsCredentialCooling(key, now) {
				continue
			}
			ids = append(ids, c.ID)
		}
		if len(ids) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(ids, func(i, j int) bool {
			ki := fmt.Sprintf("%s:%d", CredentialTypeAnthropic, ids[i])
			kj := fmt.Sprintf("%s:%d", CredentialTypeAnthropic, ids[j])
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ids[i] > ids[j]
		})
		return Selection{
			ChannelID:              ch.ID,
			ChannelType:            ch.Type,
			ChannelGroups:          ch.Groups,
			AllowServiceTier:       ch.AllowServiceTier,
			DisableStore:           ch.DisableStore,
			AllowSafetyIdentifier:  ch.AllowSafetyIdentifier,
			OpenAIOrganization:     ch.OpenAIOrganization,
			AutoBan:                ch.AutoBan,
			ForceFormat:            ch.Setting.ForceFormat,
			ThinkingToContent:      ch.Setting.ThinkingToContent,
			PassThroughBodyEnabled: ch.Setting.PassThroughBodyEnabled,
			Proxy:                  ch.Setting.Proxy,
			SystemPrompt:           ch.Setting.SystemPrompt,
			SystemPromptOverride:   ch.Setting.SystemPromptOverride,
			CacheTTLPreference:     ch.Setting.CacheTTLPreference,
			ParamOverride:          ch.ParamOverride,
			HeaderOverride:         ch.HeaderOverride,
			StatusCodeMapping:      ch.StatusCodeMapping,
			ModelSuffixPreserve:    ch.ModelSuffixPreserve,
			RequestBodyBlacklist:   ch.RequestBodyBlacklist,
			RequestBodyWhitelist:   ch.RequestBodyWhitelist,
			EndpointID:             ep.ID,
			BaseURL:                ep.BaseURL,
			CredentialType:         CredentialTypeAnthropic,
			CredentialID:           ids[0],
		}, true, nil
	case store.UpstreamTypeCodexOAuth:
		accs, err := s.st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var eligible []store.CodexOAuthAccount
		for _, a := range accs {
			if a.Status != 1 {
				continue
			}
			if a.CooldownUntil != nil && now.Before(*a.CooldownUntil) {
				continue
			}
			key := fmt.Sprintf("%s:%d", CredentialTypeCodex, a.ID)
			if s.state.IsCredentialCooling(key, now) {
				continue
			}
			eligible = append(eligible, a)
		}
		if len(eligible) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(eligible, func(i, j int) bool {
			ai := eligible[i]
			aj := eligible[j]
			if codexLastUsedBefore(ai.LastUsedAt, aj.LastUsedAt) {
				return true
			}
			if codexLastUsedBefore(aj.LastUsedAt, ai.LastUsedAt) {
				return false
			}
			ki := fmt.Sprintf("%s:%d", CredentialTypeCodex, ai.ID)
			kj := fmt.Sprintf("%s:%d", CredentialTypeCodex, aj.ID)
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ai.ID > aj.ID
		})
		return Selection{
			ChannelID:              ch.ID,
			ChannelType:            ch.Type,
			ChannelGroups:          ch.Groups,
			AllowServiceTier:       ch.AllowServiceTier,
			DisableStore:           ch.DisableStore,
			AllowSafetyIdentifier:  ch.AllowSafetyIdentifier,
			OpenAIOrganization:     ch.OpenAIOrganization,
			AutoBan:                ch.AutoBan,
			ForceFormat:            ch.Setting.ForceFormat,
			ThinkingToContent:      ch.Setting.ThinkingToContent,
			PassThroughBodyEnabled: ch.Setting.PassThroughBodyEnabled,
			Proxy:                  ch.Setting.Proxy,
			SystemPrompt:           ch.Setting.SystemPrompt,
			SystemPromptOverride:   ch.Setting.SystemPromptOverride,
			CacheTTLPreference:     ch.Setting.CacheTTLPreference,
			ParamOverride:          ch.ParamOverride,
			HeaderOverride:         ch.HeaderOverride,
			StatusCodeMapping:      ch.StatusCodeMapping,
			ModelSuffixPreserve:    ch.ModelSuffixPreserve,
			RequestBodyBlacklist:   ch.RequestBodyBlacklist,
			RequestBodyWhitelist:   ch.RequestBodyWhitelist,
			EndpointID:             ep.ID,
			BaseURL:                ep.BaseURL,
			CredentialType:         CredentialTypeCodex,
			CredentialID:           eligible[0].ID,
		}, true, nil
	default:
		return Selection{}, false, nil
	}
}

func channelInAnyGroup(groups string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	if len(allowed) == 0 {
		return false
	}
	groups = strings.TrimSpace(groups)
	for _, g := range strings.Split(groups, ",") {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, ok := allowed[g]; ok {
			return true
		}
	}
	return false
}

func (s *Scheduler) Report(sel Selection, res Result) {
	now := time.Now()
	s.state.ClearChannelProbe(sel.ChannelID)
	if res.Success {
		s.state.RecordChannelResult(sel.ChannelID, true)
		s.state.RecordCredentialResult(sel.CredentialKey(), true)
		s.state.ClearChannelBan(sel.ChannelID)
		s.state.ResetChannelFailScore(sel.ChannelID)
		s.touchCredentialLastUsed(sel)
		return
	}
	s.state.RecordChannelResult(sel.ChannelID, false)
	s.state.RecordCredentialResult(sel.CredentialKey(), false)
	if res.Retriable {
		cooldown := s.cooldownBase
		if res.StatusCode == http.StatusTooManyRequests {
			cooldown = s.cooldownBase * 2
		}
		cooldownUntil := now.Add(cooldown)
		if res.CooldownUntil != nil && res.CooldownUntil.After(cooldownUntil) {
			cooldownUntil = *res.CooldownUntil
		}
		s.state.SetCredentialCooling(sel.CredentialKey(), cooldownUntil)
		// usage_limit_reached 属于账号级耗尽，不应牵连整个 channel。
		if sel.AutoBan && res.ErrorClass != "upstream_exhausted" {
			if shouldBanChannelImmediately(res) {
				s.state.BanChannelImmediate(sel.ChannelID, now, cooldown)
			} else {
				s.state.BanChannel(sel.ChannelID, now, cooldown)
			}
		}
	}
}

func codexLastUsedBefore(a, b *time.Time) bool {
	if a == nil && b != nil {
		return true
	}
	if a != nil && b == nil {
		return false
	}
	if a == nil && b == nil {
		return false
	}
	return a.Before(*b)
}

func (s *Scheduler) touchCredentialLastUsed(sel Selection) {
	if s == nil || s.st == nil || sel.CredentialID <= 0 {
		return
	}
	if sel.CredentialType != CredentialTypeCodex {
		return
	}
	toucher, ok := s.st.(interface {
		TouchCodexOAuthAccount(ctx context.Context, accountID int64)
	})
	if !ok {
		return
	}
	toucher.TouchCodexOAuthAccount(context.Background(), sel.CredentialID)
}

func shouldBanChannelImmediately(res Result) bool {
	// 不对“凭据层”失败做立即封禁：优先让同渠道其他 key/账号接管。
	switch res.StatusCode {
	case http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired:
		return false
	}

	// 连接/读写类异常通常是渠道整体不可用，立即封禁可更快切换到其它渠道。
	switch res.ErrorClass {
	case "network", "read_upstream", "stream_idle_timeout", "stream_read_error", "stream_first_byte_timeout":
		return true
	case "upstream_status":
		// 常见为 base_url/path/能力问题：切换 channel 更有意义。
		if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusMethodNotAllowed {
			return true
		}
	}

	return false
}

func (s *Scheduler) RecordTokens(credentialKey string, tokens int) {
	if s == nil || s.state == nil {
		return
	}
	if credentialKey == "" || tokens <= 0 {
		return
	}
	s.state.RecordTokens(credentialKey, time.Now(), tokens)
}

func (s *Scheduler) Sweep(now time.Time) {
	if s == nil || s.state == nil {
		return
	}
	// Keep token history bounded even if nobody reads it.
	const tokenWindow = 10 * time.Minute
	s.state.Sweep(now, s.rpmWindow, tokenWindow)
}

func orderChannels(chs []store.UpstreamChannel, affinityChannelID int64, affinityOK bool, isProbePending func(channelID int64) bool, failScore func(channelID int64) int) []store.UpstreamChannel {
	seen := make(map[int64]struct{}, len(chs))
	var probed []store.UpstreamChannel
	var promoted []store.UpstreamChannel
	var normal []store.UpstreamChannel
	for _, c := range chs {
		if isProbePending != nil && isProbePending(c.ID) {
			probed = append(probed, c)
			continue
		}
		if c.Promotion {
			promoted = append(promoted, c)
		} else {
			normal = append(normal, c)
		}
	}
	sort.SliceStable(probed, func(i, j int) bool {
		if probed[i].Priority != probed[j].Priority {
			return probed[i].Priority > probed[j].Priority
		}
		return failScore(probed[i].ID) < failScore(probed[j].ID)
	})
	sort.SliceStable(promoted, func(i, j int) bool {
		if promoted[i].Priority != promoted[j].Priority {
			return promoted[i].Priority > promoted[j].Priority
		}
		return failScore(promoted[i].ID) < failScore(promoted[j].ID)
	})
	sort.SliceStable(normal, func(i, j int) bool {
		if normal[i].Priority != normal[j].Priority {
			return normal[i].Priority > normal[j].Priority
		}
		return failScore(normal[i].ID) < failScore(normal[j].ID)
	})

	var out []store.UpstreamChannel
	for _, c := range probed {
		out = append(out, c)
		seen[c.ID] = struct{}{}
	}
	for _, c := range promoted {
		out = append(out, c)
		seen[c.ID] = struct{}{}
	}
	if affinityOK {
		for _, c := range chs {
			if c.ID == affinityChannelID {
				if _, ok := seen[c.ID]; !ok {
					out = append(out, c)
					seen[c.ID] = struct{}{}
				}
				break
			}
		}
	}
	for _, c := range normal {
		if _, ok := seen[c.ID]; ok {
			continue
		}
		out = append(out, c)
	}
	return out
}
