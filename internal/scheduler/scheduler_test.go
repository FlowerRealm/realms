package scheduler

import (
	"context"
	"errors"
	"fmt"
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
	touchedCodex   []int64
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

func (f *fakeStore) TouchCodexOAuthAccount(_ context.Context, accountID int64) {
	f.touchedCodex = append(f.touchedCodex, accountID)
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
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2, got=%d", sel.ChannelID)
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
	sel, err := s.SelectWithConstraints(context.Background(), 99, "", Constraints{})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 1 {
		t.Fatalf("expected channel=1 due to affinity, got=%d", sel.ChannelID)
	}
}

func TestSelect_RouteKeyHashEnablesStickyChannelAndCredential(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0},
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
				{ID: 112, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 211, EndpointID: 21, Status: 1},
				{ID: 212, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	routeKeyHash := s.RouteKeyHash("session_abc")

	expectedChannel := int64(1)
	if rendezvousScore64(routeKeyHash, "channel", 2) > rendezvousScore64(routeKeyHash, "channel", 1) {
		expectedChannel = 2
	}

	expectedCred := int64(0)
	if expectedChannel == 1 {
		expectedCred = 111
		if rendezvousScore64(routeKeyHash, string(CredentialTypeOpenAI), 112) > rendezvousScore64(routeKeyHash, string(CredentialTypeOpenAI), 111) {
			expectedCred = 112
		}
	} else {
		expectedCred = 211
		if rendezvousScore64(routeKeyHash, string(CredentialTypeOpenAI), 212) > rendezvousScore64(routeKeyHash, string(CredentialTypeOpenAI), 211) {
			expectedCred = 212
		}
	}

	first, err := s.SelectWithConstraints(context.Background(), 10, routeKeyHash, Constraints{})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if first.ChannelID != expectedChannel {
		t.Fatalf("expected channel=%d, got=%d", expectedChannel, first.ChannelID)
	}
	if first.CredentialID != expectedCred {
		t.Fatalf("expected credential=%d, got=%d", expectedCred, first.CredentialID)
	}

	second, err := s.SelectWithConstraints(context.Background(), 10, routeKeyHash, Constraints{})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if second.ChannelID != first.ChannelID || second.CredentialID != first.CredentialID {
		t.Fatalf("expected sticky selection, first=%d/%d second=%d/%d", first.ChannelID, first.CredentialID, second.ChannelID, second.CredentialID)
	}
}

func TestSelectWithConstraints_RequireCredentialKey_PicksExactCredential(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
				{ID: 222, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)
	wantKey := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, 222)
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{
		RequireChannelID:     1,
		RequireCredentialKey: wantKey,
	})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 1 {
		t.Fatalf("expected channel=1, got=%d", sel.ChannelID)
	}
	if sel.CredentialKey() != wantKey {
		t.Fatalf("expected credential=%q, got=%q", wantKey, sel.CredentialKey())
	}
}

func TestSelectWithConstraints_RequireCredentialKey_CoolingFailsSelection(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
				{ID: 222, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)
	key := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, 222)
	s.state.SetCredentialCooling(key, time.Now().Add(10*time.Minute))
	if _, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{
		RequireChannelID:     1,
		RequireCredentialKey: key,
	}); !errors.Is(err, ErrRequiredCredentialUnavailable) {
		t.Fatalf("expected ErrRequiredCredentialUnavailable when required credential is cooling, got=%v", err)
	}
}

func TestSelectWithConstraints_RequireChannelID_StatusFilteredReturnsConstrainedUnavailable(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 0},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)
	if _, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{
		RequireChannelID: 1,
	}); !errors.Is(err, ErrRequiredChannelUnavailable) {
		t.Fatalf("expected ErrRequiredChannelUnavailable when required channel is filtered out, got=%v", err)
	}
}

func TestSelectWithConstraints_RequireCredentialKey_AllowsEndpointInCooldown(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 222, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)
	s.state.SetEndpointCooling(11, time.Now().Add(10*time.Minute))
	wantKey := fmt.Sprintf("%s:%d", CredentialTypeOpenAI, 222)

	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{
		RequireChannelID:     1,
		RequireCredentialKey: wantKey,
	})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.EndpointID != 11 {
		t.Fatalf("expected endpoint=11, got=%d", sel.EndpointID)
	}
	if sel.CredentialKey() != wantKey {
		t.Fatalf("expected credential=%q, got=%q", wantKey, sel.CredentialKey())
	}
}

