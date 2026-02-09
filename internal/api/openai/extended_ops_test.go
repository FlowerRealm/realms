package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"
)

type memObjectRefs struct {
	mu   sync.Mutex
	data map[string]store.OpenAIObjectRef
}

func newMemObjectRefs() *memObjectRefs {
	return &memObjectRefs{data: make(map[string]store.OpenAIObjectRef)}
}

func (m *memObjectRefs) key(objectType, objectID string) string {
	return strings.TrimSpace(objectType) + ":" + strings.TrimSpace(objectID)
}

func (m *memObjectRefs) UpsertOpenAIObjectRef(_ context.Context, ref store.OpenAIObjectRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(ref.ObjectType, ref.ObjectID)
	now := time.Now()
	existing, ok := m.data[k]
	if ok && !existing.CreatedAt.IsZero() {
		ref.CreatedAt = existing.CreatedAt
	} else {
		ref.CreatedAt = now
	}
	ref.UpdatedAt = now
	m.data[k] = ref
	return nil
}

func (m *memObjectRefs) GetOpenAIObjectRefForUser(_ context.Context, userID int64, objectType string, objectID string) (store.OpenAIObjectRef, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.data[m.key(objectType, objectID)]
	if !ok || ref.UserID != userID {
		return store.OpenAIObjectRef{}, false, nil
	}
	return ref, true, nil
}

func (m *memObjectRefs) ListOpenAIObjectRefsByUser(_ context.Context, userID int64, objectType string, limit int) ([]store.OpenAIObjectRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.OpenAIObjectRef
	for _, ref := range m.data {
		if ref.UserID != userID {
			continue
		}
		if strings.TrimSpace(ref.ObjectType) != strings.TrimSpace(objectType) {
			continue
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memObjectRefs) DeleteOpenAIObjectRef(_ context.Context, objectType string, objectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, m.key(objectType, objectID))
	return nil
}

func TestResponses_NonStream_StoresObjectRefAndTouchesBinding(t *testing.T) {
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

	refs := newMemObjectRefs()
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"resp_123","usage":{"input_tokens":1,"output_tokens":2}}`))),
		}, nil
	})
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeResponse, "resp_123"); !ok {
		t.Fatalf("expected response object ref to be stored")
	}
	if _, ok := sched.GetBinding(10, sched.RouteKeyHash("resp_123")); !ok {
		t.Fatalf("expected binding to be touched by response id")
	}
}

func TestResponses_Stream_StoresObjectRefFromSSE(t *testing.T) {
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

	refs := newMemObjectRefs()
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_456\"}}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","stream":true}`)))
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
	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeResponse, "resp_456"); !ok {
		t.Fatalf("expected response object ref to be stored from SSE")
	}
}

func TestChatCompletions_StoreTrue_InsertsOwnerMetadataAndStoresObjectRef(t *testing.T) {
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
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "o3-mini", Status: 1},
			},
		},
	}

	refs := newMemObjectRefs()
	var gotBody []byte
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"chatcmpl-123","object":"chat.completion"}`))),
		}, nil
	})
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	reqBody := `{"model":"m1","messages":[{"role":"user","content":"hi"}],"store":true}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ChatCompletions), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var forwarded map[string]any
	if err := json.Unmarshal(gotBody, &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded: %v", err)
	}
	meta, _ := forwarded["metadata"].(map[string]any)
	if meta == nil {
		t.Fatalf("expected metadata to be present")
	}
	wantOwner := realmsOwnerTagForUser(10)
	if got := strings.TrimSpace(stringFromAny(meta[realmsOwnerMetadataKey])); got != wantOwner {
		t.Fatalf("unexpected realms_owner: %q want=%q", got, wantOwner)
	}

	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeChatCompletion, "chatcmpl-123"); !ok {
		t.Fatalf("expected chat completion object ref to be stored")
	}
}

