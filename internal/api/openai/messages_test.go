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
	h := NewHandler(fs, fs, sched, doer, nil, nil, nil, nil, false, q, fakeAudit{}, nil, 123, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10})

	reqBody := `{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":"hi"}],"stream":true}`
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
	if got.InputTokens == nil || *got.InputTokens != 3 {
		t.Fatalf("unexpected input_tokens: %+v", got.InputTokens)
	}
	if got.OutputTokens == nil || *got.OutputTokens != 4 {
		t.Fatalf("unexpected output_tokens: %+v", got.OutputTokens)
	}
	if got.CachedInputTokens == nil || *got.CachedInputTokens != 3 {
		t.Fatalf("unexpected cached_input_tokens: %+v", got.CachedInputTokens)
	}
}
