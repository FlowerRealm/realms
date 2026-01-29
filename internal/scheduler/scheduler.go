// Package scheduler 实现三层选择：Channel → Endpoint → Credential，并提供最小的亲和/粘性/冷却能力。
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
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

	AllowServiceTier      bool
	DisableStore          bool
	AllowSafetyIdentifier bool
	ParamOverride         string
	HeaderOverride        string
	StatusCodeMapping     string
	ModelSuffixPreserve   string
	RequestBodyBlacklist  string
	RequestBodyWhitelist  string

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
}

type Scheduler struct {
	st UpstreamStore

	state         *State
	affinityTTL   time.Duration
	bindingTTL    time.Duration
	rpmWindow     time.Duration
	cooldownBase  time.Duration
	probeClaimTTL time.Duration
}

type Constraints struct {
	RequireChannelType string
	RequireChannelID   int64
	AllowGroups        map[string]struct{}
	AllowChannelIDs    map[int64]struct{}
}

type UpstreamStore interface {
	ListUpstreamChannels(ctx context.Context) ([]store.UpstreamChannel, error)
	ListUpstreamEndpointsByChannel(ctx context.Context, channelID int64) ([]store.UpstreamEndpoint, error)
	ListOpenAICompatibleCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error)
	ListAnthropicCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.AnthropicCredential, error)
	ListCodexOAuthAccountsByEndpoint(ctx context.Context, endpointID int64) ([]store.CodexOAuthAccount, error)
}

func New(st UpstreamStore) *Scheduler {
	return &Scheduler{
		st:            st,
		state:         NewState(),
		affinityTTL:   30 * time.Minute,
		bindingTTL:    30 * time.Minute,
		rpmWindow:     60 * time.Second,
		cooldownBase:  30 * time.Second,
		probeClaimTTL: 30 * time.Second,
	}
}

func (s *Scheduler) PinChannel(channelID int64) {
	if s == nil || s.state == nil {
		return
	}
	s.state.SetChannelPointer(channelID)
}

func (s *Scheduler) PinnedChannel() (int64, bool) {
	if s == nil || s.state == nil {
		return 0, false
	}
	return s.state.ChannelPointer(time.Now())
}

func (s *Scheduler) PinnedChannelInfo() (int64, time.Time, string, bool) {
	if s == nil || s.state == nil {
		return 0, time.Time{}, "", false
	}
	return s.state.ChannelPointerInfo(time.Now())
}

func (s *Scheduler) ClearPinnedChannel() {
	if s == nil || s.state == nil {
		return
	}
	s.state.ClearChannelPointer()
}

func (s *Scheduler) RefreshPinnedRing(ctx context.Context, st ChannelGroupStore) error {
	if s == nil || s.state == nil {
		return nil
	}
	ring, err := buildDefaultChannelRing(ctx, st)
	if err != nil {
		return err
	}
	s.state.SetChannelPointerRing(ring)
	return nil
}

func (s *Scheduler) ClearChannelBan(channelID int64) {
	if s == nil || s.state == nil {
		return
	}
	s.state.ClearChannelBan(channelID)
}

func (s *Scheduler) BanChannel(channelID int64, now time.Time, base time.Duration) time.Time {
	if s == nil || s.state == nil {
		return now
	}
	return s.state.BanChannel(channelID, now, base)
}

func (s *Scheduler) BanChannelImmediate(channelID int64, now time.Time, base time.Duration) time.Time {
	if s == nil || s.state == nil {
		return now
	}
	return s.state.BanChannelImmediate(channelID, now, base)
}

func (s *Scheduler) ResetChannelFailScore(channelID int64) {
	if s == nil || s.state == nil {
		return
	}
	s.state.ResetChannelFailScore(channelID)
}

func (s *Scheduler) SweepExpiredChannelBans(now time.Time) {
	if s == nil || s.state == nil {
		return
	}
	s.state.SweepExpiredChannelBans(now)
}

func (s *Scheduler) ListProbeDueChannels(now time.Time, limit int) []int64 {
	if s == nil || s.state == nil {
		return nil
	}
	return s.state.ListProbeDueChannels(now, limit)
}

