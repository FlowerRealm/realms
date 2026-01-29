package scheduler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"realms/internal/store"
)

type fakeStore struct {
	channels       []store.UpstreamChannel
	endpoints      map[int64][]store.UpstreamEndpoint
	creds          map[int64][]store.OpenAICompatibleCredential
	anthropicCreds map[int64][]store.AnthropicCredential
	accounts       map[int64][]store.CodexOAuthAccount
}

func (f *fakeStore) ListUpstreamChannels(_ context.Context) ([]store.UpstreamChannel, error) {
	return f.channels, nil
}

func (f *fakeStore) ListUpstreamEndpointsByChannel(_ context.Context, channelID int64) ([]store.UpstreamEndpoint, error) {
	return f.endpoints[channelID], nil
}

func (f *fakeStore) ListOpenAICompatibleCredentialsByEndpoint(_ context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error) {
	return f.creds[endpointID], nil
}

func (f *fakeStore) ListAnthropicCredentialsByEndpoint(_ context.Context, endpointID int64) ([]store.AnthropicCredential, error) {
	if f.anthropicCreds == nil {
		return nil, nil
	}
	return f.anthropicCreds[endpointID], nil
}

func (f *fakeStore) ListCodexOAuthAccountsByEndpoint(_ context.Context, endpointID int64) ([]store.CodexOAuthAccount, error) {
	return f.accounts[endpointID], nil
}

func TestSelect_PromotionBeatsPriority(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Promotion: false},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Promotion: true},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	sel, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2, got=%d", sel.ChannelID)
	}
}

func TestSelect_PinnedChannelBeatsPromotion(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Promotion: true},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Promotion: false},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	s.state.SetChannelPointerRing([]int64{2, 1})
	s.PinChannel(2)

	sel, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected pinned channel=2, got=%d", sel.ChannelID)
	}
}

func TestSelect_AffinityBeatsPriority(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	s.state.SetAffinity(99, 1, time.Now().Add(10*time.Minute))
	sel, err := s.Select(context.Background(), 99, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 1 {
		t.Fatalf("expected channel=1 due to affinity, got=%d", sel.ChannelID)
	}
}

func TestState_IsChannelBanned_ExpiredMarksProbeDue(t *testing.T) {
	st := NewState()
	now := time.Now()

	st.mu.Lock()
	st.channelBanUntil[1] = now.Add(-1 * time.Second)
	st.mu.Unlock()

	if st.IsChannelBanned(1, now) {
		t.Fatalf("expected channel to be unbanned when expired")
	}
	if !st.IsChannelProbePending(1, now) {
		t.Fatalf("expected expired ban to mark probe due")
	}
}

func TestState_BanChannelClampedToTenMinutes(t *testing.T) {
	st := NewState()
	now := time.Now()

	st.mu.Lock()
	st.channelBanUntil[1] = now.Add(20 * time.Minute)
	st.channelBanStreak[1] = 10
	st.mu.Unlock()

	until := st.BanChannel(1, now, 2*time.Minute)
	if until.After(now.Add(10 * time.Minute)) {
		t.Fatalf("expected ban_until to be clamped to <=10m, got=%v", until.Sub(now))
	}

	st.mu.Lock()
	stored := st.channelBanUntil[1]
	st.mu.Unlock()
	if stored.After(now.Add(10 * time.Minute)) {
		t.Fatalf("expected stored ban_until to be clamped to <=10m, got=%v", stored.Sub(now))
	}
}

func TestState_ListProbeDueChannels_RespectsClaimAndOrder(t *testing.T) {
	st := NewState()
	now := time.Now()

	st.mu.Lock()
	st.channelProbeDueAt[1] = now.Add(-2 * time.Second)
	st.channelProbeDueAt[2] = now.Add(-1 * time.Second)
	st.channelProbeDueAt[3] = now.Add(-3 * time.Second)
	st.channelProbeClaimUntil[2] = now.Add(10 * time.Second) // active claim
	st.channelProbeClaimUntil[3] = now.Add(-1 * time.Second) // expired claim
	st.mu.Unlock()

	got := st.ListProbeDueChannels(now, 10)
	if len(got) != 2 || got[0] != 3 || got[1] != 1 {
		t.Fatalf("unexpected probe due channels: %+v", got)
	}

	got1 := st.ListProbeDueChannels(now, 1)
	if len(got1) != 1 || got1[0] != 3 {
		t.Fatalf("unexpected probe due channels with limit=1: %+v", got1)
	}

	st.mu.Lock()
	_, ok := st.channelProbeClaimUntil[3]
	st.mu.Unlock()
	if ok {
		t.Fatalf("expected expired claim to be cleared")
	}
}

func TestSelect_ProbeChannelBeatsPromotionAndIsSingleFlight(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Promotion: false},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Promotion: true},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	s.state.mu.Lock()
	s.state.channelProbeDueAt[1] = time.Now()
	s.state.mu.Unlock()

	first, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if first.ChannelID != 1 {
		t.Fatalf("expected probe channel=1 to be selected first, got=%d", first.ChannelID)
	}

	second, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if second.ChannelID != 2 {
		t.Fatalf("expected second select to skip claimed probe and pick channel=2, got=%d", second.ChannelID)
	}
}

