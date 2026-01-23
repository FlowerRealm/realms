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

func TestSelect_ForcedChannelBeatsPromotion(t *testing.T) {
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
	s.state.SetForcedChannel(2, time.Now().Add(5*time.Minute))

	sel, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected forced channel=2, got=%d", sel.ChannelID)
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

func TestSortCandidates_ForcedChannelFirst(t *testing.T) {
	in := map[int64]channelCandidate{
		1: {ChannelID: 1, Priority: 0, Promotion: true},
		2: {ChannelID: 2, Priority: 100, Promotion: false},
	}
	out := sortCandidates(in, 2, func(int64) int { return 0 })
	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got=%d", len(out))
	}
	if out[0].ChannelID != 2 {
		t.Fatalf("expected forced channel first, got=%d", out[0].ChannelID)
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

func TestSelect_CredentialRPMLimit(t *testing.T) {
	limit := 1
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
				{ID: 111, EndpointID: 11, Status: 1, LimitRPM: &limit},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	sel1, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select #1 err: %v", err)
	}
	if sel1.ChannelID != 1 {
		t.Fatalf("expected channel=1, got=%d", sel1.ChannelID)
	}

	sel2, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select #2 err: %v", err)
	}
	if sel2.ChannelID != 2 {
		t.Fatalf("expected channel=2 due to rpm limit, got=%d", sel2.ChannelID)
	}
}

func TestSelect_CredentialSessionsLimit(t *testing.T) {
	limit := 1
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
				{ID: 111, EndpointID: 11, Status: 1, LimitSessions: &limit},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	route1 := s.RouteKeyHash("session-a")
	s.state.SetBinding(10, route1, Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   111,
	}, time.Now().Add(10*time.Minute))

	route2 := s.RouteKeyHash("session-b")
	sel, err := s.Select(context.Background(), 20, route2)
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2 due to sessions limit, got=%d", sel.ChannelID)
	}
}

func TestSelect_CredentialTPMLimit(t *testing.T) {
	limit := 10
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
				{ID: 111, EndpointID: 11, Status: 1, LimitTPM: &limit},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	key := Selection{CredentialType: CredentialTypeOpenAI, CredentialID: 111}.CredentialKey()
	s.state.RecordTokens(key, time.Now(), limit)

	sel, err := s.Select(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2 due to tpm limit, got=%d", sel.ChannelID)
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
