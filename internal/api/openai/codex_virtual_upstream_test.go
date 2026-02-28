package openai

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

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"
)

type codexQuotaCaptureStore struct {
	secret store.CodexOAuthSecret

	patchCh chan store.CodexOAuthQuotaPatch
}

func (s *codexQuotaCaptureStore) GetOpenAICompatibleCredentialSecret(_ context.Context, _ int64) (store.OpenAICredentialSecret, error) {
	return store.OpenAICredentialSecret{}, nil
}

func (s *codexQuotaCaptureStore) GetAnthropicCredentialSecret(_ context.Context, _ int64) (store.AnthropicCredentialSecret, error) {
	return store.AnthropicCredentialSecret{}, nil
}

func (s *codexQuotaCaptureStore) GetCodexOAuthSecret(_ context.Context, accountID int64) (store.CodexOAuthSecret, error) {
	sec := s.secret
	sec.ID = accountID
	return sec, nil
}

func (s *codexQuotaCaptureStore) UpdateCodexOAuthAccountTokens(_ context.Context, _ int64, _, _ string, _ *string, _ *time.Time) error {
	return nil
}

func (s *codexQuotaCaptureStore) SetCodexOAuthAccountStatus(_ context.Context, _ int64, _ int) error {
	return nil
}

func (s *codexQuotaCaptureStore) SetCodexOAuthAccountCooldown(_ context.Context, _ int64, _ time.Time) error {
	return nil
}

func (s *codexQuotaCaptureStore) SetCodexOAuthAccountQuotaError(_ context.Context, _ int64, _ *string) error {
	return nil
}

func (s *codexQuotaCaptureStore) PatchCodexOAuthAccountQuota(_ context.Context, _ int64, patch store.CodexOAuthQuotaPatch, _ time.Time) error {
	if s.patchCh == nil {
		return nil
	}
	select {
	case s.patchCh <- patch:
	default:
	}
	return nil
}

func setupOpencodeCodexInstructionsCache(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := filepath.Join(home, ".opencode", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header.txt"), []byte("cached-instructions"), 0o644); err != nil {
		t.Fatalf("write cache content: %v", err)
	}
	metaBytes, _ := json.Marshal(map[string]any{"last_checked": time.Now().UnixMilli()})
	if err := os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header-meta.json"), metaBytes, 0o644); err != nil {
		t.Fatalf("write cache meta: %v", err)
	}
}

func TestCodexVirtualUpstream_NonStream_PatchesQuotaAndForwardsXCodexHeaders(t *testing.T) {
	setupOpencodeCodexInstructionsCache(t)

	patchCh := make(chan store.CodexOAuthQuotaPatch, 1)
	st := &codexQuotaCaptureStore{
		secret: store.CodexOAuthSecret{
			AccountID:    "chatgpt-acc",
			AccessToken:  "at",
			RefreshToken: "rt",
		},
		patchCh: patchCh,
	}

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-codex-primary-used-percent", "80")
		w.Header().Set("x-codex-primary-reset-after-seconds", "1000")
		w.Header().Set("x-codex-primary-window-minutes", "10080")
		w.Header().Set("x-codex-secondary-used-percent", "20")
		w.Header().Set("x-codex-secondary-reset-after-seconds", "200")
		w.Header().Set("x-codex-secondary-window-minutes", "300")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":1,"output_tokens":2}}`))
	}))
	defer up.Close()

	exec := upstream.NewExecutorForTests(st, up.Client())

	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: up.URL + "/backend-api/codex", Status: 1},
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
	h := NewHandler(fs, fs, sched, exec, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{}, nil)

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

	if got := rr.Header().Get("x-codex-primary-used-percent"); got != "80" {
		t.Fatalf("response header x-codex-primary-used-percent=%q, want %q", got, "80")
	}
	if got := rr.Header().Get("x-codex-secondary-used-percent"); got != "20" {
		t.Fatalf("response header x-codex-secondary-used-percent=%q, want %q", got, "20")
	}

	select {
	case patch := <-patchCh:
		// secondary window is 5h => quota_primary
		if patch.PrimaryUsedPercent == nil || *patch.PrimaryUsedPercent != 20 {
			t.Fatalf("PrimaryUsedPercent=%v, want 20", patch.PrimaryUsedPercent)
		}
		if patch.SecondaryUsedPercent == nil || *patch.SecondaryUsedPercent != 80 {
			t.Fatalf("SecondaryUsedPercent=%v, want 80", patch.SecondaryUsedPercent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for quota patch")
	}
}

func TestCodexVirtualUpstream_Stream_PatchesQuota(t *testing.T) {
	setupOpencodeCodexInstructionsCache(t)

	patchCh := make(chan store.CodexOAuthQuotaPatch, 1)
	st := &codexQuotaCaptureStore{
		secret: store.CodexOAuthSecret{
			AccountID:    "chatgpt-acc",
			AccessToken:  "at",
			RefreshToken: "rt",
		},
		patchCh: patchCh,
	}

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-codex-primary-used-percent", "12")
		w.Header().Set("x-codex-primary-reset-after-seconds", "604800")
		w.Header().Set("x-codex-primary-window-minutes", "10080")
		w.Header().Set("x-codex-secondary-used-percent", "34")
		w.Header().Set("x-codex-secondary-reset-after-seconds", "3600")
		w.Header().Set("x-codex-secondary-window-minutes", "300")

		sse := strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"hi"}`,
			"",
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":4}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		_, _ = io.WriteString(w, sse)
	}))
	defer up.Close()

	exec := upstream.NewExecutorForTests(st, up.Client())

	fs := &fakeStore{
		channels: []store.UpstreamChannel{
			{ID: 1, Type: store.UpstreamTypeCodexOAuth, Status: 1},
		},
		endpoints: map[int64][]store.UpstreamEndpoint{
			1: {
				{ID: 11, ChannelID: 1, BaseURL: up.URL + "/backend-api/codex", Status: 1},
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
	h := NewHandler(fs, fs, sched, exec, nil, nil, false, nil, fakeAudit{}, nil, nil, upstream.SSEPumpOptions{MaxLineBytes: 256 << 10, InitialLineBytes: 64 << 10}, nil)

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
	if !strings.Contains(rr.Body.String(), "[DONE]") {
		t.Fatalf("expected streamed body to contain [DONE], got=%s", rr.Body.String())
	}

	select {
	case patch := <-patchCh:
		// secondary window is 5h => quota_primary
		if patch.PrimaryUsedPercent == nil || *patch.PrimaryUsedPercent != 34 {
			t.Fatalf("PrimaryUsedPercent=%v, want 34", patch.PrimaryUsedPercent)
		}
		if patch.SecondaryUsedPercent == nil || *patch.SecondaryUsedPercent != 12 {
			t.Fatalf("SecondaryUsedPercent=%v, want 12", patch.SecondaryUsedPercent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for quota patch")
	}
}
