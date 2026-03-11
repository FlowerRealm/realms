package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"realms/internal/store"
)

type fakeGroupStore struct {
	groupsByID   map[int64]store.ChannelGroup
	groupsByName map[string]store.ChannelGroup
	members      map[int64][]store.ChannelGroupMemberDetail
}

type fakeUpstreamStore struct {
	channels  []store.UpstreamChannel
	endpoints map[int64][]store.UpstreamEndpoint
	creds     map[int64][]store.OpenAICompatibleCredential
}

func (f *fakeUpstreamStore) ListUpstreamChannels(_ context.Context) ([]store.UpstreamChannel, error) {
	return f.channels, nil
}

func (f *fakeUpstreamStore) ListUpstreamEndpointsByChannel(_ context.Context, channelID int64) ([]store.UpstreamEndpoint, error) {
	return f.endpoints[channelID], nil
}

func (f *fakeUpstreamStore) ListOpenAICompatibleCredentialsByEndpoint(_ context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error) {
	return f.creds[endpointID], nil
}

func (f *fakeUpstreamStore) ListAnthropicCredentialsByEndpoint(_ context.Context, _ int64) ([]store.AnthropicCredential, error) {
	return nil, nil
}

func (f *fakeUpstreamStore) ListCodexOAuthAccountsByEndpoint(_ context.Context, _ int64) ([]store.CodexOAuthAccount, error) {
	return nil, nil
}

func (f *fakeGroupStore) GetChannelGroupByName(_ context.Context, name string) (store.ChannelGroup, error) {
	g, ok := f.groupsByName[name]
	if !ok {
		return store.ChannelGroup{}, sql.ErrNoRows
	}
	return g, nil
}

func (f *fakeGroupStore) GetChannelGroupByID(_ context.Context, id int64) (store.ChannelGroup, error) {
	g, ok := f.groupsByID[id]
	if !ok {
		return store.ChannelGroup{}, sql.ErrNoRows
	}
	return g, nil
}

func (f *fakeGroupStore) ListChannelGroupMembers(_ context.Context, parentGroupID int64) ([]store.ChannelGroupMemberDetail, error) {
	return f.members[parentGroupID], nil
}

func ptrString(v string) *string {
	return &v
}