func TestReport_SuccessResetsChannelFailScoreAndClearsProbe(t *testing.T) {
	s := New(&fakeStore{})
	sel := Selection{ChannelID: 1, CredentialType: CredentialTypeOpenAI, CredentialID: 1}

	s.state.mu.Lock()
	s.state.channelProbeDueAt[1] = time.Now()
	s.state.channelProbeClaimUntil[1] = time.Now().Add(1 * time.Minute)
	s.state.mu.Unlock()

	s.Report(sel, Result{Success: false, Retriable: false})
	if got := s.state.ChannelFailScore(1); got == 0 {
		t.Fatalf("expected channel fail score to increase after failure")
	}

	s.Report(sel, Result{Success: true})
	if got := s.state.ChannelFailScore(1); got != 0 {
		t.Fatalf("expected channel fail score to be reset after success, got=%d", got)
	}
	if s.state.IsChannelProbeDue(1) {
		t.Fatalf("expected probe state to be cleared after report")
	}
}

func TestReport_RecordsLastSuccess(t *testing.T) {
	s := New(&fakeStore{})
	sel := Selection{ChannelID: 9, CredentialType: CredentialTypeOpenAI, CredentialID: 1}
	s.Report(sel, Result{Success: true})

	got, _, ok := s.LastSuccess()
	if !ok {
		t.Fatalf("expected last success to be recorded")
	}
	if got.ChannelID != 9 {
		t.Fatalf("expected channel=9, got=%d", got.ChannelID)
	}
}

func TestSelect_LowestRPMWins(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
				{ID: 2, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)
	now := time.Now()
	s.state.RecordRPM("openai_compatible:2", now.Add(-10*time.Second))
	s.state.RecordRPM("openai_compatible:2", now.Add(-9*time.Second))

	sel, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.CredentialID != 1 {
		t.Fatalf("expected credential=1 (rpm lower), got=%d", sel.CredentialID)
	}
}

func TestSelect_BindingWinsIfNotCooling(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
				{ID: 2, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)
	routeKeyHash := s.RouteKeyHash("abc")
	want := Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   2,
	}
	s.state.SetBinding(10, routeKeyHash, want, time.Now().Add(10*time.Minute))
	sel, err := s.Select(context.Background(), 10, routeKeyHash)
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.CredentialID != 2 {
		t.Fatalf("expected credential=2 due to binding, got=%d", sel.CredentialID)
	}
}

func TestSelect_PinnedChannelOverridesBinding(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	s.state.SetChannelPointerRing([]int64{2, 1})

	routeKeyHash := s.RouteKeyHash("abc")
	s.state.SetBinding(10, routeKeyHash, Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   111,
	}, time.Now().Add(10*time.Minute))

	s.PinChannel(2)

	sel, err := s.Select(context.Background(), 10, routeKeyHash)
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2 due to pinned channel, got=%+v", sel)
	}

	got, ok := s.state.GetBinding(10, routeKeyHash)
	if !ok {
		t.Fatalf("expected binding to be set after select")
	}
	if got.ChannelID != 2 {
		t.Fatalf("expected binding channel=2 after select, got=%+v", got)
	}
}

