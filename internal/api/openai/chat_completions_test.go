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

func TestChatCompletions_DropsUnknownAndAppliesOModelRules(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://openai.example", Status: 1},
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
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "o3-mini-high", Status: 1},
			},
		},
	}

	var gotPath string
	var gotBody []byte

	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
		gotPath = downstream.URL.Path
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"prompt_tokens":1,"completion_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, upstream.SSEPumpOptions{})

	reqBody := `{"model":"m1","messages":[{"role":"system","content":"hi"}],"max_tokens":10,"temperature":0.7,"unknown":123}`
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
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("unexpected upstream path: %q", gotPath)
	}

	var forwarded map[string]any
	if err := json.Unmarshal(gotBody, &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	if forwarded["model"] != "o3-mini" {
		t.Fatalf("expected model=o3-mini, got=%#v", forwarded["model"])
	}
	if forwarded["reasoning_effort"] != "high" {
		t.Fatalf("expected reasoning_effort=high, got=%#v", forwarded["reasoning_effort"])
	}
	if _, ok := forwarded["max_tokens"]; ok {
		t.Fatalf("expected max_tokens to be removed, got=%#v", forwarded["max_tokens"])
	}
	if v, ok := forwarded["max_completion_tokens"].(float64); !ok || int64(v) != 10 {
		t.Fatalf("expected max_completion_tokens=10, got=%#v", forwarded["max_completion_tokens"])
	}
	if _, ok := forwarded["temperature"]; ok {
		t.Fatalf("expected temperature to be removed, got=%#v", forwarded["temperature"])
	}
	if _, ok := forwarded["unknown"]; ok {
		t.Fatalf("expected unknown to be dropped, got=%#v", forwarded["unknown"])
	}
	if _, ok := forwarded["stream_options"]; ok {
		t.Fatalf("expected stream_options to be absent for non-stream request, got=%#v", forwarded["stream_options"])
	}

	msgs, _ := forwarded["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("expected messages to exist")
	}
	m0, _ := msgs[0].(map[string]any)
	if strings.TrimSpace(stringFromAny(m0["role"])) != "developer" {
		t.Fatalf("expected messages[0].role=developer, got=%#v", m0["role"])
	}
}

func TestChatCompletions_AliasesMaxOutputTokens(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://openai.example", Status: 1},
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
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "o3-mini-high", Status: 1},
			},
		},
	}

	var gotBody []byte
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"ok","usage":{"prompt_tokens":1,"completion_tokens":2}}`))),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, upstream.SSEPumpOptions{})

	reqBody := `{"model":"m1","messages":[{"role":"system","content":"hi"}],"max_output_tokens":10,"temperature":0.7,"unknown":123}`
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
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	if _, ok := forwarded["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be removed, got=%#v", forwarded["max_output_tokens"])
	}
	if _, ok := forwarded["max_tokens"]; ok {
		t.Fatalf("expected max_tokens to be removed, got=%#v", forwarded["max_tokens"])
	}
	if v, ok := forwarded["max_completion_tokens"].(float64); !ok || int64(v) != 10 {
		t.Fatalf("expected max_completion_tokens=10, got=%#v", forwarded["max_completion_tokens"])
	}
	if _, ok := forwarded["unknown"]; ok {
		t.Fatalf("expected unknown to be dropped, got=%#v", forwarded["unknown"])
	}
}

func TestChatCompletions_Stream_ForcesStreamOptionsIncludeUsage(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://openai.example", Status: 1},
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
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeOpenAICompatible, PublicID: "m1", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
	}

	var gotBody []byte
	doer := DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotBody = body
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")),
		}, nil
	})

	sched := scheduler.New(fs)
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	reqBody := `{"model":"m1","messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":false}}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

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
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	so, _ := forwarded["stream_options"].(map[string]any)
	if so == nil {
		t.Fatalf("expected stream_options to exist")
	}
	if v, ok := so["include_usage"].(bool); !ok || !v {
		t.Fatalf("expected stream_options.include_usage=true, got=%#v", so["include_usage"])
	}
}
