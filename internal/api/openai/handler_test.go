package openai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"realms/internal/auth"
	"realms/internal/concurrency"
	"realms/internal/middleware"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"

	"github.com/tidwall/gjson"
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":4}}}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":true}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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
	if got.InputTokens == nil || *got.InputTokens <= 0 {
		t.Fatalf("unexpected input_tokens: %+v", got.InputTokens)
	}
	if got.OutputTokens == nil || *got.OutputTokens <= 0 {
		t.Fatalf("unexpected output_tokens: %+v", got.OutputTokens)
	}
	if got.CachedInputTokens != nil {
		t.Fatalf("expected cached_input_tokens to be nil, got=%+v", got.CachedInputTokens)
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

func TestClassifyNonRetriableFailureScope(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   scheduler.FailureScope
	}{
		{name: "bad request stays request scoped", status: http.StatusBadRequest, want: scheduler.FailureScopeChannel},
		{name: "unprocessable stays request scoped", status: http.StatusUnprocessableEntity, want: scheduler.FailureScopeChannel},
		{name: "not found is endpoint scoped", status: http.StatusNotFound, want: scheduler.FailureScopeEndpoint},
		{name: "method not allowed is endpoint scoped", status: http.StatusMethodNotAllowed, want: scheduler.FailureScopeEndpoint},
		{name: "internal error is endpoint scoped", status: http.StatusInternalServerError, want: scheduler.FailureScopeEndpoint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyNonRetriableFailureScope(tc.status); got != tc.want {
				t.Fatalf("scope=%q want=%q", got, tc.want)
			}
		})
	}
}

func (d *fakeDoer) Do(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
	d.calls = append(d.calls, sel)
	d.bodies = append(d.bodies, body)
	switch sel.CredentialID {
	case 2:
		return &http.Response{
			// 让调度优先在同渠道内切换到其它 key/账号（不触发 channel 立即封禁）。
			StatusCode: http.StatusPaymentRequired,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"upstream down"}}`))),
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","output_text":"pong","usage":{"input_tokens":1,"output_tokens":2}}`))),
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

type codexCooldownDoer struct {
	calls      []scheduler.Selection
	cooldowns  []int64
	cooldownAt []time.Time
}

func (d *codexCooldownDoer) Do(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
	d.calls = append(d.calls, sel)
	if sel.CredentialID == 1002 {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","code":"usage_limit_reached","resets_at":2000000000}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"ok-from-openai","usage":{"input_tokens":1,"output_tokens":2}}`)),
	}, nil
}

func (d *codexCooldownDoer) SetCodexOAuthAccountCooldown(_ context.Context, accountID int64, until time.Time) error {
	d.cooldowns = append(d.cooldowns, accountID)
	d.cooldownAt = append(d.cooldownAt, until)
	return nil
}

type codexQuotaPatchDoer struct {
	patched chan store.CodexOAuthQuotaPatch
	ids     chan int64
}

func (d *codexQuotaPatchDoer) Do(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("x-codex-primary-used-percent", "80")
	h.Set("x-codex-primary-reset-after-seconds", "1000")
	h.Set("x-codex-primary-window-minutes", "10080")
	h.Set("x-codex-secondary-used-percent", "20")
	h.Set("x-codex-secondary-reset-after-seconds", "200")
	h.Set("x-codex-secondary-window-minutes", "300")

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(`{"id":"ok-from-codex","usage":{"input_tokens":1,"output_tokens":2}}`)),
	}, nil
}

func (d *codexQuotaPatchDoer) PatchCodexOAuthAccountQuota(_ context.Context, accountID int64, patch store.CodexOAuthQuotaPatch, _ time.Time) error {
	if d.ids != nil {
		select {
		case d.ids <- accountID:
		default:
		}
	}
	if d.patched != nil {
		select {
		case d.patched <- patch:
		default:
		}
	}
	return nil
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

	blockGetChannelGroupByName bool
	groupLookupStarted         chan struct{}
	groupLookupStartedOnce     sync.Once

	sessionBindingsMu sync.Mutex
	sessionBindings   map[string]fakeSessionBindingEntry
}

type fakeSessionBindingEntry struct {
	payload   string
	expiresAt time.Time
}

func (f *fakeStore) ListUpstreamChannels(_ context.Context) ([]store.UpstreamChannel, error) {
	out := make([]store.UpstreamChannel, 0, len(f.channels))
	for _, ch := range f.channels {
		if strings.TrimSpace(ch.Groups) == "" {
			ch.Groups = store.DefaultGroupName
		}
		out = append(out, ch)
	}
	return out, nil
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
	if strings.TrimSpace(m.GroupName) == "" {
		m.GroupName = store.DefaultGroupName
	}
	return m, nil
}

func (f *fakeStore) GetManagedModelByPublicID(_ context.Context, publicID string) (store.ManagedModel, error) {
	m, ok := f.models[publicID]
	if !ok {
		return store.ManagedModel{}, sql.ErrNoRows
	}
	if strings.TrimSpace(m.GroupName) == "" {
		m.GroupName = store.DefaultGroupName
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
		if strings.TrimSpace(m.GroupName) == "" {
			m.GroupName = store.DefaultGroupName
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeStore) ListEnabledChannelModelBindingsByPublicID(_ context.Context, publicID string) ([]store.ChannelModelBinding, error) {
	return f.bindings[publicID], nil
}

func (f *fakeStore) GetSessionBindingPayload(_ context.Context, userID int64, routeKeyHash string, now time.Time) (string, bool, error) {
	if userID <= 0 {
		return "", false, nil
	}
	routeKeyHash = strings.TrimSpace(routeKeyHash)
	if routeKeyHash == "" {
		return "", false, nil
	}
	key := fmt.Sprintf("%d|%s", userID, routeKeyHash)
	if now.IsZero() {
		now = time.Now()
	}
	f.sessionBindingsMu.Lock()
	defer f.sessionBindingsMu.Unlock()
	if f.sessionBindings == nil {
		return "", false, nil
	}
	entry, ok := f.sessionBindings[key]
	if !ok {
		return "", false, nil
	}
	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		delete(f.sessionBindings, key)
		return "", false, nil
	}
	return entry.payload, true, nil
}

func (f *fakeStore) UpsertSessionBindingPayload(_ context.Context, userID int64, routeKeyHash string, payload string, expiresAt time.Time) error {
	if userID <= 0 {
		return nil
	}
	routeKeyHash = strings.TrimSpace(routeKeyHash)
	if routeKeyHash == "" {
		return nil
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(30 * time.Minute)
	}
	key := fmt.Sprintf("%d|%s", userID, routeKeyHash)
	f.sessionBindingsMu.Lock()
	defer f.sessionBindingsMu.Unlock()
	if f.sessionBindings == nil {
		f.sessionBindings = make(map[string]fakeSessionBindingEntry)
	}
	f.sessionBindings[key] = fakeSessionBindingEntry{payload: payload, expiresAt: expiresAt}
	return nil
}

func (f *fakeStore) DeleteSessionBinding(_ context.Context, userID int64, routeKeyHash string) error {
	if userID <= 0 {
		return nil
	}
	routeKeyHash = strings.TrimSpace(routeKeyHash)
	if routeKeyHash == "" {
		return nil
	}
	key := fmt.Sprintf("%d|%s", userID, routeKeyHash)
	f.sessionBindingsMu.Lock()
	defer f.sessionBindingsMu.Unlock()
	if f.sessionBindings != nil {
		delete(f.sessionBindings, key)
	}
	return nil
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
			Status:          1,
		}
		f.groupNameByID[id] = name
	}
}

