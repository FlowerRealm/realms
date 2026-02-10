package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestTestChannelOnce_OpenAI_ProbesAllBoundModels(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	responsesCalls := 0
	chatCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, prompt, stream, ok := decodeOpenAIProbeRequest(t, r)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		seen[model]++
		if r.URL.Path == "/v1/responses" {
			responsesCalls++
		}
		if r.URL.Path == "/v1/chat/completions" {
			chatCalls++
		}
		mu.Unlock()
		if prompt != "are you ok?" {
			t.Fatalf("expected prompt 'are you ok?', got %q", prompt)
		}
		if !stream {
			t.Fatalf("expected stream=true")
		}
		writeProbeStreamOK(w, r.URL.Path)
	}))
	defer srv.Close()

	st := openTestStore(t)
	ctx := context.Background()

	channelID := createOpenAIChannelWithCredential(t, ctx, st, srv.URL)
	createManagedAndBoundModel(t, ctx, st, channelID, "public-a", "upstream-a")
	createManagedAndBoundModel(t, ctx, st, channelID, "public-b", "")

	ok, _, msg := testChannelOnce(ctx, st, channelID)
	if !ok {
		t.Fatalf("expected ok=true, got false, msg=%q", msg)
	}
	if !strings.Contains(msg, "2/2") {
		t.Fatalf("expected summary contains 2/2, got %q", msg)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen["upstream-a"] != 1 {
		t.Fatalf("expected model upstream-a to be called once, got %d", seen["upstream-a"])
	}
	if seen["public-b"] != 1 {
		t.Fatalf("expected model public-b to be called once, got %d", seen["public-b"])
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 tested models, got %d: %#v", len(seen), seen)
	}
	if responsesCalls != 2 || chatCalls != 0 {
		t.Fatalf("expected responses=2 chat=0, got responses=%d chat=%d", responsesCalls, chatCalls)
	}
}

func TestTestChannelOnce_OpenAI_PartialFailureReturnsDetails(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	responsesCalls := 0
	chatCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, prompt, stream, ok := decodeOpenAIProbeRequest(t, r)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		seen[model]++
		if r.URL.Path == "/v1/responses" {
			responsesCalls++
		}
		if r.URL.Path == "/v1/chat/completions" {
			chatCalls++
		}
		mu.Unlock()
		if prompt != "are you ok?" {
			t.Fatalf("expected prompt 'are you ok?', got %q", prompt)
		}
		if !stream {
			t.Fatalf("expected stream=true")
		}
		if model == "model-bad" {
			http.Error(w, "bad model", http.StatusBadGateway)
			return
		}
		writeProbeStreamOK(w, r.URL.Path)
	}))
	defer srv.Close()

	st := openTestStore(t)
	ctx := context.Background()

	channelID := createOpenAIChannelWithCredential(t, ctx, st, srv.URL)
	createManagedAndBoundModel(t, ctx, st, channelID, "public-ok", "model-ok")
	createManagedAndBoundModel(t, ctx, st, channelID, "public-bad", "model-bad")

	ok, _, msg := testChannelOnce(ctx, st, channelID)
	if ok {
		t.Fatalf("expected ok=false, got true, msg=%q", msg)
	}
	if !strings.Contains(msg, "部分失败") {
		t.Fatalf("expected partial failure message, got %q", msg)
	}
	if !strings.Contains(msg, "model-bad") {
		t.Fatalf("expected failed model name in message, got %q", msg)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen["model-ok"] != 1 || seen["model-bad"] != 2 {
		t.Fatalf("expected model-ok=1 and model-bad=2 (responses+chat fallback), got %#v", seen)
	}
	if responsesCalls != 2 || chatCalls != 1 {
		t.Fatalf("expected responses=2 chat=1, got responses=%d chat=%d", responsesCalls, chatCalls)
	}
}

func TestTestChannelOnce_FallbackToDefaultTestModel(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	responsesCalls := 0
	chatCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, prompt, stream, ok := decodeOpenAIProbeRequest(t, r)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		seen[model]++
		if r.URL.Path == "/v1/responses" {
			responsesCalls++
		}
		if r.URL.Path == "/v1/chat/completions" {
			chatCalls++
		}
		mu.Unlock()
		if prompt != "are you ok?" {
			t.Fatalf("expected prompt 'are you ok?', got %q", prompt)
		}
		if !stream {
			t.Fatalf("expected stream=true")
		}
		if r.URL.Path == "/v1/responses" {
			http.Error(w, "responses not supported", http.StatusNotFound)
			return
		}
		writeProbeStreamOK(w, r.URL.Path)
	}))
	defer srv.Close()

	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, srv.URL)

	testModel := "fallback-model"
	if err := st.UpdateUpstreamChannelNewAPIMeta(ctx, channelID, nil, &testModel, nil, nil, 0, false); err != nil {
		t.Fatalf("UpdateUpstreamChannelNewAPIMeta: %v", err)
	}

	ok, _, msg := testChannelOnce(ctx, st, channelID)
	if !ok {
		t.Fatalf("expected ok=true, got false, msg=%q", msg)
	}
	if !strings.Contains(msg, "默认测试模型") {
		t.Fatalf("expected default test model source in message, got %q", msg)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen["fallback-model"] != 2 {
		t.Fatalf("expected fallback-model tested twice (responses+chat), got %#v", seen)
	}
	if len(seen) != 1 {
		t.Fatalf("expected exactly one tested model, got %#v", seen)
	}
	if responsesCalls != 1 || chatCalls != 1 {
		t.Fatalf("expected responses=1 chat=1 (fallback), got responses=%d chat=%d", responsesCalls, chatCalls)
	}
}

