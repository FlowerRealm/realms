package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

// TestChannelTypeToCLIType verifies the mapping from channel type to CLI runner cli_type.
func TestChannelTypeToCLIType(t *testing.T) {
	tests := []struct {
		chType string
		want   string
	}{
		{"openai_compatible", "codex"},
		{"anthropic", "claude"},
		{"codex_oauth", "codex_oauth"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := channelTypeToCLIType(tt.chType)
		if got != tt.want {
			t.Errorf("channelTypeToCLIType(%q) = %q, want %q", tt.chType, got, tt.want)
		}
	}
}

// TestCLITestDelegation starts a fake CLI runner, wires up the handler,
// and verifies that the SSE stream returns the expected events without
// writing to the database (no UpdateUpstreamChannelTest).
func TestCLITestDelegation(t *testing.T) {
	// Fake CLI runner: accepts POST /v1/test and returns a canned response.
	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			CLIType string `json:"cli_type"`
			BaseURL string `json:"base_url"`
			APIKey  string `json:"api_key"`
			Model   string `json:"model"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.CLIType == "" || req.APIKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 123,
			"output":     "OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	ctx := context.Background()

	// Create an OpenAI-compatible channel with a credential.
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")

	// Record the initial last_test_at so we can verify it did NOT change.
	chBefore, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}

	// Build a test Gin engine with CLI runner URL configured.
	opts := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: fakeRunner.URL,
	}

	// Directly invoke the handler function to test SSE output.
	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	c.Params = gin.Params{{Key: "channel_id", Value: fmt.Sprintf("%d", channelID)}}

	streamChannelCLITestHandler(c, opts, channelID)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Verify SSE events.
	if !strings.Contains(bodyStr, "event: start") {
		t.Error("expected SSE 'start' event")
	}
	if !strings.Contains(bodyStr, "event: model_done") {
		t.Error("expected SSE 'model_done' event")
	}
	if !strings.Contains(bodyStr, "event: summary") {
		t.Error("expected SSE 'summary' event")
	}
	if !strings.Contains(bodyStr, `"cli_runner"`) {
		t.Error("expected source to be 'cli_runner'")
	}

	// CRITICAL: verify that last_test_at was NOT updated.
	chAfter, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID after: %v", err)
	}
	if chBefore.LastTestAt == nil && chAfter.LastTestAt != nil {
		t.Errorf("expected last_test_at to remain nil (not written to DB), got %v", chAfter.LastTestAt)
	}
	if chBefore.LastTestAt != nil && chAfter.LastTestAt != nil && !chBefore.LastTestAt.Equal(*chAfter.LastTestAt) {
		t.Errorf("expected last_test_at to be unchanged, before=%v after=%v", chBefore.LastTestAt, chAfter.LastTestAt)
	}
}

func TestChannelTest_CodexOAuth_Supported(t *testing.T) {
	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			CLIType          string `json:"cli_type"`
			BaseURL          string `json:"base_url"`
			ProfileKey       string `json:"profile_key"`
			ChatGPTAccountID string `json:"chatgpt_account_id"`
			AccessToken      string `json:"access_token"`
			RefreshToken     string `json:"refresh_token"`
			IDToken          string `json:"id_token"`
			Model            string `json:"model"`
			Prompt           string `json:"prompt"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.CLIType != "codex_oauth" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ChatGPTAccountID) != "acc_test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.AccessToken) != "at" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.RefreshToken) != "rt" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.IDToken) != "it" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.BaseURL) != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ProfileKey) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 123,
			"output":     "OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	ctx := context.Background()

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "test-codex", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	endpointID, err := st.CreateUpstreamEndpoint(ctx, channelID, "https://chatgpt.com/backend-api/codex", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	expiresAt := time.Now().Add(1 * time.Hour)
	idToken := "it"
	if _, err := st.CreateCodexOAuthAccount(ctx, endpointID, "acc_test", nil, "at", "rt", &idToken, &expiresAt); err != nil {
		t.Fatalf("CreateCodexOAuthAccount: %v", err)
	}

	chBefore, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}

	ep, err := st.GetUpstreamEndpointByChannelID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamEndpointByChannelID: %v", err)
	}

	opts := Options{Store: st, ChannelTestCLIRunnerURL: fakeRunner.URL}

	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	c.Params = gin.Params{{Key: "channel_id", Value: fmt.Sprintf("%d", channelID)}}

	streamChannelCLITestHandler(c, opts, channelID)

	resp := w.Result()
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if !strings.Contains(bodyStr, "event: start") {
		t.Error("expected SSE 'start' event")
	}
	if !strings.Contains(bodyStr, "event: model_done") {
		t.Error("expected SSE 'model_done' event")
	}
	if !strings.Contains(bodyStr, "event: summary") {
		t.Error("expected SSE 'summary' event")
	}
	if !strings.Contains(bodyStr, `"success":true`) {
		t.Fatalf("expected success=true in summary, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"cli_runner"`) {
		t.Fatalf("expected source=cli_runner, got: %s", bodyStr)
	}

	chAfter, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID after: %v", err)
	}
	if chBefore.LastTestAt == nil && chAfter.LastTestAt != nil {
		t.Errorf("expected last_test_at to remain nil (not written to DB), got %v", chAfter.LastTestAt)
	}
	if chBefore.LastTestAt != nil && chAfter.LastTestAt != nil && !chBefore.LastTestAt.Equal(*chAfter.LastTestAt) {
		t.Errorf("expected last_test_at to be unchanged, before=%v after=%v", chBefore.LastTestAt, chAfter.LastTestAt)
	}

	if strings.TrimRight(strings.TrimSpace(ep.BaseURL), "/") == "" {
		t.Fatalf("expected endpoint base_url to be set")
	}
}

