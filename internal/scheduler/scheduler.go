// Package scheduler 实现三层选择：Channel → Endpoint → Credential，并提供最小的亲和/粘性/冷却能力。
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"realms/internal/store"
)

type CredentialType string

var ErrFastModeUnsupported = errors.New("当前无支持 Fast mode 的可用上游")

const (
	CredentialTypeOpenAI    CredentialType = "openai_compatible"
	CredentialTypeCodex     CredentialType = "codex_oauth"
	CredentialTypeAnthropic CredentialType = "anthropic"
)

type FailureScope string

const (
	FailureScopeCredential FailureScope = "credential"
	FailureScopeEndpoint   FailureScope = "endpoint"
	FailureScopeChannel    FailureScope = "channel"
	FailureScopeRequest    FailureScope = "request"
)

type Selection struct {
	ChannelID     int64
	ChannelType   string
	ChannelGroups string
	RouteGroup    string

	AllowServiceTier       bool
	FastMode               bool
	DisableStore           bool
	AllowSafetyIdentifier  bool
	OpenAIOrganization     *string
	AutoBan                bool
	ForceFormat            bool
	ThinkingToContent      bool
	ChatCompletionsEnabled bool
	ResponsesEnabled       bool
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
	Scope      FailureScope
	// CooldownUntil 用于上层传入精确的冷却截止时间（例如上游返回 resets_at）。
	// 为空时按调度器默认策略计算。
	CooldownUntil *time.Time
}

type Scheduler struct {
	st UpstreamStore

	state         *State
	groupPointers ChannelGroupPointerStore
	affinityTTL   time.Duration
	rpmWindow     time.Duration
	cooldownBase  time.Duration
	probeClaimTTL time.Duration

	disableCodexOAuth bool

	groupPointerPersistMu   sync.Mutex
	groupPointerPersistLast map[int64]groupPointerPersistState
	groupPointerSyncMu      sync.Mutex
	groupPointerSync        map[int64]groupPointerSyncState
}

type Constraints struct {
	RequireChannelType   string
	RequireAPI           string
	RequireChannelID     int64
	RequireCredentialKey string
	RouteGroupHint       string
	RequireFastMode      bool
	AllowGroups          map[string]struct{}
	AllowGroupOrder      []string
	AllowChannelIDs      map[int64]struct{}
	// SequentialChannelFailover 用于用户侧 API key 的顺序转移：
	// 候选 channel 按绑定顺序从前往后尝试，失败后只向后推进，不做 ring/回绕/运行时重排。
	// 这里的“失败”定义在 channel 层：只有当前 channel 已无法选出任何可用 credential/account，
	// 才继续尝试后续 channel；同 channel 内部允许 credential/account 接管。
	SequentialChannelFailover bool
	// StartChannelID 表示“本次顺序转移”的当前起点。
	// 命中后先尝试该 channel；若该 channel 整体不可继续，才继续尝试它后面的 channel。
	StartChannelID int64
}

type Options struct {
	DisableCodexOAuth bool
}

var ErrRequiredCredentialUnavailable = errors.New("required credential unavailable")
var ErrRequiredChannelUnavailable = errors.New("required channel unavailable")
var ErrConstrainedSelectionUnavailable = errors.New("constrained selection unavailable")

const (
	RequiredAPIResponses       = "responses"
	RequiredAPIChatCompletions = "chat_completions"
	RequiredAPIMessages        = "messages"
)

type UpstreamStore interface {
	ListUpstreamChannels(ctx context.Context) ([]store.UpstreamChannel, error)
	ListUpstreamEndpointsByChannel(ctx context.Context, channelID int64) ([]store.UpstreamEndpoint, error)
	ListOpenAICompatibleCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error)
	ListAnthropicCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.AnthropicCredential, error)
	ListCodexOAuthAccountsByEndpoint(ctx context.Context, endpointID int64) ([]store.CodexOAuthAccount, error)
}

type ChannelGroupPointerStore interface {
	GetChannelGroupPointer(ctx context.Context, groupID int64) (store.ChannelGroupPointer, bool, error)
	UpsertChannelGroupPointer(ctx context.Context, in store.ChannelGroupPointer) error
}

func New(st UpstreamStore) *Scheduler {
	return NewWithOptions(st, Options{})
}

