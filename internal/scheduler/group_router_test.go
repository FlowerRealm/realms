package scheduler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"realms/internal/store"
)

type fakeGroupStore struct {
	groupsByID   map[int64]store.ChannelGroup
	groupsByName map[string]store.ChannelGroup
	members      map[int64][]store.ChannelGroupMemberDetail
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
		ID:          1,
		Name:        "g0",
		MaxAttempts: 10,
		Status:      1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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
		ID:          1,
		Name:        store.DefaultGroupName,
		MaxAttempts: 10,
		Status:      1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	g1 := store.ChannelGroup{
		ID:          2,
		Name:        "g1",
		MaxAttempts: 1,
		Status:      1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	g2 := store.ChannelGroup{
		ID:          3,
		Name:        "g2",
		MaxAttempts: 1,
		Status:      1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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

func TestGroupRouter_Next_PinnedRingAdvancesOnBanAndWraps(t *testing.T) {
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	s.PinChannel(2)

	gs := &fakeGroupStore{}

	router := NewGroupRouter(gs, s, 10, "", Constraints{})
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 2 {
		t.Fatalf("expected first to start from pinned channel=2, got=%d", first.ChannelID)
	}

	// 触发 ban（立即封禁），指针应自动轮到下一个（并回绕到 channel=1）。
	s.Report(first, Result{Success: false, Retriable: true, ErrorClass: "network"})

	second, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if second.ChannelID != 1 {
		t.Fatalf("expected pinned pointer to wrap to channel=1 after ban, got=%d", second.ChannelID)
	}
}

func TestGroupRouter_Next_PinnedChannelStillTakesEffect(t *testing.T) {
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
				{ID: 101, EndpointID: 11, Status: 1},
			},
			21: {
				{ID: 201, EndpointID: 21, Status: 1},
			},
		},
	}
	s := New(fs)
	s.PinChannel(2)

	gs := &fakeGroupStore{}

	router := NewGroupRouter(gs, s, 10, "", Constraints{})
	first, err := router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next err: %v", err)
	}
	if first.ChannelID != 2 {
		t.Fatalf("expected pinned channel=2 even if not in default ring, got=%d", first.ChannelID)
	}
}
