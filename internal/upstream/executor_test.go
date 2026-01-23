package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

type fakeUpstreamStore struct {
	codexSecret     store.CodexOAuthSecret
	openaiSecret    store.OpenAICredentialSecret
	anthropicSecret store.AnthropicCredentialSecret

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

func TestExecutor_CodexOAuthRequestPassthrough_LeavesPathAndBody(t *testing.T) {
	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		upstreamTimeout:         2 * time.Minute,
		codexRequestPassthrough: true,
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

func TestExecutor_CodexOAuthRequestPassthrough_Disabled_RewritesPathAndBody(t *testing.T) {
	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		upstreamTimeout:         2 * time.Minute,
		codexRequestPassthrough: false,
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

	if got := req.URL.Path; got != "/backend-api/codex/responses" {
		t.Fatalf("expected rewritten path, got %q", got)
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}
	if v, ok := payload["stream"].(bool); !ok || !v {
		t.Fatalf("expected stream=true after rewrite, got %#v", payload["stream"])
	}
	if v, ok := payload["store"].(bool); !ok || v {
		t.Fatalf("expected store=false after rewrite, got %#v", payload["store"])
	}
	if v, ok := payload["parallel_tool_calls"].(bool); !ok || !v {
		t.Fatalf("expected parallel_tool_calls=true after rewrite, got %#v", payload["parallel_tool_calls"])
	}
	if _, ok := payload["max_output_tokens"]; ok {
		t.Fatalf("expected max_output_tokens to be stripped after rewrite")
	}
}

func TestExecutor_Do_CodexOAuth_Passthrough404_FallsBackToLegacyResponsesPath(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/backend-api/codex/v1/responses":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
		case "/backend-api/codex/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		client:                  srv.Client(),
		codexRequestPassthrough: true,
	}

	body := []byte(`{"model":"gpt-5.2","stream":true,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL + "/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
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
	if len(paths) != 2 || paths[0] != "/backend-api/codex/v1/responses" || paths[1] != "/backend-api/codex/responses" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestExecutor_Do_CodexOAuth_Passthrough400UnsupportedParam_FallsBackToLegacyResponsesPath(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/backend-api/codex/v1/responses":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: max_output_tokens"}`))
		case "/backend-api/codex/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		client:                  srv.Client(),
		codexRequestPassthrough: true,
	}

	body := []byte(`{"model":"gpt-5.2","stream":true,"max_output_tokens":123,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL + "/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
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
	if len(paths) != 2 || paths[0] != "/backend-api/codex/v1/responses" || paths[1] != "/backend-api/codex/responses" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestExecutor_Do_CodexOAuth_Passthrough403HTML_FallsBackToLegacyResponsesPath(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/backend-api/codex/v1/responses":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><head><title>Forbidden</title></head><body>nope</body></html>"))
		case "/backend-api/codex/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: [DONE]\n\n"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		client:                  srv.Client(),
		codexRequestPassthrough: true,
	}

	body := []byte(`{"model":"gpt-5.2","stream":true,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL + "/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
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
	if len(paths) != 2 || paths[0] != "/backend-api/codex/v1/responses" || paths[1] != "/backend-api/codex/responses" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestExecutor_Do_CodexOAuth_Passthrough404_IfFallbackAlso404_ReturnsOriginal(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/backend-api/codex/v1/responses":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("first"))
		case "/backend-api/codex/responses":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("second"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		client:                  srv.Client(),
		codexRequestPassthrough: true,
	}

	body := []byte(`{"model":"gpt-5.2","stream":true,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL + "/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
		CredentialID:   123,
	}, downstream, body)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "first" {
		t.Fatalf("expected original body to be returned, got %q", string(b))
	}
	if len(paths) != 2 || paths[0] != "/backend-api/codex/v1/responses" || paths[1] != "/backend-api/codex/responses" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestExecutor_Do_CodexOAuth_Legacy404_FallsBackToPassthroughResponsesPath(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/backend-api/codex/responses":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("legacy"))
		case "/backend-api/codex/v1/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {}\n\n"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	exec := &Executor{
		st: &fakeUpstreamStore{
			codexSecret: store.CodexOAuthSecret{
				AccountID:    "acc",
				AccessToken:  "at",
				RefreshToken: "rt",
			},
		},
		client:                  srv.Client(),
		codexRequestPassthrough: false,
	}

	body := []byte(`{"model":"gpt-5.2","stream":true,"input":"hi"}`)
	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", bytes.NewReader(body))
	resp, err := exec.Do(context.Background(), scheduler.Selection{
		BaseURL:        srv.URL + "/backend-api/codex",
		CredentialType: scheduler.CredentialTypeCodex,
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
	if len(paths) != 2 || paths[0] != "/backend-api/codex/responses" || paths[1] != "/backend-api/codex/v1/responses" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}