func TestResponsesCompact_RequiresOpenAICompatibleAndSkipsQuota(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://codex.example", Status: 1},
			},
			2: {
				{ID: 22, ChannelID: 2, BaseURL: "https://openai.example", Status: 1},
			},
		},
		accounts: map[int64][]store.CodexOAuthAccount{
			11: {
				{ID: 111, EndpointID: 11, Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			22: {
				{ID: 222, EndpointID: 22, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"gpt-5.2": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeCodexOAuth, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
				{ID: 2, ChannelID: 2, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "gpt-5.2", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var gotSel scheduler.Selection
	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		gotSel = sel
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`))),
		}, nil
	})

	q := &fakeQuota{}
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Responses), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if gotSel.ChannelType != store.UpstreamTypeOpenAICompatible {
		t.Fatalf("expected openai_compatible selection, got=%s", gotSel.ChannelType)
	}
	if len(q.reserveCalls) != 0 {
		t.Fatalf("expected quota reserve to be skipped, got=%d", len(q.reserveCalls))
	}
}

func TestChatCompletionRetrieve_OwnershipRequired(t *testing.T) {
	fs := &fakeStore{}
	refs := newMemObjectRefs()

	selJSON, _ := json.Marshal(scheduler.Selection{
		ChannelID:         1,
		ChannelType:       store.UpstreamTypeOpenAICompatible,
		EndpointID:        11,
		BaseURL:           "https://a.example",
		CredentialType:    scheduler.CredentialTypeOpenAI,
		CredentialID:      1,
		StatusCodeMapping: "",
	})
	_ = refs.UpsertOpenAIObjectRef(context.Background(), store.OpenAIObjectRef{
		ObjectType:    openAIObjectTypeChatCompletion,
		ObjectID:      "chatcmpl-999",
		UserID:        10,
		TokenID:       123,
		SelectionJSON: string(selJSON),
	})

	calls := 0
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"chatcmpl-999"}`))),
		}, nil
	})
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	makeReq := func(userID int64) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/chat/completions/chatcmpl-999", nil)
		tokenID := int64(123)
		p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: userID, Role: store.UserRoleUser, TokenID: &tokenID}
		return req.WithContext(auth.WithPrincipal(req.Context(), p))
	}

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ChatCompletionRetrieve), middleware.BodyCache(1<<20)).ServeHTTP(rr, makeReq(11))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatch owner, got=%d", rr.Code)
	}
	if calls != 0 {
		t.Fatalf("expected doer not to be called for mismatch owner")
	}

	rr2 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ChatCompletionRetrieve), middleware.BodyCache(1<<20)).ServeHTTP(rr2, makeReq(10))
	if rr2.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected doer to be called once, got=%d", calls)
	}
}

