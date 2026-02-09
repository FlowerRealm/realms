package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"realms/internal/scheduler"
	"realms/internal/store"
)

func TestCopyHeaders_StripsSensitiveAndHopByHop(t *testing.T) {
	src := make(http.Header)
	src.Add("Cookie", "realms_session=abc")
	src.Add("Connection", "keep-alive, Foo")
	src.Add("Foo", "bar")
	src.Add("Keep-Alive", "timeout=5")
	src.Add("Transfer-Encoding", "chunked")
	src.Add("Upgrade", "websocket")
	src.Add("Host", "evil.example.com")
	src.Add("Content-Length", "123")
	src.Add("Content-Type", "application/json")
	src.Add("User-Agent", "ua")

	dst := make(http.Header)
	copyHeaders(dst, src)

	for _, k := range []string{"Cookie", "Connection", "Foo", "Keep-Alive", "Transfer-Encoding", "Upgrade", "Host", "Content-Length"} {
		if got := dst.Get(k); got != "" {
			t.Fatalf("expected %s to be stripped, got %q", k, got)
		}
	}
	if got := dst.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type to be copied, got %q", got)
	}
	if got := dst.Get("User-Agent"); got != "ua" {
		t.Fatalf("expected User-Agent to be copied, got %q", got)
	}
}

func TestIsStreamRequest_ResponsesRetrieve_StreamQuery(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.com/v1/responses/resp_123?stream=true", nil)
	if !isStreamRequest(r, nil) {
		t.Fatalf("expected isStreamRequest to be true")
	}
}

func TestIsStreamRequest_ResponsesRetrieve_AcceptEventStream(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.com/v1/responses/resp_123", nil)
	r.Header.Set("Accept", "text/event-stream")
	if !isStreamRequest(r, nil) {
		t.Fatalf("expected isStreamRequest to be true")
	}
}

func TestIsStreamRequest_ResponsesRetrieve_DefaultFalse(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.com/v1/responses/resp_123", nil)
	if isStreamRequest(r, nil) {
		t.Fatalf("expected isStreamRequest to be false")
	}
}

func TestWrapTimeout_ResponsesRetrieve_StreamSkipsUpstreamTimeout(t *testing.T) {
	exec := &Executor{upstreamTimeout: 10 * time.Second}
	r := httptest.NewRequest(http.MethodGet, "http://example.com/v1/responses/resp_123?stream=true", nil)
	ctx, cancel := exec.wrapTimeout(context.Background(), scheduler.Selection{CredentialType: scheduler.CredentialTypeOpenAI}, r, nil)
	if cancel != nil {
		defer cancel()
	}
	if deadline, ok := ctx.Deadline(); ok {
		t.Fatalf("expected no deadline, got=%v", deadline)
	}
	if cancel != nil {
		t.Fatalf("expected cancel to be nil for stream request")
	}
}

func TestWrapTimeout_ResponsesRetrieve_NonStreamUsesUpstreamTimeout(t *testing.T) {
	exec := &Executor{upstreamTimeout: 10 * time.Second}
	r := httptest.NewRequest(http.MethodGet, "http://example.com/v1/responses/resp_123", nil)
	ctx, cancel := exec.wrapTimeout(context.Background(), scheduler.Selection{CredentialType: scheduler.CredentialTypeOpenAI}, r, nil)
	if cancel == nil {
		t.Fatalf("expected cancel to be non-nil")
	}
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatalf("expected deadline to be set")
	}
}

type fakeUpstreamStore struct {
	codexSecret     store.CodexOAuthSecret
	openaiSecret    store.OpenAICredentialSecret
	anthropicSecret store.AnthropicCredentialSecret
	channel         store.UpstreamChannel

	updateTokensCalls int
	setStatusCalls    int
	setCooldownCalls  int
}

func (f *fakeUpstreamStore) GetOpenAICompatibleCredentialSecret(_ context.Context, credentialID int64) (store.OpenAICredentialSecret, error) {
	sec := f.openaiSecret
	sec.ID = credentialID
	return sec, nil
}