func NewWithOptions(st UpstreamStore, opts Options) *Scheduler {
	s := &Scheduler{
		st:                      st,
		state:                   NewState(),
		affinityTTL:             30 * time.Minute,
		rpmWindow:               60 * time.Second,
		cooldownBase:            30 * time.Second,
		probeClaimTTL:           30 * time.Second,
		disableCodexOAuth:       opts.DisableCodexOAuth,
		groupPointerPersistLast: make(map[int64]groupPointerPersistState),
		groupPointerSync:        make(map[int64]groupPointerSyncState),
	}
	return s
}

func (s *Scheduler) SetGroupPointerStore(ps ChannelGroupPointerStore) {
	if s == nil {
		return
	}
	s.groupPointers = ps
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

func rendezvousScore64(routeKeyHash, kind string, id int64) uint64 {
	if routeKeyHash == "" || kind == "" || id <= 0 {
		return 0
	}
	key := routeKeyHash + ":" + kind + ":" + strconv.FormatInt(id, 10)
	sum := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}

func (s *Scheduler) SelectWithConstraints(ctx context.Context, userID int64, routeKeyHash string, cons Constraints) (Selection, error) {
	return s.selectWithConstraints(ctx, userID, routeKeyHash, cons, false)
}

func (s *Scheduler) SelectWithConstraintsAllowBannedRequiredChannel(ctx context.Context, userID int64, routeKeyHash string, cons Constraints) (Selection, error) {
	return s.selectWithConstraints(ctx, userID, routeKeyHash, cons, true)
}

func (s *Scheduler) selectWithConstraints(ctx context.Context, userID int64, routeKeyHash string, cons Constraints, allowBannedRequiredChannel bool) (Selection, error) {
	now := time.Now()
	requirePinnedSelection := strings.TrimSpace(cons.RequireCredentialKey) != ""

	// 1) 选择 channel：promotion > affinity > priority > fallback
	channels, err := s.st.ListUpstreamChannels(ctx)
	if err != nil {
		return Selection{}, err
	}
	var candidates []store.UpstreamChannel
	for _, ch := range channels {
		if ch.Status != 1 {
			continue
		}
		if ch.Type != store.UpstreamTypeOpenAICompatible && ch.Type != store.UpstreamTypeCodexOAuth && ch.Type != store.UpstreamTypeAnthropic {
			continue
		}
		if s.disableCodexOAuth && ch.Type == store.UpstreamTypeCodexOAuth {
			continue
		}
		if cons.RequireChannelType != "" && ch.Type != cons.RequireChannelType {
			continue
		}
		if cons.RequireAPI != "" && !channelSupportsRequiredAPI(ch, cons.RequireAPI) {
			continue
		}
		if cons.RequireChannelID != 0 && ch.ID != cons.RequireChannelID {
			continue
		}
		if cons.AllowGroups != nil && !channelInAnyGroup(ch.Groups, cons.AllowGroups) {
			continue
		}
		if cons.RequireFastMode && (!ch.AllowServiceTier || !ch.FastMode) {
			continue
		}
		if cons.AllowChannelIDs != nil {
			if _, ok := cons.AllowChannelIDs[ch.ID]; !ok {
				continue
			}
		}
		if s.state.IsChannelBanned(ch.ID, now) && !(allowBannedRequiredChannel && cons.RequireChannelID != 0 && ch.ID == cons.RequireChannelID) {
			continue
		}
		candidates = append(candidates, ch)
	}
	if len(candidates) == 0 {
		if cons.RequireFastMode {
			return Selection{}, ErrFastModeUnsupported
		}
		if cons.RequireChannelID != 0 || strings.TrimSpace(cons.RequireCredentialKey) != "" {
			if cons.RequireChannelID != 0 {
				return Selection{}, errors.Join(ErrConstrainedSelectionUnavailable, ErrRequiredChannelUnavailable)
			}
			return Selection{}, errors.Join(ErrConstrainedSelectionUnavailable, ErrRequiredCredentialUnavailable)
		}
		return Selection{}, errors.New("未配置可用上游 channel")
	}

	affinityChannelID, affinityOK := s.state.GetAffinity(userID, now)
	if affinityOK && s.state.ChannelFailScore(affinityChannelID) > 0 {
		affinityOK = false
	}
	ordered := orderChannels(candidates, affinityChannelID, affinityOK, func(channelID int64) bool {
		return s.state.IsChannelProbePending(channelID, now)
	}, s.state.ChannelFailScore)
	if routeKeyHash != "" && len(ordered) > 1 {
		// 粘性路由：对“同一会话”的请求做稳定排序，减少跨上游漂移。
		// 注意：可用性仍由候选集过滤与 probe/ban/cooldown 机制保证。
		sort.SliceStable(ordered, func(i, j int) bool {
			si := rendezvousScore64(routeKeyHash, "channel", ordered[i].ID)
			sj := rendezvousScore64(routeKeyHash, "channel", ordered[j].ID)
			if si != sj {
				return si > sj
			}
			return ordered[i].ID > ordered[j].ID
		})
	}

	// 2) 选择 endpoint + credential
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
			if !requirePinnedSelection && s.state.IsEndpointCooling(e.ID, now) {
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
			sel, ok, err := s.selectCredential(ctx, ch, ep, now, routeKeyHash, cons)
			if err != nil {
				if claimedProbe {
					s.state.ReleaseChannelProbeClaim(ch.ID)
				}
				return Selection{}, err
			}
			if ok {
				s.state.RecordRPM(sel.CredentialKey(), now)
				// routeKeyHash 非空时，调度优先满足“同一会话粘性”，避免把 affinity（user 级）扩散成跨会话副作用。
				if routeKeyHash == "" {
					s.state.SetAffinity(userID, ch.ID, now.Add(s.affinityTTL))
				}
				return sel, nil
			}
		}
		if claimedProbe {
			s.state.ReleaseChannelProbeClaim(ch.ID)
		}
	}
	if strings.TrimSpace(cons.RequireCredentialKey) != "" {
		return Selection{}, errors.Join(ErrConstrainedSelectionUnavailable, ErrRequiredCredentialUnavailable)
	}
	if cons.RequireChannelID != 0 {
		return Selection{}, errors.Join(ErrConstrainedSelectionUnavailable, ErrRequiredChannelUnavailable)
	}
	return Selection{}, errors.New("未找到可用上游 credential/account")
}