func TestTestChannelOnce_UsesBindingsWithoutManagedModel(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	responsesCalls := 0
	chatCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, prompt, stream, ok := decodeOpenAIProbeRequest(t, r)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		seen[model]++
		if r.URL.Path == "/v1/responses" {
			responsesCalls++
		}
		if r.URL.Path == "/v1/chat/completions" {
			chatCalls++
		}
		mu.Unlock()
		if prompt != "are you ok?" {
			t.Fatalf("expected prompt 'are you ok?', got %q", prompt)
		}
		if !stream {
			t.Fatalf("expected stream=true")
		}
		writeProbeStreamOK(w, r.URL.Path)
	}))
	defer srv.Close()

	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, srv.URL)

	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      "public-only-binding",
		UpstreamModel: "upstream-only-binding",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(public-only-binding): %v", err)
	}

	ok, _, msg := testChannelOnce(ctx, st, channelID)
	if !ok {
		t.Fatalf("expected ok=true, got false, msg=%q", msg)
	}
	if !strings.Contains(msg, "1/1") {
		t.Fatalf("expected summary contains 1/1, got %q", msg)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen["upstream-only-binding"] != 1 {
		t.Fatalf("expected upstream-only-binding tested once, got %#v", seen)
	}
	if responsesCalls != 1 || chatCalls != 0 {
		t.Fatalf("expected responses=1 chat=0, got responses=%d chat=%d", responsesCalls, chatCalls)
	}
}

func decodeOpenAIProbeRequest(t *testing.T, r *http.Request) (model string, prompt string, stream bool, ok bool) {
	t.Helper()
	switch r.URL.Path {
	case "/v1/responses":
		var req map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		model, _ = req["model"].(string)
		stream, _ = req["stream"].(bool)
		prompt = extractResponsesPrompt(req["input"])
		return model, prompt, stream, true
	case "/v1/chat/completions":
		var req struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		msg := ""
		if len(req.Messages) > 0 {
			msg = req.Messages[0].Content
		}
		return req.Model, msg, req.Stream, true
	default:
		return "", "", false, false
	}
}

func extractResponsesPrompt(input any) string {
	if s, ok := input.(string); ok {
		return s
	}
	items, ok := input.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := first["content"].([]any)
	if !ok || len(content) == 0 {
		return ""
	}
	part, ok := content[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := part["text"].(string)
	return text
}

func writeProbeStreamOK(w http.ResponseWriter, path string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	switch path {
	case "/v1/chat/completions":
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	default:
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}
}

func TestBuildUpstreamURL_KeepV1Path(t *testing.T) {
	u, err := buildUpstreamURL("https://example.com/v1", "/v1/responses")
	if err != nil {
		t.Fatalf("buildUpstreamURL error: %v", err)
	}
	if u != "https://example.com/v1/responses" {
		t.Fatalf("expected https://example.com/v1/responses, got %s", u)
	}
}

func TestBuildUpstreamURL_KeepProxyPrefix(t *testing.T) {
	u, err := buildUpstreamURL("https://example.com/proxy/v1", "/v1/chat/completions")
	if err != nil {
		t.Fatalf("buildUpstreamURL error: %v", err)
	}
	if u != "https://example.com/proxy/v1/chat/completions" {
		t.Fatalf("expected https://example.com/proxy/v1/chat/completions, got %s", u)
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)
	return st
}

func createOpenAIChannelWithCredential(t *testing.T, ctx context.Context, st *store.Store, baseURL string) int64 {
	t.Helper()

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "test-channel", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	endpointID, err := st.CreateUpstreamEndpoint(ctx, channelID, baseURL, 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, endpointID, nil, "sk-test"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}
	return channelID
}

func createManagedAndBoundModel(t *testing.T, ctx context.Context, st *store.Store, channelID int64, publicID string, upstreamModel string) {
	t.Helper()

	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            publicID,
		GroupName:           "",
		InputUSDPer1M:       decimal.Zero,
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(%s): %v", publicID, err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      publicID,
		UpstreamModel: upstreamModel,
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(%s): %v", publicID, err)
	}
}
