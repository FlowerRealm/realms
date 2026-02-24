package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	setQuotaErrCalls  int
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

func (f *fakeUpstreamStore) SetCodexOAuthAccountQuotaError(_ context.Context, _ int64, _ *string) error {
	f.setQuotaErrCalls++
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := filepath.Join(home, ".opencode", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header.txt"), []byte("cached-instructions"), 0o644); err != nil {
		t.Fatalf("write cache content: %v", err)
	}
	metaBytes, _ := json.Marshal(opencodeCacheMetadata{LastChecked: time.Now().UnixMilli()})
	if err := os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header-meta.json"), metaBytes, 0o644); err != nil {
		t.Fatalf("write cache meta: %v", err)
	}

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

	body := []byte(`{"model":"gpt-5.2","stream":false,"max_output_tokens":123,"prompt_cache_key":"pc1","input":"hi"}`)
	r := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))

	req, err := exec.buildRequest(context.Background(), scheduler.Selection{
		BaseURL:        "https://127.0.0.1/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
		CredentialID:   123,
	}, r, body)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}

	if got := req.URL.Path; got != "/backend-api/codex/responses" {
		t.Fatalf("path = %q, want %q", got, "/backend-api/codex/responses")
	}
	if got := req.Host; got != "chatgpt.com" {
		t.Fatalf("Host = %q, want %q", got, "chatgpt.com")
	}
	if got := req.Header.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("OpenAI-Beta = %q, want %q", got, "responses=experimental")
	}
	if got := req.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q, want %q", got, "text/event-stream")
	}
	if got := req.Header.Get("Originator"); got != "opencode" {
		t.Fatalf("Originator = %q, want %q", got, "opencode")
	}
	if got := req.Header.Get("Chatgpt-Account-Id"); got != "acc" {
		t.Fatalf("Chatgpt-Account-Id = %q, want %q", got, "acc")
	}
	if got := req.Header.Get("Authorization"); got != "Bearer at" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer at")
	}
	if got := req.Header.Get("Conversation_id"); got != "pc1" {
		t.Fatalf("Conversation_id = %q, want %q", got, "pc1")
	}
	if got := req.Header.Get("Session_id"); got != "pc1" {
		t.Fatalf("Session_id = %q, want %q", got, "pc1")
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if v, ok := payload["store"].(bool); !ok || v != false {
		t.Fatalf("store = (%T)%v, want bool(false)", payload["store"], payload["store"])
	}
	if v, ok := payload["stream"].(bool); !ok || v != true {
		t.Fatalf("stream = (%T)%v, want bool(true)", payload["stream"], payload["stream"])
	}
	if _, ok := payload["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be stripped")
	}
	if got := strings.TrimSpace(stringFromAny(payload["instructions"])); got != "cached-instructions" {
		t.Fatalf("instructions = %q, want %q", got, "cached-instructions")
	}
}

func TestCopyCodexOAuthWhitelistedHeaders(t *testing.T) {
	src := make(http.Header)
	src.Set("Accept-Language", "zh-CN")
	src.Set("Content-Type", "application/json")
	src.Set("Conversation_id", "c1")
	src.Set("User-Agent", "ua")
	src.Set("Originator", "x")
	src.Set("Session_id", "s1")
	src.Set("X-Foo", "bar")

	dst := make(http.Header)
	copyCodexOAuthWhitelistedHeaders(dst, src)

	for _, k := range []string{"Accept-Language", "Content-Type", "Conversation_id", "User-Agent", "Originator", "Session_id"} {
		if got := dst.Get(k); got == "" {
			t.Fatalf("expected %s to be copied", k)
		}
	}
	if got := dst.Get("X-Foo"); got != "" {
		t.Fatalf("expected X-Foo to be stripped, got %q", got)
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
