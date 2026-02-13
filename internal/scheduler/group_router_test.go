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
		ID:          1,
		Name:        "g0",
		MaxAttempts: 10,
		Status:      1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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

	g1 := store.ChannelGroup{ID: 1, Name: "g1", MaxAttempts: 10, Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	g2 := store.ChannelGroup{ID: 2, Name: "g2", MaxAttempts: 10, Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
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

	g0 := store.ChannelGroup{ID: 1, Name: "g0", MaxAttempts: 10, Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
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

	g0 := store.ChannelGroup{ID: 1, Name: "g0", MaxAttempts: 10, Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
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