func (s *Scheduler) TryClaimChannelProbe(channelID int64, now time.Time, ttl time.Duration) bool {
	if s == nil || s.state == nil {
		return false
	}
	return s.state.TryClaimChannelProbe(channelID, now, ttl)
}

func (s *Scheduler) ClearChannelProbe(channelID int64) {
	if s == nil || s.state == nil {
		return
	}
	s.state.ClearChannelProbe(channelID)
}

func (s *Scheduler) RouteKeyHash(routeKey string) string {
	if routeKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(routeKey))
	return hex.EncodeToString(sum[:])
}

func (s *Scheduler) GetBinding(userID int64, routeKeyHash string) (Selection, bool) {
	return s.state.GetBinding(userID, routeKeyHash)
}

func (s *Scheduler) TouchBinding(userID int64, routeKeyHash string, sel Selection) {
	if routeKeyHash == "" {
		return
	}
	s.state.SetBinding(userID, routeKeyHash, sel, time.Now().Add(s.bindingTTL))
}

func (s *Scheduler) ClearBinding(userID int64, routeKeyHash string) {
	s.state.ClearBinding(userID, routeKeyHash)
}

func (s *Scheduler) Select(ctx context.Context, userID int64, routeKeyHash string) (Selection, error) {
	return s.SelectWithConstraints(ctx, userID, routeKeyHash, Constraints{})
}

