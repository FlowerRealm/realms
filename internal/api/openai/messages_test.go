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

func TestMessages_Stream_ExtractsUsageFromSSE_AnthropicCacheTokens(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeOpenAICompatible, Status: 1},
			{ID: 2, Type: store.UpstreamTypeAnthropic, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://openai.example", Status: 1},
			},
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://anthropic.example", Status: 1},
			},
		},
		creds: map[int64][]store.OpenAICompatibleCredential{
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		anthropicCreds: map[int64][]store.AnthropicCredential{
			21: {
				{ID: 2, EndpointID: 21, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"claude-3-5-sonnet-latest": {ID: 1, PublicID: "claude-3-5-sonnet-latest", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"claude-3-5-sonnet-latest": {
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeAnthropic, PublicID: "claude-3-5-sonnet-latest", UpstreamModel: "claude-3-5-sonnet-latest", Status: 1},
			},
		},
	}

	var gotSel scheduler.Selection
	var gotBody []byte

	doer := DoerFunc(func(_ context.Context, sel scheduler.Selection, _ *http.Request, body []byte) (*http.Response, error) {
		gotSel = sel
		gotBody = body

		sse := "event: message_start\n" +
			"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"cache_read_input_tokens\":1,\"cache_creation_input_tokens\":2}}}\n\n" +
			"event: content_block_delta\n" +
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"pong\"}}\n\n" +
			"event: message_stop\n" +
			"data: {\"type\":\"message_stop\"}\n\n"

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sse)),
		}, nil
	})

	sched := scheduler.New(fs)
	q := &fakeQuota{}
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, q, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	reqBody := `{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":"hi"}],"max_tokens":123,"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Messages), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if gotSel.ChannelType != store.UpstreamTypeAnthropic {
		t.Fatalf("expected anthropic channel, got=%q", gotSel.ChannelType)
	}
	if strings.TrimSpace(rr.Body.String()) == "" {
		t.Fatalf("expected streaming body, got empty")
	}

	var forwarded map[string]any
	if err := json.Unmarshal(gotBody, &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	if v, ok := forwarded["max_tokens"].(float64); !ok || int64(v) != 123 {
		t.Fatalf("expected max_tokens=123, got=%v", forwarded["max_tokens"])
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
	if got.CachedInputTokens == nil || *got.CachedInputTokens != 3 {
		t.Fatalf("expected cached_input_tokens=3, got=%+v", got.CachedInputTokens)
	}
}

func TestMessages_ChannelRequestPolicy_IsPerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeAnthropic, Status: 1, DisableStore: true},
			{ID: 1, Type: store.UpstreamTypeAnthropic, Status: 1, AllowServiceTier: true, AllowSafetyIdentifier: true},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		anthropicCreds: map[int64][]store.AnthropicCredential{
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
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up2", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1},
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	reqBody := `{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_tokens":10,"service_tier":"default","store":true,"safety_identifier":"u123"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Messages), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 attempts, got=%d", len(calls))
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

	var second map[string]any
	if err := json.Unmarshal(bodies[1], &second); err != nil {
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

func TestMessages_ChannelParamOverride_IsPerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeAnthropic, Status: 1, DisableStore: true, ParamOverride: `{"operations":[{"path":"metadata.channel","mode":"set","value":"b"},{"path":"store","mode":"set","value":true}]}`},
			{ID: 1, Type: store.UpstreamTypeAnthropic, Status: 1, ParamOverride: `{"operations":[{"path":"metadata.channel","mode":"set","value":"a"},{"path":"store","mode":"set","value":true}]}`},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		anthropicCreds: map[int64][]store.AnthropicCredential{
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
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up2", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1},
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", bytes.NewReader([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`)))
	req.Header.Set("Content-Type", "application/json")
	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Messages), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 attempts, got=%d", len(calls))
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

	var second map[string]any
	if err := json.Unmarshal(bodies[1], &second); err != nil {
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

func TestMessages_MaxOutputTokensAlias_NormalizesToMaxTokens(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeAnthropic, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://anthropic.example", Status: 1},
			},
		},
		anthropicCreds: map[int64][]store.AnthropicCredential{
			11: {
				{ID: 1, EndpointID: 11, Status: 1},
			},
		},
		models: map[string]store.ManagedModel{
			"m1": {ID: 1, PublicID: "m1", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"m1": {
				{ID: 1, ChannelID: 1, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up", Status: 1},
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{})

	reqBody := `{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_output_tokens":123}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Messages), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

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
		t.Fatalf("expected max_output_tokens to be removed, got=%v", forwarded["max_output_tokens"])
	}
	if _, ok := forwarded["max_completion_tokens"]; ok {
		t.Fatalf("expected max_completion_tokens to be removed, got=%v", forwarded["max_completion_tokens"])
	}
}

func TestMessages_ChannelBodyFilters_ArePerChannelAttempt(t *testing.T) {
	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 2, Type: store.UpstreamTypeAnthropic, Status: 1, RequestBodyBlacklist: `["metadata.trace","extra"]`},
			{ID: 1, Type: store.UpstreamTypeAnthropic, Status: 1, RequestBodyWhitelist: `["model","messages","max_tokens"]`},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			2: {
				{ID: 21, ChannelID: 2, BaseURL: "https://b.example", Status: 1},
			},
			1: {
				{ID: 11, ChannelID: 1, BaseURL: "https://a.example", Status: 1},
			},
		},
		anthropicCreds: map[int64][]store.AnthropicCredential{
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
				{ID: 1, ChannelID: 2, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up2", Status: 1},
				{ID: 2, ChannelID: 1, ChannelType: store.UpstreamTypeAnthropic, PublicID: "m1", UpstreamModel: "m1-up1", Status: 1},
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
	h := NewHandler(fs, fs, sched, doer, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{})

	reqBody := `{"model":"m1","messages":[{"role":"user","content":"hi"}],"max_tokens":10,"metadata":{"trace":"t","keep":"k"},"extra":"x"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID}
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.Messages), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 attempts, got=%d", len(bodies))
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

	var second map[string]any
	if err := json.Unmarshal(bodies[1], &second); err != nil {
		t.Fatalf("unmarshal second body: %v", err)
	}
	if second["model"] != "m1-up1" {
		t.Fatalf("unexpected second model: %v", second["model"])
	}
	if _, ok := second["messages"]; !ok {
		t.Fatalf("expected messages to be present on channel 1")
	}
	if _, ok := second["max_tokens"]; !ok {
		t.Fatalf("expected max_tokens to be present on channel 1")
	}
	if _, ok := second["metadata"]; ok {
		t.Fatalf("expected metadata to be removed by whitelist on channel 1")
	}
	if _, ok := second["extra"]; ok {
		t.Fatalf("expected extra to be removed by whitelist on channel 1")
	}
}