func TestCLITestDelegation_ConcurrentModels(t *testing.T) {
	const wantConcurrency = 4

	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0

	release := make(chan struct{})
	var once sync.Once
	go func() {
		time.Sleep(150 * time.Millisecond)
		once.Do(func() { close(release) })
	}()

	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			CLIType string `json:"cli_type"`
			APIKey  string `json:"api_key"`
			Model   string `json:"model"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.CLIType == "" || req.APIKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		if inFlight >= wantConcurrency {
			once.Do(func() { close(release) })
		}
		mu.Unlock()

		<-release
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 10,
			"output":     "OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")

	for i := 0; i < 8; i++ {
		publicID := fmt.Sprintf("m%d", i+1)
		if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
			ChannelID:     channelID,
			PublicID:      publicID,
			UpstreamModel: publicID,
			Status:        1,
		}); err != nil {
			t.Fatalf("CreateChannelModel(%s): %v", publicID, err)
		}
	}

	opts := Options{
		Store:                     st,
		ChannelTestCLIRunnerURL:   fakeRunner.URL,
		ChannelTestCLIConcurrency: wantConcurrency,
	}

	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	c.Params = gin.Params{{Key: "channel_id", Value: fmt.Sprintf("%d", channelID)}}

	streamChannelCLITestHandler(c, opts, channelID)

	mu.Lock()
	gotMax := maxInFlight
	mu.Unlock()
	if gotMax < 2 {
		t.Fatalf("expected concurrent runner requests, got max_in_flight=%d", gotMax)
	}
}

// TestCLITestRunnerURLEmpty verifies that streamChannelCLITestHandler returns
// an error summary when ChannelTestCLIRunnerURL is empty.
func TestCLITestRunnerURLEmpty(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")

	opts := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: "",
	}

	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	c.Params = gin.Params{{Key: "channel_id", Value: fmt.Sprintf("%d", channelID)}}

	streamChannelCLITestHandler(c, opts, channelID)

	body := w.Body.String()
	if !strings.Contains(body, "event: summary") {
		t.Fatalf("expected SSE summary event, got: %s", body)
	}
	if !strings.Contains(body, "CLI runner") {
		t.Fatalf("expected error message to mention CLI runner, got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestGinContext(w *httptest.ResponseRecorder) (*gin.Context, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, nil
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