func (f *fakeStore) GetChannelGroupByName(ctx context.Context, name string) (store.ChannelGroup, error) {
	if f.blockGetChannelGroupByName {
		if f.groupLookupStarted != nil {
			f.groupLookupStartedOnce.Do(func() { close(f.groupLookupStarted) })
		}
		<-ctx.Done()
		return store.ChannelGroup{}, ctx.Err()
	}
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
			out = append(out, store.ChannelGroupMemberDetail{
				MemberID:          gid,
				ParentGroupID:     parentGroupID,
				MemberGroupID:     &gid,
				MemberGroupName:   &n,
				MemberGroupStatus: &status,
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
			groups := strings.TrimSpace(ch.Groups)
			if groups == "" {
				groups = store.DefaultGroupName
			}
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
		groups := strings.TrimSpace(ch.Groups)
		if groups == "" {
			groups = store.DefaultGroupName
		}
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

type recordingUsage struct {
	calls []store.FinalizeUsageEventInput
}

func (u *recordingUsage) FinalizeUsageEvent(_ context.Context, in store.FinalizeUsageEventInput) error {
	u.calls = append(u.calls, in)
	return nil
}

type staticFeatures struct {
	fs store.FeatureState
}

func (f staticFeatures) FeatureStateEffective(_ context.Context) store.FeatureState {
	return f.fs
}

type fakeConcurrencyManager struct {
	userErr          error
	credErr          error
	credAcquireCalls int
	credReleaseCalls int
}

func (m *fakeConcurrencyManager) AcquireUserSlotWithWait(_ context.Context, _ int64, _ int, _ func() error) (func(), error) {
	return func() {}, m.userErr
}

func (m *fakeConcurrencyManager) AcquireCredentialSlotWithWait(_ context.Context, _ string, _ int) (func(), error) {
	m.credAcquireCalls++
	return func() {
		m.credReleaseCalls++
	}, m.credErr
}

type fakePassthroughMatcher struct {
	status int
	msg    string
	skip   bool
	match  bool
}

func (m fakePassthroughMatcher) Match(_ string, _ int, _ []byte) (int, string, bool, bool) {
	if !m.match {
		return 0, "", false, false
	}
	return m.status, m.msg, m.skip, true
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
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
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

func TestResponses_RetrySameSelectionOnNetworkErrorBeforeFailover(t *testing.T) {
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
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var calls []scheduler.Selection
	attempt := 0
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls = append(calls, sel)
		attempt++
		if attempt == 1 {
			return nil, errors.New("temporary upstream dial failure")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)
	conc := &fakeConcurrencyManager{}
	h.gateway.credentialMaxConcurrency = 1
	h.SetConcurrencyManager(conc)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 attempts on same channel, got=%d", len(calls))
	}
	if calls[0].ChannelID != 1 || calls[1].ChannelID != 1 {
		t.Fatalf("expected retry to stay on same channel, got calls=%+v", calls)
	}
	if conc.credAcquireCalls != 2 || conc.credReleaseCalls != 2 {
		t.Fatalf("expected credential slot per attempt, got acquires=%d releases=%d", conc.credAcquireCalls, conc.credReleaseCalls)
	}
}

func TestResponses_RetryThenFailoverToSiblingEndpointWithinChannel(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
			12: {
				{ID: 2, EndpointID: 12, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var calls []scheduler.Selection
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls = append(calls, sel)
		if sel.EndpointID == 11 {
			return nil, errors.New("temporary endpoint dial failure")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 attempts (retry same endpoint, then sibling endpoint), got=%d", len(calls))
	}
	if calls[0].EndpointID != 11 || calls[1].EndpointID != 11 {
		t.Fatalf("expected first two attempts on endpoint=11, got=%+v", calls)
	}
	if calls[2].EndpointID != 12 {
		t.Fatalf("expected failover to sibling endpoint=12, got=%d", calls[2].EndpointID)
	}
	for _, call := range calls {
		if call.ChannelID != 1 {
			t.Fatalf("expected all attempts to stay on channel=1, got calls=%+v", calls)
		}
	}
}

func TestResponses_Retry502ThenFailoverToSiblingEndpointWithinChannel(t *testing.T) {
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
				{ID: 1, EndpointID: 11, Status: 1},
			},
			12: {
				{ID: 2, EndpointID: 12, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var calls []scheduler.Selection
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls = append(calls, sel)
		if sel.EndpointID == 11 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"bad gateway"}}`))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 attempts (retry same endpoint on 502, then sibling endpoint), got=%d", len(calls))
	}
	if calls[0].EndpointID != 11 || calls[1].EndpointID != 11 {
		t.Fatalf("expected first two attempts on endpoint=11, got=%+v", calls)
	}
	if calls[2].EndpointID != 12 {
		t.Fatalf("expected failover to sibling endpoint=12, got=%d", calls[2].EndpointID)
	}
}

func TestResponses_ChannelRequestPolicy_IsPerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, DisableStore: true},
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, AllowServiceTier: true, AllowSafetyIdentifier: true},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			21: {
				{ID: 2, EndpointID: 21, Status: 1},
			},
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up2", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1},
			},
		},
	}

	var calls []scheduler.Selection
	var bodies [][]byte
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		calls = append(calls, sel)
		bodies = append(bodies, body)

		if sel.ChannelID == 2 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"upstream down"}}`))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10}, nil)

	reqBody := `{"model":"m1","input":"hi","service_tier":"default","store":true,"safety_identifier":"u123"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 attempts, got=%d", len(calls))
	}

	var first map[string]any
	if err := json.Unmarshal(bodies[0], &first); err != nil {
		t.Fatalf("unmarshal first body: %v", err)
	}
	if first["model"] != "m1-up2" {
		t.Fatalf("unexpected first model: %v", first["model"])
	}
	if _, ok := first["service_tier"]; ok {
		t.Fatalf("expected service_tier to be removed on channel 2")
	}
	if _, ok := first["store"]; ok {
		t.Fatalf("expected store to be removed on channel 2")
	}
	if _, ok := first["safety_identifier"]; ok {
		t.Fatalf("expected safety_identifier to be removed on channel 2")
	}

	var retry map[string]any
	if err := json.Unmarshal(bodies[1], &retry); err != nil {
		t.Fatalf("unmarshal retry body: %v", err)
	}
	if retry["model"] != "m1-up2" {
		t.Fatalf("unexpected retry model: %v", retry["model"])
	}
	if _, ok := retry["service_tier"]; ok {
		t.Fatalf("expected retry service_tier to be removed on channel 2")
	}
	if _, ok := retry["store"]; ok {
		t.Fatalf("expected retry store to be removed on channel 2")
	}
	if _, ok := retry["safety_identifier"]; ok {
		t.Fatalf("expected retry safety_identifier to be removed on channel 2")
	}

	var second map[string]any
	if err := json.Unmarshal(bodies[2], &second); err != nil {
		t.Fatalf("unmarshal second body: %v", err)
	}
	if second["model"] != "m1-up1" {
		t.Fatalf("unexpected second model: %v", second["model"])
	}
	if _, ok := second["service_tier"]; !ok {
		t.Fatalf("expected service_tier to be present on channel 1")
	}
	if _, ok := second["store"]; !ok {
		t.Fatalf("expected store to be present on channel 1")
	}
	if _, ok := second["safety_identifier"]; !ok {
		t.Fatalf("expected safety_identifier to be present on channel 1")
	}
}

func TestResponses_ChannelParamOverride_IsPerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, DisableStore: true, ParamOverride: `{"operations":[{"path":"metadata.channel","mode":"set","value":"b"},{"path":"store","mode":"set","value":true}]}`},
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, ParamOverride: `{"operations":[{"path":"metadata.channel","mode":"set","value":"a"},{"path":"store","mode":"set","value":true}]}`},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			21: {
				{ID: 2, EndpointID: 21, Status: 1},
			},
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up2", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1},
			},
		},
	}

	var calls []scheduler.Selection
	var bodies [][]byte
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		calls = append(calls, sel)
		bodies = append(bodies, body)

		if sel.ChannelID == 2 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"upstream down"}}`))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 attempts, got=%d", len(calls))
	}

	var first map[string]any
	if err := json.Unmarshal(bodies[0], &first); err != nil {
		t.Fatalf("unmarshal first body: %v", err)
	}
	if first["model"] != "m1-up2" {
		t.Fatalf("unexpected first model: %v", first["model"])
	}
	meta1, _ := first["metadata"].(map[string]any)
	if meta1 == nil || meta1["channel"] != "b" {
		t.Fatalf("unexpected first metadata: %+v", first["metadata"])
	}
	if _, ok := first["store"]; !ok {
		t.Fatalf("expected store to be present on channel 2")
	}

	var retry map[string]any
	if err := json.Unmarshal(bodies[1], &retry); err != nil {
		t.Fatalf("unmarshal retry body: %v", err)
	}
	if retry["model"] != "m1-up2" {
		t.Fatalf("unexpected retry model: %v", retry["model"])
	}
	metaRetry, _ := retry["metadata"].(map[string]any)
	if metaRetry == nil || metaRetry["channel"] != "b" {
		t.Fatalf("unexpected retry metadata: %+v", retry["metadata"])
	}
	if _, ok := retry["store"]; !ok {
		t.Fatalf("expected retry store to be present on channel 2")
	}

	var second map[string]any
	if err := json.Unmarshal(bodies[2], &second); err != nil {
		t.Fatalf("unmarshal second body: %v", err)
	}
	if second["model"] != "m1-up1" {
		t.Fatalf("unexpected second model: %v", second["model"])
	}
	meta2, _ := second["metadata"].(map[string]any)
	if meta2 == nil || meta2["channel"] != "a" {
		t.Fatalf("unexpected second metadata: %+v", second["metadata"])
	}
	if _, ok := second["store"]; !ok {
		t.Fatalf("expected store to be present on channel 1")
	}
}