func TestSelect_PrefersHealthyEndpointWithinChannel(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1, Priority: 100},
				{ID: 12, ChannelID: 1, BaseURL: "https://b.example", Status: 1, Priority: 10},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			12: {
				{ID: 121, EndpointID: 12, Status: 1},
			},
		},
	}
	s := New(fs)
	s.Report(Selection{
		ChannelID:      1,
		EndpointID:     11,
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   111,
	}, Result{
		Success:    false,
		Retriable:  true,
		ErrorClass: "network",
		Scope:      FailureScopeEndpoint,
	})

	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{
		RequireChannelID: 1,
	})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.EndpointID != 12 {
		t.Fatalf("expected endpoint=12 after endpoint cooldown, got=%d", sel.EndpointID)
	}
}

func TestReport_CredentialScopedFailureSkipsEndpointAndChannelPenalty(t *testing.T) {
	s := New(&fakeStore{})
	s.cooldownBase = 200 * time.Millisecond

	sel := Selection{
		ChannelID:      7,
		EndpointID:     17,
		CredentialType: CredentialTypeCodex,
		CredentialID:   99,
		AutoBan:        true,
	}
	s.Report(sel, Result{
		Success:    false,
		Retriable:  true,
		StatusCode: http.StatusTooManyRequests,
		ErrorClass: "upstream_exhausted",
		Scope:      FailureScopeCredential,
	})

	s.state.mu.Lock()
	credCooling, credOK := s.state.credentialCooldown[sel.CredentialKey()]
	_, endpointCooling := s.state.endpointCooldown[sel.EndpointID]
	channelFails := s.state.channelFails[sel.ChannelID]
	_, channelBanned := s.state.channelBanUntil[sel.ChannelID]
	s.state.mu.Unlock()

	if !credOK || credCooling.IsZero() {
		t.Fatalf("expected credential cooldown to be set")
	}
	if endpointCooling {
		t.Fatalf("expected endpoint cooldown to be skipped for credential failure")
	}
	if channelFails != 0 {
		t.Fatalf("expected channel fail score to stay 0, got=%d", channelFails)
	}
	if channelBanned {
		t.Fatalf("expected channel ban to be skipped for credential failure")
	}
}

func TestReport_RequestScopedFailureSkipsCredentialPenalty(t *testing.T) {
	s := New(&fakeStore{})
	sel := Selection{
		ChannelID:      7,
		EndpointID:     17,
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   99,
	}
	s.Report(sel, Result{
		Success:    false,
		Retriable:  false,
		StatusCode: http.StatusBadRequest,
		ErrorClass: "upstream_status",
		Scope:      FailureScopeRequest,
	})

	s.state.mu.Lock()
	credFails := s.state.credFails[sel.CredentialKey()]
	channelFails := s.state.channelFails[sel.ChannelID]
	s.state.mu.Unlock()

	if credFails != 0 {
		t.Fatalf("expected request-scoped failure not to increment credential fails, got=%d", credFails)
	}
	if channelFails != 0 {
		t.Fatalf("expected request-scoped failure not to increment channel fails, got=%d", channelFails)
	}
}

func TestReport_ChannelModelScopedFailureBansBindingOnly(t *testing.T) {
	s := New(&fakeStore{})
	s.cooldownBase = 200 * time.Millisecond

	sel := Selection{
		ChannelID:      7,
		EndpointID:     17,
		CredentialType: CredentialTypeOpenAI,
		CredentialID:   99,
		AutoBan:        true,
	}
	s.Report(sel, Result{
		Success:               false,
		Retriable:             true,
		StatusCode:            http.StatusNotFound,
		ErrorClass:            "upstream_model_unavailable",
		Scope:                 FailureScopeChannelModel,
		ChannelModelBindingID: 701,
	})

	s.state.mu.Lock()
	credFails := s.state.credFails[sel.CredentialKey()]
	_, credCooling := s.state.credentialCooldown[sel.CredentialKey()]
	_, endpointCooling := s.state.endpointCooldown[sel.EndpointID]
	channelFails := s.state.channelFails[sel.ChannelID]
	_, channelBanned := s.state.channelBanUntil[sel.ChannelID]
	modelFails := s.state.channelModelFails[701]
	modelBanUntil, modelBanned := s.state.channelModelBanUntil[701]
	s.state.mu.Unlock()

	if credFails != 0 {
		t.Fatalf("expected credential fail score to stay 0, got=%d", credFails)
	}
	if credCooling {
		t.Fatalf("expected credential cooldown to be skipped for channel-model failure")
	}
	if endpointCooling {
		t.Fatalf("expected endpoint cooldown to be skipped for channel-model failure")
	}
	if channelFails != 0 {
		t.Fatalf("expected channel fail score to stay 0, got=%d", channelFails)
	}
	if channelBanned {
		t.Fatalf("expected channel ban to be skipped for channel-model failure")
	}
	if modelFails != 1 {
		t.Fatalf("expected model fail score=1, got=%d", modelFails)
	}
	if !modelBanned || modelBanUntil.IsZero() {
		t.Fatalf("expected model binding to be banned")
	}
}