func TestReport_Cooldown429LongerThanDefault(t *testing.T) {
	s := New(&fakeStore{})
	s.cooldownBase = 200 * time.Millisecond

	sel429 := Selection{CredentialType: CredentialTypeOpenAI, CredentialID: 1}
	start429 := time.Now()
	s.Report(sel429, Result{Success: false, Retriable: true, StatusCode: http.StatusTooManyRequests})

	s.state.mu.Lock()
	until429, ok429 := s.state.credentialCooldown[sel429.CredentialKey()]
	s.state.mu.Unlock()
	if !ok429 {
		t.Fatalf("expected credential cooldown for 429")
	}
	dur429 := until429.Sub(start429)

	sel500 := Selection{CredentialType: CredentialTypeOpenAI, CredentialID: 2}
	start500 := time.Now()
	s.Report(sel500, Result{Success: false, Retriable: true, StatusCode: http.StatusBadGateway})

	s.state.mu.Lock()
	until500, ok500 := s.state.credentialCooldown[sel500.CredentialKey()]
	s.state.mu.Unlock()
	if !ok500 {
		t.Fatalf("expected credential cooldown for 502")
	}
	dur500 := until500.Sub(start500)

	const slack = 50 * time.Millisecond
	if dur500 < s.cooldownBase-slack {
		t.Fatalf("expected default cooldown around %v, got=%v", s.cooldownBase, dur500)
	}
	if dur429 < (s.cooldownBase*2)-slack {
		t.Fatalf("expected 429 cooldown around %v, got=%v", s.cooldownBase*2, dur429)
	}
	if dur429 <= dur500 {
		t.Fatalf("expected 429 cooldown > default cooldown, got 429=%v default=%v", dur429, dur500)
	}
}

func TestSelectWithConstraints_ChannelType(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
			{ID: 2, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{RequireChannelType: store.UpstreamTypeCodexOAuth})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.ChannelType != store.UpstreamTypeCodexOAuth || sel.ChannelID != 2 || sel.CredentialType != CredentialTypeCodex {
		t.Fatalf("unexpected selection: %+v", sel)
	}
}

func TestSelectWithConstraints_ChannelID(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{RequireChannelID: 1})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.ChannelID != 1 || sel.CredentialID != 111 {
		t.Fatalf("unexpected selection: %+v", sel)
	}
}

func TestSelectWithConstraints_BindingMismatchIgnored(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
			{ID: 2, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	routeKeyHash := s.RouteKeyHash("abc")
	s.state.SetBinding(10, routeKeyHash, Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   111,
	}, time.Now().Add(10*time.Minute))

	sel, err := s.SelectWithConstraints(context.Background(), 10, routeKeyHash, Constraints{RequireChannelType: store.UpstreamTypeCodexOAuth})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.ChannelType != store.UpstreamTypeCodexOAuth {
		t.Fatalf("expected codex selection, got=%+v", sel)
	}
}

func TestSelectWithConstraints_AllowChannelIDs(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	allow := map[int64]struct{}{1: {}}
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{AllowChannelIDs: allow})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.ChannelID != 1 {
		t.Fatalf("expected allowed channel=1, got=%+v", sel)
	}
}

func TestSelectWithConstraints_AllowGroups(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g1"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Groups: "g2"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	allow := map[string]struct{}{"g2": {}}
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{AllowGroups: allow})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2 due to allow-groups, got=%+v", sel)
	}

	denyAll := map[string]struct{}{"nope": {}}
	if _, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{AllowGroups: denyAll}); err == nil {
		t.Fatalf("expected err when allow-groups matches none")
	}
}

func TestSelectWithConstraints_BindingMismatchByAllowSetIgnored(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	routeKeyHash := s.RouteKeyHash("abc")
	s.state.SetBinding(10, routeKeyHash, Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   111,
	}, time.Now().Add(10*time.Minute))

	allow := map[int64]struct{}{2: {}}
	sel, err := s.SelectWithConstraints(context.Background(), 10, routeKeyHash, Constraints{AllowChannelIDs: allow})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2 due to allow-set, got=%+v", sel)
	}
}