func TestResponses_MaxTokensAlias_PreservesMaxTokens(t *testing.T) {
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up", Status: 1},
			},
		},
	}

	var gotBody []byte
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi","max_tokens":123}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var forwarded map[string]any
	if err := json.Unmarshal(gotBody, &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	if v, ok := forwarded["max_tokens"].(float64); !ok || int64(v) != 123 {
		t.Fatalf("expected max_tokens=123, got=%v", forwarded["max_tokens"])
	}
	if _, ok := forwarded["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be absent, got=%v", forwarded["max_output_tokens"])
	}
	if _, ok := forwarded["max_completion_tokens"]; ok {
		t.Fatalf("expected max_completion_tokens to be absent, got=%v", forwarded["max_completion_tokens"])
	}
}

func TestResponses_ChannelParamOverride_MaxTokens_PreservesMaxTokens(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, ParamOverride: `{"max_tokens":7}`},
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up", Status: 1},
			},
		},
	}

	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			return nil, err
		}
		if _, ok := got["max_completion_tokens"]; ok {
			t.Fatalf("expected max_completion_tokens to be absent, got=%v", got["max_completion_tokens"])
		}
		if v, ok := got["max_tokens"].(float64); !ok || v != 7 {
			t.Fatalf("expected max_tokens=7, got=%v", got["max_tokens"])
		}
		if _, ok := got["max_output_tokens"]; ok {
			t.Fatalf("expected max_output_tokens to be absent, got=%v", got["max_output_tokens"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestResponses_ModelSuffixEffort_IsPerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, ModelSuffixPreserve: `["o1-mini-high"]`},
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			21: {
				{ID: 2, EndpointID: 21, Status: 1},
			},
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "o1-mini-high", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "o1-mini-high", Status: 1},
			},
		},
	}

	var bodies [][]byte
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		bodies = append(bodies, body)
		if sel.ChannelID == 2 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"upstream down"}}`))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(bodies) != 3 {
		t.Fatalf("expected 3 attempts, got=%d", len(bodies))
	}

	var first map[string]any
	if err := json.Unmarshal(bodies[0], &first); err != nil {
		t.Fatalf("unmarshal first body: %v", err)
	}
	if first["model"] != "o1-mini-high" {
		t.Fatalf("expected preserved model=o1-mini-high, got=%v", first["model"])
	}
	if _, ok := first["reasoning"]; ok {
		t.Fatalf("expected reasoning to be absent when preserved, got=%v", first["reasoning"])
	}

	var retry map[string]any
	if err := json.Unmarshal(bodies[1], &retry); err != nil {
		t.Fatalf("unmarshal retry body: %v", err)
	}
	if retry["model"] != "o1-mini-high" {
		t.Fatalf("expected retry preserved model=o1-mini-high, got=%v", retry["model"])
	}
	if _, ok := retry["reasoning"]; ok {
		t.Fatalf("expected retry reasoning to be absent when preserved, got=%v", retry["reasoning"])
	}

	var second map[string]any
	if err := json.Unmarshal(bodies[2], &second); err != nil {
		t.Fatalf("unmarshal second body: %v", err)
	}
	if second["model"] != "o1-mini" {
		t.Fatalf("expected converted model=o1-mini, got=%v", second["model"])
	}
	reasoning, _ := second["reasoning"].(map[string]any)
	if reasoning == nil || reasoning["effort"] != "high" {
		t.Fatalf("expected reasoning.effort=high, got=%v", second["reasoning"])
	}
}

func TestResponses_ChannelBodyFilters_ArePerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, RequestBodyBlacklist: `["metadata.trace","extra"]`},
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, RequestBodyWhitelist: `["model","input"]`},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			21: {
				{ID: 2, EndpointID: 21, Status: 1},
			},
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up2", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1},
			},
		},
	}

	var bodies [][]byte
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		bodies = append(bodies, body)
		if sel.ChannelID == 2 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"upstream down"}}`))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	reqBody := `{"model":"m1","input":"hi","metadata":{"trace":"t","keep":"k"},"extra":"x"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(bodies) != 3 {
		t.Fatalf("expected 3 attempts, got=%d", len(bodies))
	}

	var first map[string]any
	if err := json.Unmarshal(bodies[0], &first); err != nil {
		t.Fatalf("unmarshal first body: %v", err)
	}
	if first["model"] != "m1-up2" {
		t.Fatalf("unexpected first model: %v", first["model"])
	}
	if _, ok := first["extra"]; ok {
		t.Fatalf("expected extra to be removed on channel 2")
	}
	meta1, _ := first["metadata"].(map[string]any)
	if meta1 == nil || meta1["keep"] != "k" {
		t.Fatalf("expected metadata.keep=k on channel 2, got=%v", first["metadata"])
	}
	if _, ok := meta1["trace"]; ok {
		t.Fatalf("expected metadata.trace to be removed on channel 2, got=%v", meta1["trace"])
	}

	var retry map[string]any
	if err := json.Unmarshal(bodies[1], &retry); err != nil {
		t.Fatalf("unmarshal retry body: %v", err)
	}
	if retry["model"] != "m1-up2" {
		t.Fatalf("unexpected retry model: %v", retry["model"])
	}
	if _, ok := retry["extra"]; ok {
		t.Fatalf("expected retry extra to be removed on channel 2")
	}
	metaRetry, _ := retry["metadata"].(map[string]any)
	if metaRetry == nil || metaRetry["keep"] != "k" {
		t.Fatalf("expected retry metadata.keep=k on channel 2, got=%v", retry["metadata"])
	}
	if _, ok := metaRetry["trace"]; ok {
		t.Fatalf("expected retry metadata.trace to be removed on channel 2, got=%v", metaRetry["trace"])
	}

	var second map[string]any
	if err := json.Unmarshal(bodies[2], &second); err != nil {
		t.Fatalf("unmarshal second body: %v", err)
	}
	if second["model"] != "m1-up1" {
		t.Fatalf("unexpected second model: %v", second["model"])
	}
	if _, ok := second["input"]; !ok {
		t.Fatalf("expected input to be present on channel 1")
	}
	if _, ok := second["metadata"]; ok {
		t.Fatalf("expected metadata to be removed by whitelist on channel 1")
	}
	if _, ok := second["extra"]; ok {
		t.Fatalf("expected extra to be removed by whitelist on channel 1")
	}
}

func TestResponses_StatusCodeMapping_OverridesDownstreamStatus(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, StatusCodeMapping: `{"400":"200"}`},
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, statusDoer{status: http.StatusBadRequest, body: `{"error":{"message":"bad request"}}`}, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj == nil || errObj["message"] != "bad request" {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestResponses_UsageEvent_RecordsUpstreamErrorMessage(t *testing.T) {
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	q := &fakeQuota{}
	usage := &recordingUsage{}
	h := NewHandler(fs, fs, sched, statusDoer{status: http.StatusBadRequest, body: `{"detail":"Unsupported parameter: max_tokens"}`}, nil, nil, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi","max_tokens":123}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected quota void, got=%d", len(q.voidCalls))
	}
	if len(usage.calls) != 1 {
		t.Fatalf("expected 1 usage finalization, got=%d", len(usage.calls))
	}
	call := usage.calls[0]
	if call.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected usage status: %d", call.StatusCode)
	}
	if call.ErrorClass == nil || *call.ErrorClass != "upstream_status" {
		t.Fatalf("unexpected usage error_class: %v", call.ErrorClass)
	}
	if call.ErrorMessage == nil || *call.ErrorMessage != "Unsupported parameter: max_tokens" {
		t.Fatalf("unexpected usage error_message: %v", call.ErrorMessage)
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
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &failoverOnceDoer{failStatus: http.StatusPaymentRequired}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &okDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false,"prompt_cache_key":"rk_body"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "rk_header")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Realms-Route-Key-Source"); got != "payload" {
		t.Fatalf("expected route key source to be payload, got=%q", got)
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &okDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-RC-Route-Key", "rk_header")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Realms-Route-Key-Source"); got != "header" {
		t.Fatalf("expected route key source to be header, got=%q", got)
	}
}

func TestResponses_RouteKeyFallsBackToHeaderXSessionID(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &okDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Id", "rk_x_session")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Realms-Route-Key-Source"); got != "header" {
		t.Fatalf("expected route key source to be header, got=%q", got)
	}
}

func TestResponses_RouteKeyFallsBackToBodyMetadataSessionID(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &okDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false,"metadata":{"session_id":"rk_meta"}}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Realms-Route-Key-Source"); got != "payload" {
		t.Fatalf("expected route key source to be payload, got=%q", got)
	}
}

func TestResponses_CodexSessionCompletion_FillsPromptCacheKeyAndSessionHeader(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var reqPayload map[string]any
	var sessionHeader string
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
		_ = json.Unmarshal(body, &reqPayload)
		sessionHeader = downstream.Header.Get("Session_id")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if sessionHeader == "" {
		t.Fatalf("expected Session_id header to be completed")
	}
	promptCacheKey := stringFromAny(reqPayload["prompt_cache_key"])
	if promptCacheKey == "" {
		t.Fatalf("expected prompt_cache_key to be completed")
	}
	if promptCacheKey != sessionHeader {
		t.Fatalf("expected prompt_cache_key and Session_id to be aligned, got body=%q header=%q", promptCacheKey, sessionHeader)
	}
}

func TestResponsesCompact_RemoteRequiresSessionID(t *testing.T) {
	fs := &fakeStore{}
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, nil, &fakeQuota{}, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "session_id is required") {
		t.Fatalf("expected session_id error, got body=%s", rr.Body.String())
	}
}

func TestResponsesCompact_RemoteNotUsingLegacyStickyRouting(t *testing.T) {
	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: store.DefaultGroupName, Status: 1}}}
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, nil, &fakeQuota{}, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","prompt_cache_key":"rk1","session_id":"s1","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 without compact gateway config, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "compact gateway is not configured") {
		t.Fatalf("expected compact gateway config error, got body=%s", rr.Body.String())
	}
}

func TestResponses_CodexSession_ClearStickyOnSelectionChange_KeepsCompaction(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 0},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 22, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			22: {
				{ID: 222, EndpointID: 22, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
				{ID: 2, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var gotCh2Body []byte
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		if sel.ChannelID == 2 {
			gotCh2Body = append([]byte(nil), body...)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req1 := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses",
		bytes.NewReader([]byte(`{"model":"gpt-5.2","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false,"prompt_cache_key":"rk1"}`)))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(auth.WithPrincipal(req1.Context(), p))
	rr1 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr1.Code, rr1.Body.String())
	}

	fs.channels = []store.UpstreamChannel{
		{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 0},
		{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
	}

	req2 := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses",
		bytes.NewReader([]byte(`{"model":"gpt-5.2","input":[{"type":"item_reference","id":"ref1"},{"type":"message","role":"user","content":[{"type":"input_text","text":"next"}]}],"stream":false,"prompt_cache_key":"rk1","previous_response_id":"resp_123","compaction":{"encrypted_content":"mRYQ...71fD"}}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.WithPrincipal(req2.Context(), p))
	rr2 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := rr2.Header().Get("X-Realms-Codex-Sticky-Cleared"); got != "1" {
		t.Fatalf("expected sticky cleared header, got=%q", got)
	}
	if len(gotCh2Body) == 0 {
		t.Fatalf("expected upstream call to channel 2")
	}
	if got := gjson.GetBytes(gotCh2Body, "compaction.encrypted_content").String(); got != "mRYQ...71fD" {
		t.Fatalf("expected compaction to be preserved, got=%q body=%s", got, string(gotCh2Body))
	}
	if got := gjson.GetBytes(gotCh2Body, "previous_response_id").String(); got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved, got=%q body=%s", got, string(gotCh2Body))
	}
	payload, ok, err := fs.GetSessionBindingPayload(context.Background(), 10, sched.RouteKeyHash("rk1"), time.Now())
	if err != nil || !ok {
		t.Fatalf("expected session binding to exist after switch, ok=%v err=%v", ok, err)
	}
	route, parsed := parseCodexStickyBindingPayload(payload, time.Now())
	if !parsed {
		t.Fatalf("expected binding payload to parse, payload=%s", payload)
	}
	if route.channelID != 2 {
		t.Fatalf("expected binding to move to channel 2, got=%d payload=%s", route.channelID, payload)
	}
}

func TestResponses_CodexSession_ClearSticky_NoUserText_KeepsCompaction(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 0},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 22, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			22: {
				{ID: 222, EndpointID: 22, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
				{ID: 2, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var gotSel scheduler.Selection
	var gotBody []byte
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotSel = sel
		gotBody = append([]byte(nil), body...)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req1 := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses",
		bytes.NewReader([]byte(`{"model":"gpt-5.2","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false,"prompt_cache_key":"rk1"}`)))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(auth.WithPrincipal(req1.Context(), p))
	rr1 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr1.Code, rr1.Body.String())
	}

	fs.channels = []store.UpstreamChannel{
		{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 0},
		{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
	}

	req2 := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses",
		bytes.NewReader([]byte(`{"model":"gpt-5.2","input":[{"type":"item_reference","id":"ref1"}],"stream":false,"prompt_cache_key":"rk1","compaction":{"encrypted_content":"mRYQ...71fD"}}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.WithPrincipal(req2.Context(), p))
	rr2 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := rr2.Header().Get("X-Realms-Codex-Sticky-Cleared"); got != "1" {
		t.Fatalf("expected sticky cleared header, got=%q", got)
	}
	if gotSel.ChannelID != 2 {
		t.Fatalf("expected switch to channel 2, got=%d", gotSel.ChannelID)
	}
	if got := gjson.GetBytes(gotBody, "compaction.encrypted_content").String(); got != "mRYQ...71fD" {
		t.Fatalf("expected compaction to be preserved, got=%q body=%s", got, string(gotBody))
	}
}

func TestResponses_CodexSession_BindingPersistsAcrossHandlerInstances(t *testing.T) {
	sha256Hex := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	}
	rendezvousScore64 := func(routeKeyHash, kind string, id int64) uint64 {
		key := routeKeyHash + ":" + kind + ":" + strconv.FormatInt(id, 10)
		sum := sha256.Sum256([]byte(key))
		return binary.BigEndian.Uint64(sum[:8])
	}

	routeKey := ""
	for i := 0; i < 2000; i++ {
		candidate := fmt.Sprintf("rk_test_persist_%d", i)
		rkh := sha256Hex(candidate)
		if rendezvousScore64(rkh, "channel", 1) > rendezvousScore64(rkh, "channel", 2) {
			routeKey = candidate
			break
		}
	}
	if routeKey == "" {
		t.Fatalf("failed to find a routeKey that prefers channel 1")
	}

	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 0},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
			2: {
				{ID: 22, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
			22: {
				{ID: 222, EndpointID: 22, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
				{ID: 2, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	doer1 := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})

	sched1 := scheduler.New(fs)
	h1 := NewHandler(fs, fs, sched1, doer1, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req1 := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses",
		bytes.NewReader([]byte(fmt.Sprintf(`{"model":"gpt-5.2","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false,"prompt_cache_key":%q}`, routeKey))))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(auth.WithPrincipal(req1.Context(), p))
	rr1 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h1.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr1.Code, rr1.Body.String())
	}

	fs.channels = []store.UpstreamChannel{
		{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
	}

	doer2 := &okDoer{}
	sched2 := scheduler.New(fs)
	h2 := NewHandler(fs, fs, sched2, doer2, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req2 := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses",
		bytes.NewReader([]byte(fmt.Sprintf(`{"model":"gpt-5.2","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"next"}]}],"stream":false,"prompt_cache_key":%q}`, routeKey))))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.WithPrincipal(req2.Context(), p))
	rr2 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h2.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if len(doer2.calls) != 1 {
		t.Fatalf("expected 1 upstream call, got=%d", len(doer2.calls))
	}
	if doer2.calls[0].ChannelID != 2 {
		t.Fatalf("expected stored binding to force channel 2, got=%d", doer2.calls[0].ChannelID)
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	audit := &recordingAudit{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, audit, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "k1")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := statusDoer{status: http.StatusBadRequest, body: `{"error":{"message":"secret-body"}}`}
	audit := &recordingAudit{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, audit, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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

func TestResponses_CodexOAuth_UsageLimitFailoverToNextAccount(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example/backend-api/codex", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			11: {
				{ID: 1002, EndpointID: 11, Status: 1},
				{ID: 1001, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeCodexOAuth, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	var calls []scheduler.Selection
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls = append(calls, sel)
		if sel.CredentialID == 1002 {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","code":"usage_limit_reached"}}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"ok-from-openai","usage":{"input_tokens":1,"output_tokens":2}}`)),
		}, nil
	})
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "The usage limit has been reached") {
		t.Fatalf("expected failover response not to include exhausted response body, got=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "ok-from-openai") {
		t.Fatalf("expected fallback response from openai channel, got=%s", rr.Body.String())
	}
	if len(calls) < 2 {
		t.Fatalf("expected failover attempts across codex accounts, got=%d", len(calls))
	}
	if calls[0].CredentialType != scheduler.CredentialTypeCodex {
		t.Fatalf("expected first attempt on codex channel, got=%s", calls[0].CredentialType)
	}
	if calls[0].CredentialID != 1002 {
		t.Fatalf("expected first attempt on exhausted account 1002, got=%d", calls[0].CredentialID)
	}
	if calls[1].CredentialType != scheduler.CredentialTypeCodex || calls[1].CredentialID != 1001 {
		t.Fatalf("expected second attempt on fallback account 1001, got type=%s id=%d", calls[1].CredentialType, calls[1].CredentialID)
	}
}

func TestResponses_CodexOAuth_PatchesQuotaFromXCodexHeaders(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example/backend-api/codex", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			11: {
				{ID: 1001, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeCodexOAuth, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &codexQuotaPatchDoer{
		patched: make(chan store.CodexOAuthQuotaPatch, 1),
		ids:     make(chan int64, 1),
	}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	select {
	case gotID := <-doer.ids:
		if gotID != 1001 {
			t.Fatalf("patched account_id=%d, want 1001", gotID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for quota patch call")
	}

	select {
	case patch := <-doer.patched:
		// window-minutes mapping: 300 => 5h => quota_primary
		if patch.PrimaryUsedPercent == nil || *patch.PrimaryUsedPercent != 20 {
			t.Fatalf("PrimaryUsedPercent=%v, want 20", patch.PrimaryUsedPercent)
		}
		if patch.SecondaryUsedPercent == nil || *patch.SecondaryUsedPercent != 80 {
			t.Fatalf("SecondaryUsedPercent=%v, want 80", patch.SecondaryUsedPercent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for quota patch payload")
	}
}

func TestResponses_CodexOAuth_UsageLimitSetsPersistentCooldown(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example/backend-api/codex", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			11: {
				{ID: 1002, EndpointID: 11, Status: 1},
				{ID: 1001, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeCodexOAuth, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}
	sched := scheduler.New(fs)
	doer := &codexCooldownDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.cooldowns) != 1 {
		t.Fatalf("expected one cooldown update, got=%d", len(doer.cooldowns))
	}
	if doer.cooldowns[0] != 1002 {
		t.Fatalf("expected exhausted account 1002 cooldown update, got=%d", doer.cooldowns[0])
	}
	wantUnix := int64(2000000000)
	if got := doer.cooldownAt[0].Unix(); got != wantUnix {
		t.Fatalf("expected cooldown until unix=%d, got=%d", wantUnix, got)
	}
}

type codexQuotaMarkDoer struct {
	calls     []scheduler.Selection
	quotaErrs []int64
	quotaMsg  []*string
}

func (d *codexQuotaMarkDoer) Do(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
	d.calls = append(d.calls, sel)
	if sel.CredentialID == 1002 {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","code":"usage_limit_reached"}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"ok-from-openai","usage":{"input_tokens":1,"output_tokens":2}}`)),
	}, nil
}

func (d *codexQuotaMarkDoer) SetCodexOAuthAccountQuotaError(_ context.Context, accountID int64, msg *string) error {
	d.quotaErrs = append(d.quotaErrs, accountID)
	d.quotaMsg = append(d.quotaMsg, msg)
	return nil
}

func TestResponses_CodexOAuth_UsageLimitMarksBalanceDepleted(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example/backend-api/codex", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			11: {
				{ID: 1002, EndpointID: 11, Status: 1},
				{ID: 1001, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeCodexOAuth, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}
	sched := scheduler.New(fs)
	doer := &codexQuotaMarkDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(doer.quotaErrs) != 1 {
		t.Fatalf("expected one quota_error mark, got=%d", len(doer.quotaErrs))
	}
	if doer.quotaErrs[0] != 1002 {
		t.Fatalf("expected quota_error mark on exhausted account 1002, got=%d", doer.quotaErrs[0])
	}
	if doer.quotaMsg[0] == nil || *doer.quotaMsg[0] != "余额用尽" {
		got := "<nil>"
		if doer.quotaMsg[0] != nil {
			got = *doer.quotaMsg[0]
		}
		t.Fatalf("expected quota_error msg to be 余额用尽, got=%q", got)
	}
}

func TestResponses_CodexOAuth_UsageLimitNoFallbackReturnsUpstreamUnavailable(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example/backend-api/codex", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			11: {
				{ID: 1001, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeCodexOAuth, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","code":"usage_limit_reached"}}`)),
		}, nil
	})
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "The usage limit has been reached") {
		t.Fatalf("expected generic unavailable message instead of raw upstream body, got=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "上游不可用") {
		t.Fatalf("expected generic unavailable message, got=%s", rr.Body.String())
	}
}

func TestResponses_UpstreamUnavailableFinalizesUsageWithUpstreamChannelID(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
		groupByName: map[string]store.ChannelGroup{
			store.DefaultGroupName: {
				ID:              1,
				Name:            store.DefaultGroupName,
				PriceMultiplier: store.DefaultGroupPriceMultiplier,
				Status:          1,
			},
		},
		groupNameByID: map[int64]string{
			1: store.DefaultGroupName,
		},
	}

	sched := scheduler.New(fs)
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		return nil, errors.New("network down")
	})
	q := &fakeQuota{}
	u := &recordingUsage{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, u, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(u.calls) != 1 {
		t.Fatalf("expected 1 finalize call, got=%d", len(u.calls))
	}
	if u.calls[0].UpstreamChannelID == nil || *u.calls[0].UpstreamChannelID != 1 {
		t.Fatalf("expected upstream_channel_id=1, got=%+v", u.calls[0].UpstreamChannelID)
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
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
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
	if call.InputTokens == nil || *call.InputTokens <= 0 {
		t.Fatalf("expected input_tokens>0, got=%v", call.InputTokens)
	}
	if call.OutputTokens == nil || *call.OutputTokens <= 0 {
		t.Fatalf("expected output_tokens>0, got=%v", call.OutputTokens)
	}
}

func TestResponses_QuotaCommitIgnoresUpstreamCostFields(t *testing.T) {
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
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		// 仅包含 cost 字段，不包含 tokens：应被忽略（Commit 仍会被调用，但 tokens 应为 nil）。
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"total_cost":123.45,"cost_usd":123.45}}`))),
		}, nil
	})
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{
		ActorType: auth.ActorTypeToken,
		UserID:    10,
		Role:      store.UserRoleUser,
		TokenID:   &tokenID,
		Groups:    []string{store.DefaultGroupName},
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
	if call.UpstreamChannelID == nil || *call.UpstreamChannelID != 1 {
		t.Fatalf("expected upstream_channel_id=1, got=%v", call.UpstreamChannelID)
	}
	if call.InputTokens != nil || call.OutputTokens != nil || call.CachedInputTokens != nil || call.CachedOutputTokens != nil {
		t.Fatalf("expected all token fields to be nil, got input=%v cached_in=%v output=%v cached_out=%v", call.InputTokens, call.CachedInputTokens, call.OutputTokens, call.CachedOutputTokens)
	}
}

func TestResponses_Non2xxErrorBodyIsTruncated(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	huge := strings.Repeat("x", int(upstreamErrorBodyMaxBytes)+1024)
	sched := scheduler.New(fs)
	doer := statusDoer{status: http.StatusBadRequest, body: huge}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body_len=%d", rr.Code, rr.Body.Len())
	}
	if rr.Body.Len() != int(upstreamErrorBodyMaxBytes) {
		t.Fatalf("expected truncated body len=%d, got=%d", upstreamErrorBodyMaxBytes, rr.Body.Len())
	}
	if !strings.HasPrefix(huge, rr.Body.String()) {
		t.Fatalf("unexpected body prefix mismatch")
	}
}

func TestResponses_NonStreamLargeBodySkipsExtractionButStillProxies(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	hugeText := strings.Repeat("a", int(upstreamNonStreamExtractMaxBytes)+1024)
	body := `{"id":"ok","output_text":"` + hugeText + `","usage":{"input_tokens":1,"output_tokens":2}}`
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body_len=%d", rr.Code, rr.Body.Len())
	}
	if rr.Body.Len() != len(body) {
		t.Fatalf("expected body to be fully proxied, want_len=%d got=%d", len(body), rr.Body.Len())
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected exactly 1 commit call, got=%d", len(q.commitCalls))
	}
	call := q.commitCalls[0]
	if call.InputTokens != nil || call.OutputTokens != nil || call.CachedInputTokens != nil || call.CachedOutputTokens != nil {
		t.Fatalf("expected all token fields to be nil due to buffer exceed, got input=%v cached_in=%v output=%v cached_out=%v", call.InputTokens, call.CachedInputTokens, call.OutputTokens, call.CachedOutputTokens)
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
			"gpt-5.2": {
				ID:       1,
				PublicID: "gpt-5.2",
				Status:   1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
				{ID: 2, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"nope","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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
	h := NewHandler(fs, fs, sched, doer, nil, features, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"nope","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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

func TestResponses_ModelPassthrough_AliasRewriteIfBindingExists(t *testing.T) {
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
		bindings: map[string][]store.ChannelModelBinding{
			"alias": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "alias", UpstreamModel: "real-model", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	doer := &fakeDoer{}
	features := staticFeatures{fs: store.FeatureState{ModelsDisabled: true, BillingDisabled: true}}
	h := NewHandler(fs, fs, sched, doer, nil, features, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"alias","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"alias","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
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

func TestCodexUsageLimitCooldownUntil(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name string
		body string
		want time.Time
	}{
		{
			name: "resets_at 优先",
			body: `{"error":{"type":"usage_limit_reached","resets_at":2000000000}}`,
			want: time.Unix(2_000_000_000, 0),
		},
		{
			name: "resets_in_seconds 回退",
			body: `{"error":{"type":"usage_limit_reached","resets_in_seconds":120}}`,
			want: now.Add(120 * time.Second),
		},
		{
			name: "无重置信息使用默认5分钟",
			body: `{"error":{"type":"usage_limit_reached"}}`,
			want: now.Add(5 * time.Minute),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := codexUsageLimitCooldownUntil(now, []byte(tc.body))
			if got.Unix() != tc.want.Unix() {
				t.Fatalf("unexpected cooldown until: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestExtractRouteKeyFromPayload_MetadataSessionUserID(t *testing.T) {
	payload := map[string]any{
		"metadata": map[string]any{
			"user_id": "user_x_account_y_session_550e8400-e29b-41d4-a716-446655440000",
		},
	}
	got := extractRouteKeyFromPayload(payload)
	want := "550e8400-e29b-41d4-a716-446655440000"
	if got != want {
		t.Fatalf("unexpected route key: got=%q want=%q", got, want)
	}
}

func TestDeriveRouteKeyFromConversationPayload_FallbackHash(t *testing.T) {
	payload := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "hello"},
				},
			},
		},
	}
	got := normalizeRouteKey(deriveRouteKeyFromConversationPayload(payload))
	if got == "" {
		t.Fatalf("expected non-empty fallback route key")
	}
	if len(got) != 32 {
		t.Fatalf("expected 32 hex fallback hash, got=%q", got)
	}
	again := normalizeRouteKey(deriveRouteKeyFromConversationPayload(payload))
	if got != again {
		t.Fatalf("expected stable fallback hash, first=%q second=%q", got, again)
	}
}

func TestResponses_ContextCanceledDuringExec_FinalizesAsClientDisconnect(t *testing.T) {
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	q := &fakeQuota{}
	u := &recordingUsage{}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	doer := DoerFunc(func(ctx context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		cancel()
		return nil, ctx.Err()
	})
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, u, nil, upstream.SSEPumpOptions{}, nil)

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty body, got=%q", rr.Body.String())
	}
	if len(q.voidCalls) != 1 || q.voidCalls[0] != 1 {
		t.Fatalf("expected quota void(1), got=%v", q.voidCalls)
	}
	if len(u.calls) != 1 {
		t.Fatalf("expected 1 finalize call, got=%d", len(u.calls))
	}
	if u.calls[0].ErrorClass == nil || *u.calls[0].ErrorClass != "client_disconnect" {
		t.Fatalf("expected error_class=client_disconnect, got=%+v", u.calls[0].ErrorClass)
	}
	if u.calls[0].ErrorMessage != nil {
		t.Fatalf("expected empty error_message, got=%+v", u.calls[0].ErrorMessage)
	}
	if u.calls[0].StatusCode != 0 {
		t.Fatalf("expected status_code=0, got=%d", u.calls[0].StatusCode)
	}
}

func TestResponses_ContextCanceledDuringRouterNext_FinalizesAsClientDisconnect(t *testing.T) {
	started := make(chan struct{})
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
		blockGetChannelGroupByName: true,
		groupLookupStarted:         started,
	}

	sched := scheduler.New(fs)
	q := &fakeQuota{}
	u := &recordingUsage{}
	h := NewHandler(fs, fs, sched, &okDoer{}, nil, nil, q, fakeAudit{}, u, nil, upstream.SSEPumpOptions{}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":false}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for group lookup to start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for request to finish after cancel")
	}

	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty body, got=%q", rr.Body.String())
	}
	if len(q.voidCalls) != 1 || q.voidCalls[0] != 1 {
		t.Fatalf("expected quota void(1), got=%v", q.voidCalls)
	}
	if len(u.calls) != 1 {
		t.Fatalf("expected 1 finalize call, got=%d", len(u.calls))
	}
	if u.calls[0].ErrorClass == nil || *u.calls[0].ErrorClass != "client_disconnect" {
		t.Fatalf("expected error_class=client_disconnect, got=%+v", u.calls[0].ErrorClass)
	}
	if u.calls[0].ErrorMessage != nil {
		t.Fatalf("expected empty error_message, got=%+v", u.calls[0].ErrorMessage)
	}
	if u.calls[0].StatusCode != 0 {
		t.Fatalf("expected status_code=0, got=%d", u.calls[0].StatusCode)
	}
}

func TestResponses_FastModeUnsupportedReturnsBadRequest(t *testing.T) {
	decimalPtr := func(v string) *decimal.Decimal {
		d, err := decimal.NewFromString(v)
		if err != nil {
			t.Fatalf("decimal parse: %v", err)
		}
		return &d
	}
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1, AllowServiceTier: true, FastMode: false},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1}},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {{ID: 1, EndpointID: 11, Status: 1}},
		},
		models: map[string]store.ManagedModel{
			"m1": {
				ID:                     1,
				PublicID:               "m1",
				GroupName:              store.DefaultGroupName,
				InputUSDPer1M:          decimal.NewFromInt(1),
				OutputUSDPer1M:         decimal.NewFromInt(1),
				CacheInputUSDPer1M:     decimal.Zero,
				CacheOutputUSDPer1M:    decimal.Zero,
				PriorityPricingEnabled: true,
				PriorityInputUSDPer1M:  decimalPtr("2"),
				PriorityOutputUSDPer1M: decimalPtr("3"),
				Status:                 1,
			},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {{ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1}},
		},
	}
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected upstream call")
		return nil, nil
	})
	q := &fakeQuota{}
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10}, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi","service_tier":"fast"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "fast mode") && !strings.Contains(rr.Body.String(), "Fast mode") {
		t.Fatalf("expected fast mode error, got=%s", rr.Body.String())
	}
	if len(q.reserveCalls) != 1 {
		t.Fatalf("expected reserve called once, got=%d", len(q.reserveCalls))
	}
	if q.reserveCalls[0].ServiceTier == nil || *q.reserveCalls[0].ServiceTier != "priority" {
		t.Fatalf("reserve service_tier=%v, want priority", q.reserveCalls[0].ServiceTier)
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected void called once, got=%d", len(q.voidCalls))
	}
	if len(q.commitCalls) != 0 {
		t.Fatalf("expected commit not called, got=%d", len(q.commitCalls))
	}
}

func TestResponses_FailoverExhausted429MapsToRateLimitError(t *testing.T) {
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, statusDoer{
		status: http.StatusTooManyRequests,
		body:   `{"error":{"message":"raw upstream 429"}}`,
	}, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)
	h.SetGatewayPolicy(GatewayPolicy{MaxFailoverSwitches: 1})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"rate_limit_error"`) {
		t.Fatalf("expected rate_limit_error body, got=%s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "raw upstream 429") {
		t.Fatalf("expected stable message, got=%s", rr.Body.String())
	}
	if rr.Header().Get("Retry-After") != "30" {
		t.Fatalf("expected Retry-After=30, got=%q", rr.Header().Get("Retry-After"))
	}
}

func TestResponses_UserConcurrencyQueueFullReturns429(t *testing.T) {
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, &okDoer{}, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)
	h.SetGatewayPolicy(GatewayPolicy{UserMaxConcurrency: 1})
	h.SetConcurrencyManager(&fakeConcurrencyManager{userErr: concurrency.ErrQueueFull})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"rate_limit_error"`) {
		t.Fatalf("expected rate_limit_error, got=%s", rr.Body.String())
	}
}

func TestResponses_FailoverExhaustedAppliesPassthroughMatcher(t *testing.T) {
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, statusDoer{
		status: http.StatusTooManyRequests,
		body:   `{"error":{"message":"upstream throttled"}}`,
	}, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)
	h.SetGatewayPolicy(GatewayPolicy{MaxFailoverSwitches: 1})
	h.SetErrorPassthroughMatcher(fakePassthroughMatcher{
		status: http.StatusTeapot,
		msg:    "自定义透传错误",
		match:  true,
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"m1","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "自定义透传错误") {
		t.Fatalf("expected passthrough message, got=%s", rr.Body.String())
	}
}

func TestGeminiProxy_UserConcurrencyQueueFullReturnsGeminiErrorShape(t *testing.T) {
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
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, &okDoer{}, nil, nil, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)
	h.SetGatewayPolicy(GatewayPolicy{UserMaxConcurrency: 1})
	h.SetConcurrencyManager(&fakeConcurrencyManager{userErr: concurrency.ErrQueueFull})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1beta/models/m1:generateContent", bytes.NewReader([]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.GeminiProxy), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"type":"error"`) {
		t.Fatalf("expected non-OpenAI Gemini error shape, got=%s", rr.Body.String())
	}
	if gjson.Get(rr.Body.String(), "error.code").Int() != int64(http.StatusTooManyRequests) {
		t.Fatalf("expected error.code=429, got body=%s", rr.Body.String())
	}
	if gjson.Get(rr.Body.String(), "error.status").String() != "RESOURCE_EXHAUSTED" {
		t.Fatalf("expected error.status=RESOURCE_EXHAUSTED, got body=%s", rr.Body.String())
	}
}