func (f *fakeUpstreamStore) GetAnthropicCredentialSecret(_ context.Context, credentialID int64) (store.AnthropicCredentialSecret, error) {
	sec := f.anthropicSecret
	sec.ID = credentialID
	return sec, nil
}

func (f *fakeUpstreamStore) GetUpstreamChannelByID(_ context.Context, channelID int64) (store.UpstreamChannel, error) {
	ch := f.channel
	ch.ID = channelID
	return ch, nil
}

func (f *fakeUpstreamStore) GetCodexOAuthSecret(_ context.Context, accountID int64) (store.CodexOAuthSecret, error) {
	sec := f.codexSecret
	sec.ID = accountID
	return sec, nil
}

func (f *fakeUpstreamStore) UpdateCodexOAuthAccountTokens(_ context.Context, _ int64, _, _ string, _ *string, _ *time.Time) error {
	f.updateTokensCalls++
	return nil
}

func (f *fakeUpstreamStore) SetCodexOAuthAccountStatus(_ context.Context, _ int64, _ int) error {
	f.setStatusCalls++
	return nil
}

func (f *fakeUpstreamStore) SetCodexOAuthAccountCooldown(_ context.Context, _ int64, _ time.Time) error {
	f.setCooldownCalls++
	return nil
}

func TestExecutor_HeaderOverride_AppliesAndDoesNotOverrideDefaultAuth(t *testing.T) {
	exec := &Executor{
		st: &fakeUpstreamStore{
			openaiSecret: store.OpenAICredentialSecret{
				APIKey: "sk_test",
			},
		},
		upstreamTimeout: 2 * time.Minute,
	}

	body := []byte(`{"model":"m1","stream":false,"input":"hi"}`)
	r := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	req, err := exec.buildRequest(context.Background(), scheduler.Selection{
		BaseURL:        "https://127.0.0.1/v1",
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   1,
		HeaderOverride: `{"X-Proxy-Key":"{api_key}","Authorization":"Bearer override"}`,
	}, r, body)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}
	if got := req.Header.Get("X-Proxy-Key"); got != "sk_test" {
		t.Fatalf("expected X-Proxy-Key to be overridden, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk_test" {
		t.Fatalf("expected Authorization to use upstream api key, got %q", got)
	}
}

func TestExecutor_CodexOAuth_LeavesPathAndBody(t *testing.T) {
	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		upstreamTimeout: 2 * time.Minute,
	}

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_output_tokens":123,"input":"hi"}`)
	r := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))

	req, err := exec.buildRequest(context.Background(), scheduler.Selection{
		BaseURL:        "https://127.0.0.1/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
		CredentialID:   123,
	}, r, body)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}

	if got := req.URL.Path; got != "/backend-api/codex/v1/responses" {
		t.Fatalf("expected path to be passthrough, got %q", got)
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("expected body to be passthrough, got %s", string(gotBody))
	}
}

func TestApplyCodexHeaders_ReusesSessionIDFromSessionDashID(t *testing.T) {
	h := make(http.Header)
	h.Set("Session-Id", "sess-from-dash")

	applyCodexHeaders(h, "")

	if got := h.Get("Session_id"); got != "sess-from-dash" {
		t.Fatalf("expected Session_id from Session-Id, got %q", got)
	}
}

func TestApplyCodexHeaders_ReusesSessionIDFromXSessionID(t *testing.T) {
	h := make(http.Header)
	h.Set("X-Session-Id", "sess-from-x")

	applyCodexHeaders(h, "")

	if got := h.Get("Session_id"); got != "sess-from-x" {
		t.Fatalf("expected Session_id from X-Session-Id, got %q", got)
	}
}