func TestResponses_ExtendedOps_OwnershipAndDeleteCleanup(t *testing.T) {
	refs := newMemObjectRefs()
	selJSON, _ := json.Marshal(scheduler.Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   1,
	})
	_ = refs.UpsertOpenAIObjectRef(context.Background(), store.OpenAIObjectRef{
		ObjectType:    openAIObjectTypeResponse,
		ObjectID:      "resp_999",
		UserID:        10,
		TokenID:       123,
		SelectionJSON: string(selJSON),
	})

	var gotPath string
	var gotMethod string
	var gotBody []byte
	calls := 0
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
		calls++
		gotPath = downstream.URL.Path
		gotMethod = downstream.Method
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`))),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	makeReq := func(method, path string, userID int64, body string) *http.Request {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, "http://example.com"+path, rdr)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		tokenID := int64(123)
		p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: userID, Role: store.UserRoleUser, TokenID: &tokenID}
		return req.WithContext(auth.WithPrincipal(req.Context(), p))
	}

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponseRetrieve), middleware.BodyCache(1<<20)).ServeHTTP(rr, makeReq(http.MethodGet, "/v1/responses/resp_999", 11, ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got=%d", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponseRetrieve), middleware.BodyCache(1<<20)).ServeHTTP(rr2, makeReq(http.MethodGet, "/v1/responses/resp_999", 10, ""))
	if rr2.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if gotPath != "/v1/responses/resp_999" || gotMethod != http.MethodGet {
		t.Fatalf("unexpected upstream request: %s %s", gotMethod, gotPath)
	}

	rr3 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponseCancel), middleware.BodyCache(1<<20)).ServeHTTP(rr3, makeReq(http.MethodPost, "/v1/responses/resp_999/cancel", 10, `{"reason":"x"}`))
	if rr3.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr3.Code, rr3.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/responses/resp_999/cancel" {
		t.Fatalf("unexpected cancel upstream request: %s %s", gotMethod, gotPath)
	}
	if string(gotBody) != `{"reason":"x"}` {
		t.Fatalf("unexpected cancel body: %s", string(gotBody))
	}

	rr4 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponseInputItems), middleware.BodyCache(1<<20)).ServeHTTP(rr4, makeReq(http.MethodGet, "/v1/responses/resp_999/input_items", 10, ""))
	if rr4.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr4.Code, rr4.Body.String())
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/responses/resp_999/input_items" {
		t.Fatalf("unexpected input_items upstream request: %s %s", gotMethod, gotPath)
	}

	rr5 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponseDelete), middleware.BodyCache(1<<20)).ServeHTTP(rr5, makeReq(http.MethodDelete, "/v1/responses/resp_999", 10, ""))
	if rr5.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr5.Code, rr5.Body.String())
	}
	if calls == 0 {
		t.Fatalf("expected upstream calls")
	}
	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeResponse, "resp_999"); ok {
		t.Fatalf("expected delete to cleanup local object ref")
	}
}

func TestChatCompletionsList_FiltersByLocalRefsAndForcesOwnerMetadataQuery(t *testing.T) {
	refs := newMemObjectRefs()
	selJSON, _ := json.Marshal(scheduler.Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   1,
	})
	_ = refs.UpsertOpenAIObjectRef(context.Background(), store.OpenAIObjectRef{
		ObjectType:    openAIObjectTypeChatCompletion,
		ObjectID:      "chatcmpl-allow",
		UserID:        10,
		TokenID:       123,
		SelectionJSON: string(selJSON),
	})

	var gotQuery string
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, downstream *http.Request, _ []byte) (*http.Response, error) {
		gotQuery = downstream.URL.RawQuery
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewReader([]byte(`{
  "object":"list",
  "data":[
    {"id":"chatcmpl-allow","object":"chat.completion"},
    {"id":"chatcmpl-other","object":"chat.completion"}
  ],
  "first_id":"chatcmpl-allow",
  "last_id":"chatcmpl-other",
  "has_more":false
}`))),
		}, nil
	})

	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/chat/completions?metadata[foo]=bar&limit=2", nil)
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ChatCompletionsList), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(gotQuery, "metadata%5Bfoo%5D") {
		t.Fatalf("expected user metadata filter to be stripped, got query=%s", gotQuery)
	}
	wantOwner := realmsOwnerTagForUser(10)
	if !strings.Contains(gotQuery, "metadata%5B"+realmsOwnerMetadataKey+"%5D="+wantOwner) {
		t.Fatalf("expected owner metadata filter, got query=%s", gotQuery)
	}

	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, _ := out["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 filtered item, got=%d", len(data))
	}
	item, _ := data[0].(map[string]any)
	if strings.TrimSpace(stringFromAny(item["id"])) != "chatcmpl-allow" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if strings.TrimSpace(stringFromAny(out["first_id"])) != "chatcmpl-allow" {
		t.Fatalf("unexpected first_id: %#v", out["first_id"])
	}
	if strings.TrimSpace(stringFromAny(out["last_id"])) != "chatcmpl-allow" {
		t.Fatalf("unexpected last_id: %#v", out["last_id"])
	}
}

func TestChatCompletionUpdate_ForcesOwnerMetadata(t *testing.T) {
	refs := newMemObjectRefs()
	selJSON, _ := json.Marshal(scheduler.Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeOpenAICompatible,
		EndpointID:     11,
		BaseURL:        "https://a.example",
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   1,
	})
	_ = refs.UpsertOpenAIObjectRef(context.Background(), store.OpenAIObjectRef{
		ObjectType:    openAIObjectTypeChatCompletion,
		ObjectID:      "chatcmpl-777",
		UserID:        10,
		TokenID:       123,
		SelectionJSON: string(selJSON),
	})

	var gotBody []byte
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"chatcmpl-777"}`))),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions/chatcmpl-777", bytes.NewReader([]byte(`{"metadata":{"realms_owner":"evil","x":"y"}}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ChatCompletionUpdate), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var forwarded map[string]any
	if err := json.Unmarshal(gotBody, &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	meta, _ := forwarded["metadata"].(map[string]any)
	if meta == nil {
		t.Fatalf("expected metadata")
	}
	wantOwner := realmsOwnerTagForUser(10)
	if got := strings.TrimSpace(stringFromAny(meta[realmsOwnerMetadataKey])); got != wantOwner {
		t.Fatalf("unexpected realms_owner: %q want=%q", got, wantOwner)
	}
}

func TestModelRetrieve_ExistsAndMissing(t *testing.T) {
	fs := &fakeStore{
		models: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "m1", Status: 1},
			},
		},
	}
	h := NewHandler(fs, fs, scheduler.New(fs), nil, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{})

	makeReq := func(path string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		tokenID := int64(123)
		p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
		return req.WithContext(auth.WithPrincipal(req.Context(), p))
	}

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ModelRetrieve), middleware.BodyCache(1<<20)).ServeHTTP(rr, makeReq("/v1/models/m1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	rr2 := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ModelRetrieve), middleware.BodyCache(1<<20)).ServeHTTP(rr2, makeReq("/v1/models/nope"))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got=%d body=%s", rr2.Code, rr2.Body.String())
	}
}