func (s *Scheduler) SelectWithConstraints(ctx context.Context, userID int64, routeKeyHash string, cons Constraints) (Selection, error) {
	now := time.Now()

	pointerID, pointerOK := s.state.ChannelPointer(now)
	pointerRing := s.state.ChannelPointerRing()
	pointerRelevant := pointerOK && pointerID != 0 && cons.RequireChannelID == 0 && len(pointerRing) > 0

	// 1) 会话粘性：命中绑定则优先
	if routeKeyHash != "" && !pointerRelevant {
		if sel, ok := s.state.GetBinding(userID, routeKeyHash); ok {
			credKey := sel.CredentialKey()
			if selectionMatchesConstraints(sel, cons) &&
				!s.state.IsChannelBanned(sel.ChannelID, now) &&
				!s.state.IsCredentialCooling(credKey, now) &&
				s.state.ChannelFailScore(sel.ChannelID) == 0 {
				// 若该 channel 处于“封禁到期待探测”，先抢占 probe，避免并发探测风暴。
				if s.state.IsChannelProbeDue(sel.ChannelID) && !s.state.TryClaimChannelProbe(sel.ChannelID, now, s.probeClaimTTL) {
					// 已绑定但不可用：清理绑定，避免 session 永久占用导致 limits 失真。
					s.state.ClearBinding(userID, routeKeyHash)
				} else {
					// 命中成功后 touch 续期（TTL）。
					s.state.SetBinding(userID, routeKeyHash, sel, now.Add(s.bindingTTL))
					s.state.RecordRPM(credKey, now)
					return sel, nil
				}
			}
			// 已绑定但不可用：清理绑定，避免 session 永久占用导致 limits 失真。
			s.state.ClearBinding(userID, routeKeyHash)
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
	var ordered []store.UpstreamChannel
	if pointerRelevant {
		byID := make(map[int64]store.UpstreamChannel, len(candidates))
		for _, ch := range candidates {
			byID[ch.ID] = ch
		}
		startIdx := 0
		for i, id := range pointerRing {
			if id == pointerID {
				startIdx = i
				break
			}
		}
		ordered = make([]store.UpstreamChannel, 0, len(candidates))
		for step := 0; step < len(pointerRing); step++ {
			id := pointerRing[(startIdx+step)%len(pointerRing)]
			if ch, ok := byID[id]; ok {
				ordered = append(ordered, ch)
			}
		}
		if len(ordered) == 0 {
			ordered = orderChannels(candidates, affinityChannelID, affinityOK, func(channelID int64) bool {
				return s.state.IsChannelProbePending(channelID, now)
			}, s.state.ChannelFailScore)
		} else if len(ordered) < len(candidates) {
			seen := make(map[int64]struct{}, len(ordered))
			for _, ch := range ordered {
				seen[ch.ID] = struct{}{}
			}
			for _, ch := range candidates {
				if _, ok := seen[ch.ID]; ok {
					continue
				}
				ordered = append(ordered, ch)
			}
		}
	} else {
		ordered = orderChannels(candidates, affinityChannelID, affinityOK, func(channelID int64) bool {
			return s.state.IsChannelProbePending(channelID, now)
		}, s.state.ChannelFailScore)
	}

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
					s.state.SetBinding(userID, routeKeyHash, sel, now.Add(s.bindingTTL))
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
			ChannelID:             ch.ID,
			ChannelType:           ch.Type,
			ChannelGroups:         ch.Groups,
			AllowServiceTier:      ch.AllowServiceTier,
			DisableStore:          ch.DisableStore,
			AllowSafetyIdentifier: ch.AllowSafetyIdentifier,
			ParamOverride:         ch.ParamOverride,
			HeaderOverride:        ch.HeaderOverride,
			StatusCodeMapping:     ch.StatusCodeMapping,
			ModelSuffixPreserve:   ch.ModelSuffixPreserve,
			RequestBodyBlacklist:  ch.RequestBodyBlacklist,
			RequestBodyWhitelist:  ch.RequestBodyWhitelist,
			EndpointID:            ep.ID,
			BaseURL:               ep.BaseURL,
			CredentialType:        CredentialTypeOpenAI,
			CredentialID:          ids[0],
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
			ChannelID:             ch.ID,
			ChannelType:           ch.Type,
			ChannelGroups:         ch.Groups,
			AllowServiceTier:      ch.AllowServiceTier,
			DisableStore:          ch.DisableStore,
			AllowSafetyIdentifier: ch.AllowSafetyIdentifier,
			ParamOverride:         ch.ParamOverride,
			HeaderOverride:        ch.HeaderOverride,
			StatusCodeMapping:     ch.StatusCodeMapping,
			ModelSuffixPreserve:   ch.ModelSuffixPreserve,
			RequestBodyBlacklist:  ch.RequestBodyBlacklist,
			RequestBodyWhitelist:  ch.RequestBodyWhitelist,
			EndpointID:            ep.ID,
			BaseURL:               ep.BaseURL,
			CredentialType:        CredentialTypeAnthropic,
			CredentialID:          ids[0],
		}, true, nil
	case store.UpstreamTypeCodexOAuth:
		accs, err := s.st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		var ids []int64
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
			ids = append(ids, a.ID)
		}
		if len(ids) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(ids, func(i, j int) bool {
			ki := fmt.Sprintf("%s:%d", CredentialTypeCodex, ids[i])
			kj := fmt.Sprintf("%s:%d", CredentialTypeCodex, ids[j])
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ids[i] > ids[j]
		})
		return Selection{
			ChannelID:             ch.ID,
			ChannelType:           ch.Type,
			ChannelGroups:         ch.Groups,
			AllowServiceTier:      ch.AllowServiceTier,
			DisableStore:          ch.DisableStore,
			AllowSafetyIdentifier: ch.AllowSafetyIdentifier,
			ParamOverride:         ch.ParamOverride,
			HeaderOverride:        ch.HeaderOverride,
			StatusCodeMapping:     ch.StatusCodeMapping,
			ModelSuffixPreserve:   ch.ModelSuffixPreserve,
			RequestBodyBlacklist:  ch.RequestBodyBlacklist,
			RequestBodyWhitelist:  ch.RequestBodyWhitelist,
			EndpointID:            ep.ID,
			BaseURL:               ep.BaseURL,
			CredentialType:        CredentialTypeCodex,
			CredentialID:          ids[0],
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
	if groups == "" {
		groups = "default"
	}
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
		return
	}
	s.state.RecordChannelResult(sel.ChannelID, false)
	s.state.RecordCredentialResult(sel.CredentialKey(), false)
	if res.Retriable {
		cooldown := s.cooldownBase
		if res.StatusCode == http.StatusTooManyRequests {
			cooldown = s.cooldownBase * 2
		}
		s.state.SetCredentialCooling(sel.CredentialKey(), now.Add(cooldown))
		if shouldBanChannelImmediately(res) {
			s.state.BanChannelImmediate(sel.ChannelID, now, cooldown)
		} else {
			s.state.BanChannel(sel.ChannelID, now, cooldown)
		}
	}
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
