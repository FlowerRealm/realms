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
	EndpointID    int64
	BaseURL       string

	CredentialType CredentialType
	CredentialID   int64

	CredentialLimitSessions *int
	CredentialLimitRPM      *int
	CredentialLimitTPM      *int
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

	state        *State
	affinityTTL  time.Duration
	bindingTTL   time.Duration
	rpmWindow    time.Duration
	cooldownBase time.Duration
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
		st:           st,
		state:        NewState(),
		affinityTTL:  30 * time.Minute,
		bindingTTL:   30 * time.Minute,
		rpmWindow:    60 * time.Second,
		cooldownBase: 30 * time.Second,
	}
}

func (s *Scheduler) ForceChannelFor(channelID int64, d time.Duration) time.Time {
	if s == nil || s.state == nil || channelID <= 0 || d <= 0 {
		return time.Time{}
	}
	until := time.Now().Add(d)
	s.state.SetForcedChannel(channelID, until)
	return until
}

func (s *Scheduler) ForcedChannel(now time.Time) (int64, time.Time, bool) {
	if s == nil || s.state == nil {
		return 0, time.Time{}, false
	}
	return s.state.ForcedChannel(now)
}

func (s *Scheduler) ClearChannelBan(channelID int64) {
	if s == nil || s.state == nil {
		return
	}
	s.state.ClearChannelBan(channelID)
}