func TestGroupRouterNext_FallbackWithoutChannelGroups(t *testing.T) {
	fs := &fakeUpstreamStore{
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
				{ID: 111, EndpointID: 11, Status: 1},
			},
		},
	}
	s := New(fs)

	router := NewGroupRouter(nil, s, 10, "", Constraints{})
	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 1 {
		t.Fatalf("expected channel=1, got=%d", sel.ChannelID)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func TestGroupRouter_Next_AllowsOneRetryThenSwitchesChannel(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Groups: "g0"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
				{ID: 202, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{
		ID:        1,
		Name:      "g0",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{
					MemberID:            1,
					ParentGroupID:       1,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            100,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
				{
					MemberID:            2,
					ParentGroupID:       1,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	cons := Constraints{
		AllowGroups: map[string]struct{}{g0.Name: {}},
		AllowGroupOrder: []string{
			g0.Name,
		},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 2 {
		t.Fatalf("expected first to pick higher priority channel=2, got=%d", first.ChannelID)
	}

	second, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if second.ChannelID != 2 {
		t.Fatalf("expected second to retry the same channel=2, got=%d", second.ChannelID)
	}

	third, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if third.ChannelID != 1 {
		t.Fatalf("expected third to switch to next channel=1, got=%d", third.ChannelID)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverOnlyMovesForward(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 3, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
			2: {{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1}},
			3: {{ID: 31, ChannelID: 3, BaseURL: "https://c.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
			21: {{ID: 201, EndpointID: 21, Status: 1}},
			31: {{ID: 301, EndpointID: 31, Status: 1}},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(3), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 300, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 2, ParentGroupID: 1, MemberChannelID: ptrInt64(2), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 200, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 3, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
	})

	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("first Next err: %v", err)
	}
	if first.ChannelID != 3 {
		t.Fatalf("expected first sequential channel=3, got=%d", first.ChannelID)
	}
	s.Report(first, Result{Success: false, Retriable: true, StatusCode: 429, ErrorClass: "upstream_throttled"})

	second, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("second Next err: %v", err)
	}
	if second.ChannelID != 2 {
		t.Fatalf("expected second sequential channel=2, got=%d", second.ChannelID)
	}
	s.Report(second, Result{Success: false, Retriable: true, StatusCode: 429, ErrorClass: "upstream_throttled"})

	third, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("third Next err: %v", err)
	}
	if third.ChannelID != 1 {
		t.Fatalf("expected third sequential channel=1, got=%d", third.ChannelID)
	}
	s.Report(third, Result{Success: false, Retriable: true, StatusCode: 429, ErrorClass: "upstream_throttled"})

	if _, err := router.Next(context.Background()); err == nil || err.Error() != "上游不可用" {
		t.Fatalf("expected upstream unavailable after exhausting ordered channels, got=%v", err)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverResumesFromBoundChannelWithoutWrap(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 3, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
			2: {{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1}},
			3: {{ID: 31, ChannelID: 3, BaseURL: "https://c.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
			21: {{ID: 201, EndpointID: 21, Status: 1}},
			31: {{ID: 301, EndpointID: 31, Status: 1}},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(3), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 300, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 2, ParentGroupID: 1, MemberChannelID: ptrInt64(2), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 200, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 3, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
		StartChannelID:            2,
	})

	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("first Next err: %v", err)
	}
	if first.ChannelID != 2 {
		t.Fatalf("expected first sequential resume channel=2, got=%d", first.ChannelID)
	}
	s.Report(first, Result{Success: false, Retriable: true, StatusCode: 429, ErrorClass: "upstream_throttled"})

	second, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("second Next err: %v", err)
	}
	if second.ChannelID != 1 {
		t.Fatalf("expected second sequential resume channel=1, got=%d", second.ChannelID)
	}
	s.Report(second, Result{Success: false, Retriable: true, StatusCode: 429, ErrorClass: "upstream_throttled"})

	if _, err := router.Next(context.Background()); err == nil || err.Error() != "上游不可用" {
		t.Fatalf("expected no wrap back to channel 3, got err=%v", err)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverSkipsExcludedBoundChannel(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
			2: {{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
			21: {{ID: 201, EndpointID: 21, Status: 1}},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(2), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 200, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 2, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
	})
	router.ExcludeChannel(1)

	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected excluded start channel to be skipped and select channel=2, got=%d", sel.ChannelID)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverIgnoresGroupPointer(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
			2: {{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
			21: {{ID: 201, EndpointID: 21, Status: 1}},
		},
	}
	s := New(fs)
	ps := &fakeGroupPointerStore{
		recs: map[int64]store.ChannelGroupPointer{
			1: {GroupID: 1, ChannelID: 1, Pinned: true},
		},
	}
	s.SetGroupPointerStore(ps)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(2), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 200, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 2, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
	})

	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected sequential failover to ignore pinned pointer and pick channel=2, got=%d", sel.ChannelID)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverSortsMembersByPriority(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
			2: {{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
			21: {{ID: 201, EndpointID: 21, Status: 1}},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{MemberID: 2, ParentGroupID: 1, MemberChannelID: ptrInt64(2), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 200, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
	})

	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected priority-sorted sequential channel=2, got=%d", sel.ChannelID)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverMissingStartReturnsExplicitError(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
		StartChannelID:            99,
	})

	if _, err := router.nextFromOrderedGroupsSequential(context.Background()); !errors.Is(err, ErrSequentialStartMissing) {
		t.Fatalf("expected ErrSequentialStartMissing, got=%v", err)
	}
}

func TestGroupRouter_Next_SequentialChannelFailoverFallsBackToNearestUnban(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g0"},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
			2: {{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 101, EndpointID: 11, Status: 1}},
			21: {{ID: 201, EndpointID: 21, Status: 1}},
		},
	}
	s := New(fs)

	now := time.Now()
	s.state.BanChannelImmediate(1, now, 20*time.Second)
	s.state.BanChannelImmediate(2, now, 10*time.Second)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: now, UpdatedAt: now}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{1: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			1: {
				{MemberID: 1, ParentGroupID: 1, MemberChannelID: ptrInt64(1), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 200, CreatedAt: now, UpdatedAt: now},
				{MemberID: 2, ParentGroupID: 1, MemberChannelID: ptrInt64(2), MemberChannelType: ptrString(store.UpstreamTypeOpenAICompatible), MemberChannelGroups: ptrString(g0.Name), Priority: 100, CreatedAt: now, UpdatedAt: now},
			},
		},
	}

	router := NewGroupRouter(gs, s, 10, "", Constraints{
		AllowGroups:               map[string]struct{}{g0.Name: {}},
		AllowGroupOrder:           []string{g0.Name},
		SequentialChannelFailover: true,
	})

	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected nearest-unban channel=2, got=%d", sel.ChannelID)
	}
}

