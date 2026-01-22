package openai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"
)

type fakeDoer struct {
	calls  []scheduler.Selection
	bodies [][]byte
}

func TestResponses_Stream_ExtractsUsageFromSSE(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {ID: 1, PublicID: "gpt-4.1-mini", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, q, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":true}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected 1 commit call, got=%d", len(q.commitCalls))
	}
	got := q.commitCalls[0]
	if got.InputTokens == nil || *got.InputTokens != 3 {
		t.Fatalf("unexpected input_tokens: %+v", got.InputTokens)
	}
	if got.OutputTokens == nil || *got.OutputTokens != 4 {
		t.Fatalf("unexpected output_tokens: %+v", got.OutputTokens)
	}
	if got.CachedInputTokens == nil || *got.CachedInputTokens != 1 {
		t.Fatalf("unexpected cached_input_tokens: %+v", got.CachedInputTokens)
	}
}

type DoerFunc func(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)

func (f DoerFunc) Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
	return f(ctx, sel, downstream, body)
}

func TestExtractUsageTokens_FindsNestedUsage(t *testing.T) {
	in := []byte(`{"response":{"id":"x","usage":{"input_tokens":7,"output_tokens":9}}}`)
	inTok, outTok, _, _ := extractUsageTokens(in)
	if inTok == nil || *inTok != 7 {
		t.Fatalf("unexpected input_tokens: %+v", inTok)
	}
	if outTok == nil || *outTok != 9 {
		t.Fatalf("unexpected output_tokens: %+v", outTok)
	}
}

func (d *fakeDoer) Do(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
	d.calls = append(d.calls, sel)
	d.bodies = append(d.bodies, body)
	switch sel.CredentialID {
	case 2:
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"upstream down"}}`))),
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	}
}

type failoverOnceDoer struct {
	calls      []scheduler.Selection
	failStatus int
}

func (d *failoverOnceDoer) Do(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
	d.calls = append(d.calls, sel)
	if sel.CredentialID == 2 {
		return &http.Response{
			StatusCode: d.failStatus,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"insufficient"}}`))),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
	}, nil
}

type okDoer struct {
	calls []scheduler.Selection
}

func (d *okDoer) Do(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
	d.calls = append(d.calls, sel)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
	}, nil
}

type statusDoer struct {
	status int
	body   string
}

func (d statusDoer) Do(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
	return &http.Response{
		StatusCode: d.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(d.body))),
	}, nil
}