func selectionMatchesConstraints(sel Selection, c Constraints) bool {
	if c.RequireChannelType != "" && sel.ChannelType != c.RequireChannelType {
		return false
	}
	if c.RequireAPI != "" && !selectionSupportsRequiredAPI(sel, c.RequireAPI) {
		return false
	}
	if c.RequireChannelID != 0 && sel.ChannelID != c.RequireChannelID {
		return false
	}
	if strings.TrimSpace(c.RequireCredentialKey) != "" && sel.CredentialKey() != strings.TrimSpace(c.RequireCredentialKey) {
		return false
	}
	if c.AllowGroups != nil && !channelInAnyGroup(sel.ChannelGroups, c.AllowGroups) {
		return false
	}
	if c.RequireFastMode && (!sel.AllowServiceTier || !sel.FastMode) {
		return false
	}
	if c.AllowChannelIDs != nil {
		if _, ok := c.AllowChannelIDs[sel.ChannelID]; !ok {
			return false
		}
	}
	return true
}

func channelSupportsRequiredAPI(ch store.UpstreamChannel, requiredAPI string) bool {
	chatEnabled, responsesEnabled := resolvedAPICapabilities(ch.Type, ch.Setting.ChatCompletionsEnabled, ch.Setting.ResponsesEnabled)
	switch strings.TrimSpace(requiredAPI) {
	case "":
		return true
	case RequiredAPIResponses:
		return responsesEnabled
	case RequiredAPIChatCompletions:
		return chatEnabled
	case RequiredAPIMessages:
		return ch.Type == store.UpstreamTypeAnthropic
	default:
		return true
	}
}

func selectionSupportsRequiredAPI(sel Selection, requiredAPI string) bool {
	chatEnabled, responsesEnabled := resolvedAPICapabilities(sel.ChannelType, sel.ChatCompletionsEnabled, sel.ResponsesEnabled)
	switch strings.TrimSpace(requiredAPI) {
	case "":
		return true
	case RequiredAPIResponses:
		return responsesEnabled
	case RequiredAPIChatCompletions:
		return chatEnabled
	case RequiredAPIMessages:
		return sel.ChannelType == store.UpstreamTypeAnthropic
	default:
		return true
	}
}

