package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"
)

func makeTokenRequest(method string, path string, body string, userID int64) *http.Request {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "http://example.com"+path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: userID, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"g1"}}
	return req.WithContext(auth.WithPrincipal(req.Context(), p))
}

func runHandler(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)
	return rr
}

func TestResponses_ExtendedOps_OwnershipMismatch_Returns404WithoutUpstream(t *testing.T) {
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

	calls := 0
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	for name, tc := range map[string]struct {
		fn     http.HandlerFunc
		method string
		path   string
		body   string
	}{
		"delete":      {fn: h.ResponseDelete, method: http.MethodDelete, path: "/v1/responses/resp_999"},
		"cancel":      {fn: h.ResponseCancel, method: http.MethodPost, path: "/v1/responses/resp_999/cancel", body: `{"reason":"x"}`},
		"input_items": {fn: h.ResponseInputItems, method: http.MethodGet, path: "/v1/responses/resp_999/input_items"},
	} {
		t.Run(name, func(t *testing.T) {
			rr := runHandler(tc.fn, makeTokenRequest(tc.method, tc.path, tc.body, 11))
			if rr.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}

	if calls != 0 {
		t.Fatalf("expected no upstream calls, got=%d", calls)
	}
	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeResponse, "resp_999"); !ok {
		t.Fatalf("expected local ref to remain for owner")
	}
}

func TestResponseRetrieve_CodexSelection_Returns501(t *testing.T) {
	refs := newMemObjectRefs()
	selJSON, _ := json.Marshal(scheduler.Selection{
		ChannelID:      1,
		ChannelType:    store.UpstreamTypeCodexOAuth,
		EndpointID:     11,
		BaseURL:        "https://codex.example",
		CredentialType: scheduler.CredentialTypeCodex,
		CredentialID:   111,
	})
	_ = refs.UpsertOpenAIObjectRef(context.Background(), store.OpenAIObjectRef{
		ObjectType:    openAIObjectTypeResponse,
		ObjectID:      "resp_123",
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
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	rr := runHandler(h.ResponseRetrieve, makeTokenRequest(http.MethodGet, "/v1/responses/resp_123", "", 10))
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("expected no upstream calls, got=%d", calls)
	}
}

func TestResponsesInputTokens_RequiresOpenAICompatibleAndSkipsQuota(t *testing.T) {
	const groupName = "g1"
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
			{ID: 2, Type: store.UpstreamTypeOpenAICompatible, Status: 1, Groups: groupName},
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
			"gpt-5.2": {ID: 1, PublicID: "gpt-5.2", GroupName: groupName, Status: 1},
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
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})

	q := &fakeQuota{}
	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{})

	req := makeTokenRequest(http.MethodPost, "/v1/responses/input_tokens", `{"model":"gpt-5.2","input":"hi"}`, 10)
	rr := runHandler(h.Responses, req)
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

func TestChatCompletionUpdate_MismatchOwner_Returns404(t *testing.T) {
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

	calls := 0
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	req := makeTokenRequest(http.MethodPost, "/v1/chat/completions/chatcmpl-777", `{"metadata":{"x":"y"}}`, 11)
	rr := runHandler(h.ChatCompletionUpdate, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("expected no upstream calls, got=%d", calls)
	}
}

func TestChatCompletionDelete_OwnershipAndCleanup(t *testing.T) {
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
		ObjectID:      "chatcmpl-del",
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
			Body:       io.NopCloser(strings.NewReader(`{"deleted":true}`)),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	rr := runHandler(h.ChatCompletionDelete, makeTokenRequest(http.MethodDelete, "/v1/chat/completions/chatcmpl-del", "", 11))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatch owner, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("expected no upstream calls for mismatch, got=%d", calls)
	}
	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeChatCompletion, "chatcmpl-del"); !ok {
		t.Fatalf("expected local ref to remain after mismatch attempt")
	}

	rr2 := runHandler(h.ChatCompletionDelete, makeTokenRequest(http.MethodDelete, "/v1/chat/completions/chatcmpl-del", "", 10))
	if rr2.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected 1 upstream call, got=%d", calls)
	}
	if _, ok, _ := refs.GetOpenAIObjectRefForUser(context.Background(), 10, openAIObjectTypeChatCompletion, "chatcmpl-del"); ok {
		t.Fatalf("expected local ref to be cleaned up after delete")
	}
}

func TestChatCompletionMessages_OwnershipRequired(t *testing.T) {
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
		ObjectID:      "chatcmpl-msg",
		UserID:        10,
		TokenID:       123,
		SelectionJSON: string(selJSON),
	})

	calls := 0
	var gotPath string
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, downstream *http.Request, _ []byte) (*http.Response, error) {
		calls++
		gotPath = downstream.URL.Path
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[]}`)),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	rr := runHandler(h.ChatCompletionMessages, makeTokenRequest(http.MethodGet, "/v1/chat/completions/chatcmpl-msg/messages", "", 11))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatch owner, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("expected no upstream calls for mismatch, got=%d", calls)
	}

	rr2 := runHandler(h.ChatCompletionMessages, makeTokenRequest(http.MethodGet, "/v1/chat/completions/chatcmpl-msg/messages", "", 10))
	if rr2.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected 1 upstream call, got=%d", calls)
	}
	if gotPath != "/v1/chat/completions/chatcmpl-msg/messages" {
		t.Fatalf("unexpected upstream path: %q", gotPath)
	}
}

func TestChatCompletionsList_EmptyRefs_ReturnsEmptyListWithoutUpstream(t *testing.T) {
	refs := newMemObjectRefs()
	calls := 0
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"object":"list","data":[{"id":"x"}]}`))),
		}, nil
	})
	h := NewHandler(nil, nil, scheduler.New(&fakeStore{}), doer, nil, nil, false, nil, fakeAudit{}, nil, refs, upstream.SSEPumpOptions{})

	rr := runHandler(h.ChatCompletionsList, makeTokenRequest(http.MethodGet, "/v1/chat/completions", "", 10))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != `{"object":"list","data":[]}` {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("expected no upstream calls, got=%d", calls)
	}
}