type fakeStore struct {
	channels       []store.UpstreamChannel
	endpoints      map[int64][]store.UpstreamEndpoint
	creds          map[int64][]store.OpenAICompatibleCredential
	anthropicCreds map[int64][]store.AnthropicCredential
	accounts       map[int64][]store.CodexOAuthAccount
	models         map[string]store.ManagedModel
	bindings       map[string][]store.ChannelModelBinding

	groupByName   map[string]store.ChannelGroup
	groupNameByID map[int64]string
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

func (f *fakeStore) GetEnabledManagedModelByPublicID(_ context.Context, publicID string) (store.ManagedModel, error) {
	m, ok := f.models[publicID]
	if !ok || m.Status != 1 {
		return store.ManagedModel{}, sql.ErrNoRows
	}
	return m, nil
}

func (f *fakeStore) GetManagedModelByPublicID(_ context.Context, publicID string) (store.ManagedModel, error) {
	m, ok := f.models[publicID]
	if !ok {
		return store.ManagedModel{}, sql.ErrNoRows
	}
	return m, nil
}

func (f *fakeStore) ListEnabledManagedModelsWithBindings(_ context.Context) ([]store.ManagedModel, error) {
	out := make([]store.ManagedModel, 0, len(f.models))
	for _, m := range f.models {
		if m.Status != 1 {
			continue
		}
		if len(f.bindings[m.PublicID]) == 0 {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeStore) ListEnabledChannelModelBindingsByPublicID(_ context.Context, publicID string) ([]store.ChannelModelBinding, error) {
	return f.bindings[publicID], nil
}

func (f *fakeStore) ensureGroups() {
	if f.groupByName != nil {
		return
	}
	groupNames := make([]string, 0, 8)
	seen := make(map[string]struct{})
	addGroup := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		groupNames = append(groupNames, name)
	}
	addGroup(store.DefaultGroupName)
	for _, ch := range f.channels {
		raw := strings.TrimSpace(ch.Groups)
		if raw == "" {
			addGroup(store.DefaultGroupName)
			continue
		}
		for _, part := range strings.Split(raw, ",") {
			addGroup(part)
		}
	}

	f.groupByName = make(map[string]store.ChannelGroup, len(groupNames))
	f.groupNameByID = make(map[int64]string, len(groupNames))

	// 固定 default=1，其他组按名称排序，保证测试稳定性。
	defaultID := int64(1)
	f.groupByName[store.DefaultGroupName] = store.ChannelGroup{
		ID:              defaultID,
		Name:            store.DefaultGroupName,
		PriceMultiplier: store.DefaultGroupPriceMultiplier,
		MaxAttempts:     50,
		Status:          1,
	}
	f.groupNameByID[defaultID] = store.DefaultGroupName

	var others []string
	for _, name := range groupNames {
		if name == store.DefaultGroupName {
			continue
		}
		others = append(others, name)
	}
	sort.Strings(others)
	nextID := int64(2)
	for _, name := range others {
		id := nextID
		nextID++
		f.groupByName[name] = store.ChannelGroup{
			ID:              id,
			Name:            name,
			PriceMultiplier: store.DefaultGroupPriceMultiplier,
			MaxAttempts:     50,
			Status:          1,
		}
		f.groupNameByID[id] = name
	}
}

func (f *fakeStore) GetChannelGroupByName(_ context.Context, name string) (store.ChannelGroup, error) {
	f.ensureGroups()
	name = strings.TrimSpace(name)
	g, ok := f.groupByName[name]
	if !ok {
		return store.ChannelGroup{}, sql.ErrNoRows
	}
	return g, nil
}

func (f *fakeStore) GetChannelGroupByID(_ context.Context, id int64) (store.ChannelGroup, error) {
	f.ensureGroups()
	if id == 0 {
		return store.ChannelGroup{}, sql.ErrNoRows
	}
	name, ok := f.groupNameByID[id]
	if !ok {
		return store.ChannelGroup{}, sql.ErrNoRows
	}
	return f.groupByName[name], nil
}

func (f *fakeStore) ListChannelGroupMembers(_ context.Context, parentGroupID int64) ([]store.ChannelGroupMemberDetail, error) {
	f.ensureGroups()
	parentName, ok := f.groupNameByID[parentGroupID]
	if !ok {
		return nil, nil
	}

	hasGroup := func(csv string, want string) bool {
		want = strings.TrimSpace(want)
		if want == "" {
			return false
		}
		csv = strings.TrimSpace(csv)
		if csv == "" {
			return want == store.DefaultGroupName
		}
		for _, part := range strings.Split(csv, ",") {
			if strings.TrimSpace(part) == want {
				return true
			}
		}
		return false
	}

	var out []store.ChannelGroupMemberDetail
	if parentName == store.DefaultGroupName {
		// default 根组：挂载所有现有组（不含 default 自身）
		var ids []int64
		for id, name := range f.groupNameByID {
			if name == store.DefaultGroupName {
				continue
			}
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			name := f.groupNameByID[id]
			g := f.groupByName[name]
			gid := g.ID
			n := name
			status := g.Status
			maxAttempts := g.MaxAttempts
			out = append(out, store.ChannelGroupMemberDetail{
				MemberID:               gid,
				ParentGroupID:          parentGroupID,
				MemberGroupID:          &gid,
				MemberGroupName:        &n,
				MemberGroupStatus:      &status,
				MemberGroupMaxAttempts: &maxAttempts,
			})
		}
		// default 组内可直接挂载 default 渠道
		for _, ch := range f.channels {
			if !hasGroup(ch.Groups, store.DefaultGroupName) {
				continue
			}
			id := ch.ID
			name := ch.Name
			typ := ch.Type
			groups := ch.Groups
			status := ch.Status
			out = append(out, store.ChannelGroupMemberDetail{
				MemberID:            id,
				ParentGroupID:       parentGroupID,
				MemberChannelID:     &id,
				MemberChannelName:   &name,
				MemberChannelType:   &typ,
				MemberChannelGroups: &groups,
				MemberChannelStatus: &status,
			})
		}
		return out, nil
	}

	// 非 default：按“组 -> 渠道”回填
	for _, ch := range f.channels {
		if !hasGroup(ch.Groups, parentName) {
			continue
		}
		id := ch.ID
		name := ch.Name
		typ := ch.Type
		groups := ch.Groups
		status := ch.Status
		out = append(out, store.ChannelGroupMemberDetail{
			MemberID:            id,
			ParentGroupID:       parentGroupID,
			MemberChannelID:     &id,
			MemberChannelName:   &name,
			MemberChannelType:   &typ,
			MemberChannelGroups: &groups,
			MemberChannelStatus: &status,
		})
	}
	return out, nil
}

type fakeQuota struct {
	reserveCalls []quota.ReserveInput
	commitCalls  []quota.CommitInput
	voidCalls    []int64
}

func (q *fakeQuota) Reserve(_ context.Context, in quota.ReserveInput) (quota.ReserveResult, error) {
	q.reserveCalls = append(q.reserveCalls, in)
	return quota.ReserveResult{UsageEventID: 1}, nil
}

func (q *fakeQuota) Commit(_ context.Context, in quota.CommitInput) error {
	q.commitCalls = append(q.commitCalls, in)
	return nil
}

func (q *fakeQuota) Void(_ context.Context, usageEventID int64) error {
	q.voidCalls = append(q.voidCalls, usageEventID)
	return nil
}

type fakeAudit struct{}

func (fakeAudit) InsertAuditEvent(_ context.Context, _ store.AuditEventInput) error { return nil }

type staticFeatures struct {
	fs store.FeatureState
}

func (f staticFeatures) FeatureStateEffective(_ context.Context, _ bool) store.FeatureState {
	return f.fs
}

type recordingAudit struct {
	events []store.AuditEventInput
}

func (a *recordingAudit) InsertAuditEvent(_ context.Context, in store.AuditEventInput) error {
	a.events = append(a.events, in)
	return nil
}

func TestResponses_FailoverCredential(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
				{ID: 2, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {
				ID:       1,
				PublicID: "gpt-4.1-mini",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.calls) < 2 {
		t.Fatalf("expected failover with >=2 calls, got=%d", len(doer.calls))
	}
	if doer.calls[0].CredentialID != 2 || doer.calls[1].CredentialID != 1 {
		t.Fatalf("unexpected call order: %+v", doer.calls)
	}
}

func TestResponses_FailoverCredentialOn402PaymentRequired(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
				{ID: 2, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {
				ID:       1,
				PublicID: "gpt-4.1-mini",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &failoverOnceDoer{failStatus: http.StatusPaymentRequired}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.calls) < 2 {
		t.Fatalf("expected failover with >=2 calls, got=%d", len(doer.calls))
	}
	if doer.calls[0].CredentialID != 2 || doer.calls[1].CredentialID != 1 {
		t.Fatalf("unexpected call order: %+v", doer.calls)
	}
}

func TestResponses_RouteKeyPrefersPromptCacheKeyInBody(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {ID: 1, PublicID: "gpt-4.1-mini", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &okDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false,"prompt_cache_key":"rk_body"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "rk_header")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	if _, ok := sched.GetBinding(p.UserID, sched.RouteKeyHash("rk_body")); !ok {
		t.Fatalf("expected binding for body route key")
	}
	if _, ok := sched.GetBinding(p.UserID, sched.RouteKeyHash("rk_header")); ok {
		t.Fatalf("expected no binding for header route key when body prompt_cache_key exists")
	}
}

func TestResponses_RouteKeyFallsBackToHeader(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {ID: 1, PublicID: "gpt-4.1-mini", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &okDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-RC-Route-Key", "rk_header")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	if _, ok := sched.GetBinding(p.UserID, sched.RouteKeyHash("rk_header")); !ok {
		t.Fatalf("expected binding for header route key when body prompt_cache_key missing")
	}
}

func TestResponses_AuditFailoverDoesNotRecordResponseBody(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
				{ID: 2, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {ID: 1, PublicID: "gpt-4.1-mini", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	audit := &recordingAudit{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, audit, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	hasFailover := false
	for _, ev := range audit.events {
		if ev.Action != "failover" {
			continue
		}
		hasFailover = true
		msg := ""
		if ev.ErrorMessage != nil {
			msg = *ev.ErrorMessage
		}
		if msg == "" {
			t.Fatalf("expected failover audit to include an error_message")
		}
		if msg == "upstream down" || msg == `{"error":{"message":"upstream down"}}` {
			t.Fatalf("audit should not record upstream error body: %q", msg)
		}
	}
	if !hasFailover {
		t.Fatalf("expected at least one failover audit event")
	}
}

func TestResponses_AuditUpstreamErrorDoesNotRecordResponseBody(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {ID: 1, PublicID: "gpt-4.1-mini", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := statusDoer{status: http.StatusBadRequest, body: `{"error":{"message":"secret-body"}}`}
	audit := &recordingAudit{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, audit, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	hasUpstreamError := false
	for _, ev := range audit.events {
		if ev.Action != "upstream_error" {
			continue
		}
		hasUpstreamError = true
		msg := ""
		if ev.ErrorMessage != nil {
			msg = *ev.ErrorMessage
		}
		if msg == "" {
			t.Fatalf("expected upstream_error audit to include an error_message")
		}
		if msg == "secret-body" || msg == `{"error":{"message":"secret-body"}}` {
			t.Fatalf("audit should not record upstream error body: %q", msg)
		}
	}
	if !hasUpstreamError {
		t.Fatalf("expected at least one upstream_error audit event")
	}
}

func TestResponses_QuotaCommitIncludesUpstreamChannelID(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {
				ID:       1,
				PublicID: "gpt-4.1-mini",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, q, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected exactly 1 commit call, got=%d", len(q.commitCalls))
	}
	call := q.commitCalls[0]
	if call.UsageEventID != 1 {
		t.Fatalf("expected usage_event_id=1, got=%d", call.UsageEventID)
	}
	if call.UpstreamChannelID == nil || *call.UpstreamChannelID != 1 {
		t.Fatalf("expected upstream_channel_id=1, got=%v", call.UpstreamChannelID)
	}
	if call.InputTokens == nil || *call.InputTokens != 1 {
		t.Fatalf("expected input_tokens=1, got=%v", call.InputTokens)
	}
	if call.OutputTokens == nil || *call.OutputTokens != 2 {
		t.Fatalf("expected output_tokens=2, got=%v", call.OutputTokens)
	}
}

func TestResponses_GroupConstraintFiltersChannels(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: "g1"},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Promotion: true, Groups: "g2"},
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
		models: map[string]store.ManagedModel{
			"gpt-4.1-mini": {
				ID:       1,
				PublicID: "gpt-4.1-mini",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-4.1-mini": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
				{ID: 2, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-4.1-mini", UpstreamModel: "gpt-4.1-mini", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 1024, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-4.1-mini","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{"default", "g1"},
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.calls) != 1 {
		t.Fatalf("expected exactly 1 upstream call, got=%d", len(doer.calls))
	}
	if doer.calls[0].ChannelID != 1 {
		t.Fatalf("expected channel=1 due to group constraint, got=%+v", doer.calls[0])
	}
}

func TestResponses_ModelNotEnabledRejected(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 0, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"nope","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.calls) != 0 {
		t.Fatalf("unexpected upstream calls: %+v", doer.calls)
	}
}

func TestResponses_ModelPassthrough_AllowsDisabledModelWithoutBindings(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"nope": {ID: 1, PublicID: "nope", Status: 0},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	features := staticFeatures{fs: store.FeatureState{ModelsDisabled: true}}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, features, false, nil, fakeAudit{}, nil, 0, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"nope","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.bodies) == 0 {
		t.Fatalf("expected upstream call")
	}
	var payload map[string]any
	if err := json.Unmarshal(doer.bodies[0], &payload); err != nil {
		t.Fatalf("unmarshal passthrough body: %v", err)
	}
	if payload["model"] != "nope" {
		t.Fatalf("expected passthrough model=nope, got=%v", payload["model"])
	}
}

func TestResponses_AliasRewrite(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"alias": {
				ID:       1,
				PublicID: "alias",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"alias": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "alias", UpstreamModel: "real-model", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 0, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"alias","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.bodies) == 0 {
		t.Fatalf("expected upstream call")
	}
	var payload map[string]any
	if err := json.Unmarshal(doer.bodies[0], &payload); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}
	if payload["model"] != "real-model" {
		t.Fatalf("expected rewritten model=real-model, got=%v", payload["model"])
	}
}

func TestModels_ReturnsManagedModels(t *testing.T) {
	fs := &fakeStore{
		models: map[string]store.ManagedModel{
			"m1": {
				ID:       1,
				PublicID: "m1",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, nil, fakeAudit{}, nil, 0, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Models)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.calls) != 0 {
		t.Fatalf("unexpected upstream calls: %+v", doer.calls)
	}
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Object != "list" || len(resp.Data) != 1 || resp.Data[0].ID != "m1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