func resolvedAPICapabilities(channelType string, chatEnabled bool, responsesEnabled bool) (bool, bool) {
	if chatEnabled || responsesEnabled {
		return chatEnabled, responsesEnabled
	}
	switch strings.TrimSpace(channelType) {
	case store.UpstreamTypeOpenAICompatible:
		return true, true
	case store.UpstreamTypeCodexOAuth:
		return false, true
	default:
		return false, false
	}
}

func parseCredentialKey(key string) (CredentialType, int64, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", 0, false
	}
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	typ := CredentialType(strings.TrimSpace(parts[0]))
	if typ == "" {
		return "", 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	return typ, id, true
}

func (s *Scheduler) selectCredential(ctx context.Context, ch store.UpstreamChannel, ep store.UpstreamEndpoint, now time.Time, routeKeyHash string, cons Constraints) (Selection, bool, error) {
	requireCredKey := strings.TrimSpace(cons.RequireCredentialKey)
	requireCredType, requireCredID, requireCredOK := parseCredentialKey(requireCredKey)
	if requireCredKey != "" && !requireCredOK {
		// Invalid/stale bindings should not hard-fail routing; treat as "no match".
		return Selection{}, false, nil
	}
	switch ch.Type {
	case store.UpstreamTypeOpenAICompatible:
		if requireCredKey != "" && requireCredType != CredentialTypeOpenAI {
			return Selection{}, false, nil
		}
		creds, err := s.st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var ids []int64
		for _, c := range creds {
			if c.Status != 1 {
				continue
			}
			if requireCredKey != "" && c.ID != requireCredID {
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
		if routeKeyHash != "" && len(ids) > 1 {
			sort.SliceStable(ids, func(i, j int) bool {
				si := rendezvousScore64(routeKeyHash, string(CredentialTypeOpenAI), ids[i])
				sj := rendezvousScore64(routeKeyHash, string(CredentialTypeOpenAI), ids[j])
				if si != sj {
					return si > sj
				}
				return ids[i] > ids[j]
			})
		} else {
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
		}
		return Selection{
			ChannelID:              ch.ID,
			ChannelType:            ch.Type,
			ChannelGroups:          ch.Groups,
			AllowServiceTier:       ch.AllowServiceTier,
			FastMode:               ch.FastMode,
			DisableStore:           ch.DisableStore,
			AllowSafetyIdentifier:  ch.AllowSafetyIdentifier,
			OpenAIOrganization:     ch.OpenAIOrganization,
			AutoBan:                ch.AutoBan,
			ForceFormat:            ch.Setting.ForceFormat,
			ThinkingToContent:      ch.Setting.ThinkingToContent,
			ChatCompletionsEnabled: ch.Setting.ChatCompletionsEnabled,
			ResponsesEnabled:       ch.Setting.ResponsesEnabled,
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
		if requireCredKey != "" && requireCredType != CredentialTypeAnthropic {
			return Selection{}, false, nil
		}
		creds, err := s.st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var ids []int64
		for _, c := range creds {
			if c.Status != 1 {
				continue
			}
			if requireCredKey != "" && c.ID != requireCredID {
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
		if routeKeyHash != "" && len(ids) > 1 {
			sort.SliceStable(ids, func(i, j int) bool {
				si := rendezvousScore64(routeKeyHash, string(CredentialTypeAnthropic), ids[i])
				sj := rendezvousScore64(routeKeyHash, string(CredentialTypeAnthropic), ids[j])
				if si != sj {
					return si > sj
				}
				return ids[i] > ids[j]
			})
		} else {
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
		}
		return Selection{
			ChannelID:              ch.ID,
			ChannelType:            ch.Type,
			ChannelGroups:          ch.Groups,
			AllowServiceTier:       ch.AllowServiceTier,
			FastMode:               ch.FastMode,
			DisableStore:           ch.DisableStore,
			AllowSafetyIdentifier:  ch.AllowSafetyIdentifier,
			OpenAIOrganization:     ch.OpenAIOrganization,
			AutoBan:                ch.AutoBan,
			ForceFormat:            ch.Setting.ForceFormat,
			ThinkingToContent:      ch.Setting.ThinkingToContent,
			ChatCompletionsEnabled: ch.Setting.ChatCompletionsEnabled,
			ResponsesEnabled:       ch.Setting.ResponsesEnabled,
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
		if requireCredKey != "" && requireCredType != CredentialTypeCodex {
			return Selection{}, false, nil
		}
		accs, err := s.st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var eligible []store.CodexOAuthAccount
		for _, a := range accs {
			if a.Status != 1 {
				continue
			}
			if requireCredKey != "" && a.ID != requireCredID {
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
		if routeKeyHash != "" && len(eligible) > 1 {
			sort.SliceStable(eligible, func(i, j int) bool {
				ai := eligible[i]
				aj := eligible[j]
				si := rendezvousScore64(routeKeyHash, string(CredentialTypeCodex), ai.ID)
				sj := rendezvousScore64(routeKeyHash, string(CredentialTypeCodex), aj.ID)
				if si != sj {
					return si > sj
				}
				return ai.ID > aj.ID
			})
		} else {
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
		}
		return Selection{
			ChannelID:              ch.ID,
			ChannelType:            ch.Type,
			ChannelGroups:          ch.Groups,
			AllowServiceTier:       ch.AllowServiceTier,
			FastMode:               ch.FastMode,
			DisableStore:           ch.DisableStore,
			AllowSafetyIdentifier:  ch.AllowSafetyIdentifier,
			OpenAIOrganization:     ch.OpenAIOrganization,
			AutoBan:                ch.AutoBan,
			ForceFormat:            ch.Setting.ForceFormat,
			ThinkingToContent:      ch.Setting.ThinkingToContent,
			ChatCompletionsEnabled: ch.Setting.ChatCompletionsEnabled,
			ResponsesEnabled:       ch.Setting.ResponsesEnabled,
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
	scope := res.Scope
	if scope == "" {
		scope = FailureScopeChannel
	}
	s.state.ClearChannelProbe(sel.ChannelID)
	if res.Success {
		s.state.ClearEndpointCooldown(sel.EndpointID)
		s.state.RecordChannelResult(sel.ChannelID, true)
		s.state.RecordCredentialResult(sel.CredentialKey(), true)
		s.state.ClearChannelBan(sel.ChannelID)
		s.state.ResetChannelFailScore(sel.ChannelID)
		s.touchCredentialLastUsed(sel)
		return
	}
	if scope != FailureScopeRequest {
		s.state.RecordCredentialResult(sel.CredentialKey(), false)
	}
	if scope == FailureScopeChannel {
		s.state.RecordChannelResult(sel.ChannelID, false)
	}
	if res.Retriable {
		cooldown := s.cooldownBase
		if res.StatusCode == http.StatusTooManyRequests {
			cooldown = s.cooldownBase * 2
		}
		cooldownUntil := now.Add(cooldown)
		if res.CooldownUntil != nil && res.CooldownUntil.After(cooldownUntil) {
			cooldownUntil = *res.CooldownUntil
		}
		switch scope {
		case FailureScopeCredential:
			s.state.SetCredentialCooling(sel.CredentialKey(), cooldownUntil)
		case FailureScopeEndpoint:
			s.state.SetEndpointCooling(sel.EndpointID, cooldownUntil)
		case FailureScopeChannel:
			s.state.SetEndpointCooling(sel.EndpointID, cooldownUntil)
			s.state.SetCredentialCooling(sel.CredentialKey(), cooldownUntil)
		default:
			s.state.SetCredentialCooling(sel.CredentialKey(), cooldownUntil)
		}
		// usage_limit_reached / rate_limit_exceeded / credential invalid 属于账号级耗尽/限流/不可用，不应牵连整个 channel。
		if scope == FailureScopeChannel && sel.AutoBan && res.ErrorClass != "upstream_exhausted" && res.ErrorClass != "upstream_throttled" && res.ErrorClass != "upstream_credential_invalid" {
			if shouldBanChannelImmediately(res) {
				s.state.BanChannelImmediate(sel.ChannelID, now, cooldown)
			} else {
				s.state.BanChannel(sel.ChannelID, now, cooldown)
			}
		}
	}
}

func (s *Scheduler) IsEndpointCooling(endpointID int64) bool {
	if s == nil || s.state == nil {
		return false
	}
	return s.state.IsEndpointCooling(endpointID, time.Now())
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
	if res.Scope != "" && res.Scope != FailureScopeChannel {
		return false
	}
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