func TestGroupRouter_Next_OrderedGroupsHonorsPriorityAndSetsRouteGroup(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g1"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g2"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	root := store.ChannelGroup{
		ID:        1,
		Name:      store.DefaultGroupName,
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	g1 := store.ChannelGroup{
		ID:        2,
		Name:      "g1",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	g2 := store.ChannelGroup{
		ID:        3,
		Name:      "g2",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	gs := &fakeGroupStore{
		groupsByID: map[int64]store.ChannelGroup{
			root.ID: root,
			g1.ID:   g1,
			g2.ID:   g2,
		},
		groupsByName: map[string]store.ChannelGroup{
			root.Name: root,
			g1.Name:   g1,
			g2.Name:   g2,
		},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g1.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g1.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString("g1"),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
			g2.ID: {
				{
					MemberID:            2,
					ParentGroupID:       g2.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString("g2"),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	cons := Constraints{
		AllowGroups: map[string]struct{}{
			"g1": {},
			"g2": {},
		},
		AllowGroupOrder: []string{"g1", "g2"},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)

	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 1 {
		t.Fatalf("expected first to pick group g1 channel=1, got=%d", first.ChannelID)
	}
	if first.RouteGroup != "g1" {
		t.Fatalf("expected first route_group=g1, got=%q", first.RouteGroup)
	}

	// 模拟 g1 变为不可用（例如渠道被 ban / 所有 key 冷却），此时应按 AllowGroupOrder failover 到 g2。
	s.state.BanChannelImmediate(1, time.Now(), time.Minute)

	second, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if second.ChannelID != 2 {
		t.Fatalf("expected second to failover to group g2 channel=2, got=%d", second.ChannelID)
	}
	if second.RouteGroup != "g2" {
		t.Fatalf("expected second route_group=g2, got=%q", second.RouteGroup)
	}
}

func TestGroupRouter_Next_SingleGroupAllBannedFallsBackToNearestUnban(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{g0.ID: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g0.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
				{
					MemberID:            2,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	now := time.Now()
	s.state.channelBanUntil[1] = now.Add(3 * time.Minute)
	s.state.channelBanUntil[2] = now.Add(30 * time.Second)

	cons := Constraints{
		AllowGroups:     map[string]struct{}{g0.Name: {}},
		AllowGroupOrder: []string{g0.Name},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected fallback to pick nearest-unban channel=2, got=%d", sel.ChannelID)
	}
	if sel.RouteGroup != g0.Name {
		t.Fatalf("expected route_group=%q, got=%q", g0.Name, sel.RouteGroup)
	}
}

func TestGroupRouter_Next_AllGroupsBannedFallsBackToNearestUnbanAcrossGroups(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g1"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g2"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)

	g1 := store.ChannelGroup{ID: 1, Name: "g1", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	g2 := store.ChannelGroup{ID: 2, Name: "g2", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID: map[int64]store.ChannelGroup{
			g1.ID: g1,
			g2.ID: g2,
		},
		groupsByName: map[string]store.ChannelGroup{
			g1.Name: g1,
			g2.Name: g2,
		},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g1.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g1.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g1.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
			g2.ID: {
				{
					MemberID:            2,
					ParentGroupID:       g2.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g2.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	now := time.Now()
	s.state.channelBanUntil[1] = now.Add(5 * time.Minute)
	s.state.channelBanUntil[2] = now.Add(10 * time.Second)

	cons := Constraints{
		AllowGroups: map[string]struct{}{
			g1.Name: {},
			g2.Name: {},
		},
		AllowGroupOrder: []string{g1.Name, g2.Name},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	sel, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if sel.ChannelID != 2 {
		t.Fatalf("expected fallback to pick nearest-unban channel=2 across groups, got=%d", sel.ChannelID)
	}
	if sel.RouteGroup != g2.Name {
		t.Fatalf("expected route_group=%q, got=%q", g2.Name, sel.RouteGroup)
	}
}

func TestGroupRouter_Next_GroupPointerPinnedOverridesPriorityWithinGroup(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Groups: "g0"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	g0 := store.ChannelGroup{
		ID:        1,
		Name:      "g0",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{g0.ID: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g0.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            100,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
				{
					MemberID:            2,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	// 指针模式：从 group 指针位置开始遍历，不受 priority 影响。
	s.setChannelGroupPointer(g0.ID, 1, true, "manual")

	cons := Constraints{
		AllowGroups:     map[string]struct{}{g0.Name: {}},
		AllowGroupOrder: []string{g0.Name},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 1 {
		t.Fatalf("expected pointer to pick channel=1, got=%d", first.ChannelID)
	}
	if first.RouteGroup != "g0" {
		t.Fatalf("expected route_group=g0, got=%q", first.RouteGroup)
	}
}

func TestGroupRouter_Next_GroupPointerDoesNotBypassAllowGroupOrder(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g1"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g2"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	g1 := store.ChannelGroup{ID: 1, Name: "g1", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	g2 := store.ChannelGroup{ID: 2, Name: "g2", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID: map[int64]store.ChannelGroup{
			g1.ID: g1,
			g2.ID: g2,
		},
		groupsByName: map[string]store.ChannelGroup{
			g1.Name: g1,
			g2.Name: g2,
		},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g1.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g1.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g1.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
			g2.ID: {
				{
					MemberID:            2,
					ParentGroupID:       g2.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g2.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	// 即使 g2 开启指针模式，也不应绕过 AllowGroupOrder：应先尝试 g1。
	s.setChannelGroupPointer(g2.ID, 2, true, "manual")

	cons := Constraints{
		AllowGroups: map[string]struct{}{
			"g1": {},
			"g2": {},
		},
		AllowGroupOrder: []string{"g1", "g2"},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 1 || first.RouteGroup != "g1" {
		t.Fatalf("expected first to pick g1 channel=1, got=%+v", first)
	}
}

func TestGroupRouter_Next_GroupPointerRotatesOnBan(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{g0.ID: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g0.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
				{
					MemberID:            2,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	s.setChannelGroupPointer(g0.ID, 1, true, "manual")
	now := time.Now()
	s.state.BanChannelImmediate(1, now, 10*time.Second)

	cons := Constraints{
		AllowGroups:     map[string]struct{}{g0.Name: {}},
		AllowGroupOrder: []string{g0.Name},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 2 {
		t.Fatalf("expected pointer to rotate to channel=2 after ban, got=%d", first.ChannelID)
	}

	_, ok, rec := ps.snapshot(g0.ID)
	if !ok {
		t.Fatalf("expected pointer record to exist")
	}
	if rec.ChannelID != 2 {
		t.Fatalf("expected pointer to persist channel_id=2, got=%+v", rec)
	}
	if rec.Reason != "ban" {
		t.Fatalf("expected reason=ban, got %q", rec.Reason)
	}
}

func TestGroupRouter_Next_GroupPointerInvalidFallsBackToRingStart(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 0, Groups: "g0"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Priority: 100, Groups: "g0"},
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	g0 := store.ChannelGroup{ID: 1, Name: "g0", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	gs := &fakeGroupStore{
		groupsByID:   map[int64]store.ChannelGroup{g0.ID: g0},
		groupsByName: map[string]store.ChannelGroup{g0.Name: g0},
		members: map[int64][]store.ChannelGroupMemberDetail{
			g0.ID: {
				{
					MemberID:            1,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(2),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            100,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
				{
					MemberID:            2,
					ParentGroupID:       g0.ID,
					MemberChannelID:     ptrInt64(1),
					MemberChannelType:   ptrString(store.UpstreamTypeOpenAICompatible),
					MemberChannelGroups: ptrString(g0.Name),
					Priority:            0,
					Promotion:           false,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				},
			},
		},
	}

	s.setChannelGroupPointer(g0.ID, 999, true, "manual")

	cons := Constraints{
		AllowGroups:     map[string]struct{}{g0.Name: {}},
		AllowGroupOrder: []string{g0.Name},
	}
	router := NewGroupRouter(gs, s, 10, "", cons)
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	// ring 起点由 promotion/priority/id 决定，此处应回退到 channel=2。
	if first.ChannelID != 2 {
		t.Fatalf("expected fallback to ring start channel=2, got=%d", first.ChannelID)
	}

	_, ok, rec := ps.snapshot(g0.ID)
	if !ok {
		t.Fatalf("expected pointer record to exist")
	}
	if rec.ChannelID != 2 {
		t.Fatalf("expected pointer to be corrected to channel_id=2, got %+v", rec)
	}
	if rec.Reason != "invalid" {
		t.Fatalf("expected reason=invalid, got %q", rec.Reason)
	}
}