func TestApplyAnthropicCacheTTLPreference_RewritesEphemeralBlocks(t *testing.T) {
	body := []byte(`{
	  "model":"claude-sonnet",
	  "messages":[
	    {"role":"user","content":[
	      {"type":"text","text":"a","cache_control":{"type":"ephemeral"}},
	      {"type":"text","text":"b","cache_control":{"type":"ephemeral","ttl":"5m"}},
	      {"type":"text","text":"c","cache_control":{"type":"persistent"}}
	    ]}
	  ]
	}`)

	rewritten, changed := applyAnthropicCacheTTLPreference(body, "1h")
	if !changed {
		t.Fatalf("expected body to be rewritten")
	}

	var payload map[string]any
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("unexpected messages: %#v", payload["messages"])
	}
	msg, _ := messages[0].(map[string]any)
	content, _ := msg["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("unexpected content: %#v", msg["content"])
	}
	for idx, expected := range []string{"1h", "1h", ""} {
		block, _ := content[idx].(map[string]any)
		cacheControl, _ := block["cache_control"].(map[string]any)
		got := strings.TrimSpace(stringFromAny(cacheControl["ttl"]))
		if expected != got {
			t.Fatalf("block %d expected ttl=%q, got %q", idx, expected, got)
		}
	}
}

func TestExecutor_BuildRequest_AnthropicTTL1hAddsBetaHeaderAndBodyTTL(t *testing.T) {
	exec := &Executor{
		st: &fakeUpstreamStore{
			anthropicSecret: store.AnthropicCredentialSecret{
				APIKey: "sk-anthropic",
			},
		},
		upstreamTimeout: 2 * time.Minute,
	}

	body := []byte(`{
	  "model":"claude-sonnet",
	  "messages":[{"role":"user","content":[{"type":"text","text":"warmup","cache_control":{"type":"ephemeral"}}]}]
	}`)
	r := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	req, err := exec.buildRequest(context.Background(), scheduler.Selection{
		BaseURL:              "https://127.0.0.1/v1",
		CredentialType:       scheduler.CredentialTypeAnthropic,
		CredentialID:         1,
		CacheTTLPreference:   "1h",
		HeaderOverride:       `{"anthropic-beta":"context-1m-2025-08-07"}`,
		StatusCodeMapping:    "",
		ParamOverride:        "",
		RequestBodyBlacklist: "",
	}, r, body)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}
	if got := req.Header.Get("anthropic-beta"); !strings.Contains(got, "extended-cache-ttl-2025-04-11") {
		t.Fatalf("expected anthropic-beta to include extended ttl flag, got %q", got)
	}

	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(reqBody, &payload); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	messages, _ := payload["messages"].([]any)
	msg, _ := messages[0].(map[string]any)
	content, _ := msg["content"].([]any)
	block, _ := content[0].(map[string]any)
	cacheControl, _ := block["cache_control"].(map[string]any)
	if got := stringFromAny(cacheControl["ttl"]); got != "1h" {
		t.Fatalf("expected cache_control.ttl=1h, got %q", got)
	}
}