func TestSelectWithConstraints_SkipsBannedChannelModelBinding(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 10},
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
	s.state.BanChannelModel(101, time.Now(), time.Minute, time.Time{})

	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{
		AllowChannelIDs:        map[int64]struct{}{1: {}, 2: {}},
		ChannelModelBindingIDs: map[int64]int64{1: 101, 2: 202},
	})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected channel=2 after channel-model ban, got=%d", sel.ChannelID)
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

func TestRuntimeChannelStats_ExpiredBanMarksProbeDue(t *testing.T) {
	s := New(&fakeStore{})
	now := time.Now()

	s.state.mu.Lock()
	s.state.channelBanUntil[1] = now.Add(-1 * time.Second)
	s.state.mu.Unlock()

	rt := s.RuntimeChannelStats(1)
	if rt.BannedUntil != nil {
		t.Fatalf("expected expired ban to be cleared from runtime view")
	}
	if !s.state.IsChannelProbeDue(1) {
		t.Fatalf("expected runtime stats sweep to mark probe due")
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

	first, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if first.ChannelID != 1 {
		t.Fatalf("expected probe channel=1 to be selected first, got=%d", first.ChannelID)
	}

	second, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{})
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

	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{})
	if err != nil {
		t.Fatalf("Select err: %v", err)
	}
	if sel.CredentialID != 1 {
		t.Fatalf("expected credential=1 (rpm lower), got=%d", sel.CredentialID)
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

func TestReport_UpstreamExhaustedUsesOverrideAndSkipsChannelBan(t *testing.T) {
	s := New(&fakeStore{})
	s.cooldownBase = 200 * time.Millisecond

	sel := Selection{
		ChannelID:      7,
		CredentialType: CredentialTypeCodex,
		CredentialID:   99,
		AutoBan:        true,
	}
	overrideUntil := time.Now().Add(3 * time.Minute)
	s.Report(sel, Result{
		Success:       false,
		Retriable:     true,
		StatusCode:    http.StatusTooManyRequests,
		ErrorClass:    "upstream_exhausted",
		CooldownUntil: &overrideUntil,
	})

	s.state.mu.Lock()
	cooldownUntil, ok := s.state.credentialCooldown[sel.CredentialKey()]
	_, banned := s.state.channelBanUntil[sel.ChannelID]
	s.state.mu.Unlock()
	if !ok {
		t.Fatalf("expected credential cooldown to be set")
	}
	if cooldownUntil.Before(overrideUntil.Add(-50 * time.Millisecond)) {
		t.Fatalf("expected cooldown override >= %v, got=%v", overrideUntil, cooldownUntil)
	}
	if banned {
		t.Fatalf("expected channel ban to be skipped for upstream_exhausted")
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

func TestSelectWithConstraints_CodexPrefersLeastRecentlyUsed(t *testing.T) {
	recent := time.Now().Add(-1 * time.Minute)
	older := time.Now().Add(-1 * time.Hour)
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			21: {
				{ID: 211, EndpointID: 21, Status: 1, LastUsedAt: &recent},
				{ID: 212, EndpointID: 21, Status: 1, LastUsedAt: &older},
				{ID: 213, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	sel, err := s.SelectWithConstraints(context.Background(), 10, "", Constraints{RequireChannelType: store.UpstreamTypeCodexOAuth})
	if err != nil {
		t.Fatalf("SelectWithConstraints err: %v", err)
	}
	if sel.CredentialType != CredentialTypeCodex {
		t.Fatalf("expected codex credential type, got=%+v", sel)
	}
	if sel.CredentialID != 213 {
		t.Fatalf("expected never-used codex account first, got=%d", sel.CredentialID)
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

func TestReport_CodexSuccessTouchesLastUsed(t *testing.T) {
	fs := &fakeStore{}
	s := New(fs)
	sel := Selection{
		ChannelID:      2,
		ChannelType:    store.UpstreamTypeCodexOAuth,
		CredentialType: CredentialTypeCodex,
		CredentialID:   211,
	}
	s.Report(sel, Result{Success: true})
	if len(fs.touchedCodex) != 1 || fs.touchedCodex[0] != 211 {
		t.Fatalf("expected touched codex account 211, got=%v", fs.touchedCodex)
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