func (s *Scheduler) LastSuccess() (Selection, time.Time, bool) {
	if s == nil || s.state == nil {
		return Selection{}, time.Time{}, false
	}
	return s.state.LastSuccess()
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

	// 1) 会话粘性：命中绑定则优先
	if routeKeyHash != "" {
		if sel, ok := s.state.GetBinding(userID, routeKeyHash); ok {
			credKey := sel.CredentialKey()
			overRPM := sel.CredentialLimitRPM != nil && *sel.CredentialLimitRPM > 0 && s.state.RPM(credKey, now, s.rpmWindow) >= *sel.CredentialLimitRPM
			overTPM := sel.CredentialLimitTPM != nil && *sel.CredentialLimitTPM > 0 && s.state.TPM(credKey, now, s.rpmWindow) >= *sel.CredentialLimitTPM
			if selectionMatchesConstraints(sel, cons) &&
				!s.state.IsChannelBanned(sel.ChannelID, now) &&
				!s.state.IsCredentialCooling(credKey, now) &&
				!overRPM &&
				!overTPM {
				// 命中成功后 touch 续期（TTL）。
				s.state.SetBinding(userID, routeKeyHash, sel, now.Add(s.bindingTTL))
				s.state.RecordRPM(credKey, now)
				return sel, nil
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
	forcedChannelID, _, forcedOK := s.state.ForcedChannel(now)
	if !forcedOK {
		forcedChannelID = 0
	}
	ordered := orderChannels(candidates, forcedChannelID, affinityChannelID, affinityOK, s.state.ChannelFailScore)

	// 3) 选择 endpoint + credential
	for _, ch := range ordered {
		endpoints, err := s.st.ListUpstreamEndpointsByChannel(ctx, ch.ID)
		if err != nil {
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
			sel, ok, err := s.selectCredential(ctx, ch, ep, now, routeKeyHash != "")
			if err != nil {
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

func (s *Scheduler) selectCredential(ctx context.Context, ch store.UpstreamChannel, ep store.UpstreamEndpoint, now time.Time, isNewSession bool) (Selection, bool, error) {
	switch ch.Type {
	case store.UpstreamTypeOpenAICompatible:
		creds, err := s.st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		type cand struct {
			id            int64
			limitSessions *int
			limitRPM      *int
			limitTPM      *int
		}
		var ids []cand
		for _, c := range creds {
			if c.Status != 1 {
				continue
			}
			key := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, c.ID)
			if s.state.IsCredentialCooling(key, now) {
				continue
			}
			if c.LimitRPM != nil && *c.LimitRPM > 0 {
				if s.state.RPM(key, now, s.rpmWindow) >= *c.LimitRPM {
					continue
				}
			}
			if c.LimitTPM != nil && *c.LimitTPM > 0 {
				if s.state.TPM(key, now, s.rpmWindow) >= *c.LimitTPM {
					continue
				}
			}
			if isNewSession && c.LimitSessions != nil && *c.LimitSessions > 0 {
				if s.state.CredentialSessions(key, now) >= *c.LimitSessions {
					continue
				}
			}
			ids = append(ids, cand{id: c.ID, limitSessions: c.LimitSessions, limitRPM: c.LimitRPM, limitTPM: c.LimitTPM})
		}
		if len(ids) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(ids, func(i, j int) bool {
			ki := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, ids[i].id)
			kj := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, ids[j].id)
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ids[i].id > ids[j].id
		})
		return Selection{
			ChannelID:      ch.ID,
			ChannelType:    ch.Type,
			ChannelGroups:  ch.Groups,
			EndpointID:     ep.ID,
			BaseURL:        ep.BaseURL,
			CredentialType: CredentialTypeOpenAI,
			CredentialID:   ids[0].id,

			CredentialLimitSessions: ids[0].limitSessions,
			CredentialLimitRPM:      ids[0].limitRPM,
			CredentialLimitTPM:      ids[0].limitTPM,
		}, true, nil
	case store.UpstreamTypeAnthropic:
		creds, err := s.st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		type cand struct {
			id            int64
			limitSessions *int
			limitRPM      *int
			limitTPM      *int
		}
		var ids []cand
		for _, c := range creds {
			if c.Status != 1 {
				continue
			}
			key := fmt.Sprintf("%s:%d", CredentialTypeAnthropic, c.ID)
			if s.state.IsCredentialCooling(key, now) {
				continue
			}
			if c.LimitRPM != nil && *c.LimitRPM > 0 {
				if s.state.RPM(key, now, s.rpmWindow) >= *c.LimitRPM {
					continue
				}
			}
			if c.LimitTPM != nil && *c.LimitTPM > 0 {
				if s.state.TPM(key, now, s.rpmWindow) >= *c.LimitTPM {
					continue
				}
			}
			if isNewSession && c.LimitSessions != nil && *c.LimitSessions > 0 {
				if s.state.CredentialSessions(key, now) >= *c.LimitSessions {
					continue
				}
			}
			ids = append(ids, cand{id: c.ID, limitSessions: c.LimitSessions, limitRPM: c.LimitRPM, limitTPM: c.LimitTPM})
		}
		if len(ids) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(ids, func(i, j int) bool {
			ki := fmt.Sprintf("%s:%d", CredentialTypeAnthropic, ids[i].id)
			kj := fmt.Sprintf("%s:%d", CredentialTypeAnthropic, ids[j].id)
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ids[i].id > ids[j].id
		})
		return Selection{
			ChannelID:      ch.ID,
			ChannelType:    ch.Type,
			ChannelGroups:  ch.Groups,
			EndpointID:     ep.ID,
			BaseURL:        ep.BaseURL,
			CredentialType: CredentialTypeAnthropic,
			CredentialID:   ids[0].id,

			CredentialLimitSessions: ids[0].limitSessions,
			CredentialLimitRPM:      ids[0].limitRPM,
			CredentialLimitTPM:      ids[0].limitTPM,
		}, true, nil
	case store.UpstreamTypeCodexOAuth:
		accs, err := s.st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
		if err != nil {
			return Selection{}, false, err
		}
		type cand struct {
			id            int64
			limitSessions *int
			limitRPM      *int
			limitTPM      *int
		}
		var ids []cand
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
			if a.LimitRPM != nil && *a.LimitRPM > 0 {
				if s.state.RPM(key, now, s.rpmWindow) >= *a.LimitRPM {
					continue
				}
			}
			if a.LimitTPM != nil && *a.LimitTPM > 0 {
				if s.state.TPM(key, now, s.rpmWindow) >= *a.LimitTPM {
					continue
				}
			}
			if isNewSession && a.LimitSessions != nil && *a.LimitSessions > 0 {
				if s.state.CredentialSessions(key, now) >= *a.LimitSessions {
					continue
				}
			}
			ids = append(ids, cand{id: a.ID, limitSessions: a.LimitSessions, limitRPM: a.LimitRPM, limitTPM: a.LimitTPM})
		}
		if len(ids) == 0 {
			return Selection{}, false, nil
		}
		sort.SliceStable(ids, func(i, j int) bool {
			ki := fmt.Sprintf("%s:%d", CredentialTypeCodex, ids[i].id)
			kj := fmt.Sprintf("%s:%d", CredentialTypeCodex, ids[j].id)
			ri := s.state.RPM(ki, now, s.rpmWindow)
			rj := s.state.RPM(kj, now, s.rpmWindow)
			if ri != rj {
				return ri < rj
			}
			return ids[i].id > ids[j].id
		})
		return Selection{
			ChannelID:      ch.ID,
			ChannelType:    ch.Type,
			ChannelGroups:  ch.Groups,
			EndpointID:     ep.ID,
			BaseURL:        ep.BaseURL,
			CredentialType: CredentialTypeCodex,
			CredentialID:   ids[0].id,

			CredentialLimitSessions: ids[0].limitSessions,
			CredentialLimitRPM:      ids[0].limitRPM,
			CredentialLimitTPM:      ids[0].limitTPM,
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
	if res.Success {
		s.state.RecordChannelResult(sel.ChannelID, true)
		s.state.RecordCredentialResult(sel.CredentialKey(), true)
		s.state.ClearChannelBan(sel.ChannelID)
		s.state.RecordLastSuccess(sel, now)
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
		s.state.BanChannel(sel.ChannelID, now, cooldown)
	}
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

func orderChannels(chs []store.UpstreamChannel, forcedChannelID int64, affinityChannelID int64, affinityOK bool, failScore func(channelID int64) int) []store.UpstreamChannel {
	seen := make(map[int64]struct{}, len(chs))
	var forced []store.UpstreamChannel
	var promoted []store.UpstreamChannel
	var normal []store.UpstreamChannel
	for _, c := range chs {
		if forcedChannelID != 0 && c.ID == forcedChannelID {
			forced = append(forced, c)
			continue
		}
		if c.Promotion {
			promoted = append(promoted, c)
		} else {
			normal = append(normal, c)
		}
	}
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
	for _, c := range forced {
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