func TestExecutor_Do_OpenAICompat_UnsupportedMaxOutputTokens_RewritesToMaxTokens(t *testing.T) {
	var bodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"bad json"}`))
			return
		}
		bodies = append(bodies, payload)

		if _, ok := payload["max_output_tokens"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: max_output_tokens"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			openaiSecret: store.OpenAICredentialSecret{
				APIKey: "sk-test",
			},
		},
		client: srv.Client(),
	}

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_output_tokens":123,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL,
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   123,
	}, downstream, body)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (%s)", resp.StatusCode, string(b))
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(bodies))
	}
	if _, ok := bodies[0]["max_output_tokens"]; !ok {
		t.Fatalf("expected max_output_tokens in first request, got %#v", bodies[0])
	}
	if _, ok := bodies[1]["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be removed in retry, got %#v", bodies[1])
	}
	if got, ok := bodies[1]["max_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_tokens=123 in retry, got %#v", bodies[1]["max_tokens"])
	}
}

func TestExecutor_Do_OpenAICompat_UnsupportedMaxOutputTokens_SSEErrorBody_RewritesToMaxTokens(t *testing.T) {
	var bodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("data: {\"detail\":\"bad json\"}\n\ndata: [DONE]\n\n"))
			return
		}
		bodies = append(bodies, payload)

		if _, ok := payload["max_output_tokens"]; ok {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("data: {\"detail\":\"Unsupported parameter: max_output_tokens\"}\n\ndata: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			openaiSecret: store.OpenAICredentialSecret{
				APIKey: "sk-test",
			},
		},
		client: srv.Client(),
	}

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_output_tokens":123,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL,
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   123,
	}, downstream, body)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (%s)", resp.StatusCode, string(b))
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(bodies))
	}
	if _, ok := bodies[0]["max_output_tokens"]; !ok {
		t.Fatalf("expected max_output_tokens in first request, got %#v", bodies[0])
	}
	if _, ok := bodies[1]["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be removed in retry, got %#v", bodies[1])
	}
	if got, ok := bodies[1]["max_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_tokens=123 in retry, got %#v", bodies[1]["max_tokens"])
	}
}

func TestExecutor_Do_OpenAICompat_UnsupportedMaxTokens_RewritesToMaxOutputTokens(t *testing.T) {
	var bodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"bad json"}`))
			return
		}
		bodies = append(bodies, payload)

		if _, ok := payload["max_tokens"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: max_tokens"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			openaiSecret: store.OpenAICredentialSecret{
				APIKey: "sk-test",
			},
		},
		client: srv.Client(),
	}

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_tokens":123,"max_output_tokens":999,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL,
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   123,
	}, downstream, body)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (%s)", resp.StatusCode, string(b))
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(bodies))
	}
	if _, ok := bodies[0]["max_tokens"]; !ok {
		t.Fatalf("expected max_tokens in first request, got %#v", bodies[0])
	}
	if _, ok := bodies[1]["max_tokens"]; ok {
		t.Fatalf("expected max_tokens to be removed in retry, got %#v", bodies[1])
	}
	if got, ok := bodies[1]["max_output_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_output_tokens=123 in retry, got %#v", bodies[1]["max_output_tokens"])
	}
}

func TestExecutor_Do_OpenAICompat_UnsupportedMaxTokens_WithSuggestion_RewritesToMaxOutputTokens(t *testing.T) {
	var bodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"bad json"}`))
			return
		}
		bodies = append(bodies, payload)

		if _, ok := payload["max_tokens"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: max_tokens. Please use max_output_tokens instead."}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			openaiSecret: store.OpenAICredentialSecret{
				APIKey: "sk-test",
			},
		},
		client: srv.Client(),
	}

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_tokens":123,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL,
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   123,
	}, downstream, body)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (%s)", resp.StatusCode, string(b))
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(bodies))
	}
	if _, ok := bodies[1]["max_tokens"]; ok {
		t.Fatalf("expected max_tokens to be removed in retry, got %#v", bodies[1]["max_tokens"])
	}
	if got, ok := bodies[1]["max_output_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_output_tokens=123 in retry, got %#v", bodies[1]["max_output_tokens"])
	}
}

func TestExecutor_Do_OpenAICompat_UnsupportedMaxTokens_WhenBodyLacksMaxTokens_DoesNotTryMaxTokensFallback(t *testing.T) {
	var bodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"bad json"}`))
			return
		}
		bodies = append(bodies, payload)

		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: max_tokens"}`))
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			openaiSecret: store.OpenAICredentialSecret{
				APIKey: "sk-test",
			},
		},
		client: srv.Client(),
	}

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_output_tokens":123,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL,
		CredentialType: scheduler.CredentialTypeOpenAI,
		CredentialID:   123,
	}, downstream, body)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d (%s)", resp.StatusCode, string(b))
	}
	if len(bodies) != 1 {
		t.Fatalf("expected 1 request, got %d", len(bodies))
	}
	if _, ok := bodies[0]["max_output_tokens"]; !ok {
		t.Fatalf("expected max_output_tokens in first request, got %#v", bodies[0])
	}
}
