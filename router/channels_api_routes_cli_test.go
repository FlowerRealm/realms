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

	"realms/internal/channeltest"
	"realms/internal/scheduler"
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

func TestCLITestDelegation(t *testing.T) {
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
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")

	chBefore, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}

	opts := Options{Store: st, ChannelTestCLIRunnerURL: fakeRunner.URL}
	startedAt := time.Now()
	ok, latencyMS, message, summary := runChannelCLITest(ctx, opts, channelID)
	elapsedMS := int(time.Since(startedAt) / time.Millisecond)

	if !ok {
		t.Fatalf("expected ok=true, message=%s", message)
	}
	if summary.LatencyMS != latencyMS {
		t.Fatalf("expected summary latency to match top-level latency, got summary=%d top=%d", summary.LatencyMS, latencyMS)
	}
	if latencyMS > elapsedMS+100 {
		t.Fatalf("expected latency to track wall-clock time, got latency=%d elapsed=%d", latencyMS, elapsedMS)
	}
	if summary.Source != "cli_runner" {
		t.Fatalf("expected source=cli_runner, got %q", summary.Source)
	}
	if summary.Total != 1 || summary.Success != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if len(summary.Results) != 1 || !summary.Results[0].OK {
		t.Fatalf("unexpected results: %+v", summary.Results)
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
		if req.CLIType != "codex_oauth" || strings.TrimSpace(req.ChatGPTAccountID) != "acc_test" || strings.TrimSpace(req.AccessToken) != "at" || strings.TrimSpace(req.RefreshToken) != "rt" || strings.TrimSpace(req.IDToken) != "it" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.BaseURL) != "" || strings.TrimSpace(req.ProfileKey) == "" || strings.TrimSpace(req.Prompt) == "" {
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
	ok, _, _, summary := runChannelCLITest(ctx, opts, channelID)
	if !ok {
		t.Fatalf("expected success summary, got %+v", summary)
	}
	if summary.Source != "cli_runner" || summary.Total != 1 || summary.Success != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
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

	ok, _, _, summary := runChannelCLITest(ctx, opts, channelID)
	if !ok {
		t.Fatalf("expected concurrent test to succeed, got %+v", summary)
	}

	mu.Lock()
	gotMax := maxInFlight
	mu.Unlock()
	if gotMax < 2 {
		t.Fatalf("expected concurrent runner requests, got max_in_flight=%d", gotMax)
	}
}

func TestCLITestDelegation_ReportsWallClockLatency(t *testing.T) {
	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 500,
			"output":     "OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")
	for i := 0; i < 4; i++ {
		modelID := fmt.Sprintf("latency-%d", i+1)
		if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
			ChannelID:     channelID,
			PublicID:      modelID,
			UpstreamModel: modelID,
			Status:        1,
		}); err != nil {
			t.Fatalf("CreateChannelModel(%s): %v", modelID, err)
		}
	}

	opts := Options{
		Store:                     st,
		ChannelTestCLIRunnerURL:   fakeRunner.URL,
		ChannelTestCLIConcurrency: 4,
	}

	startedAt := time.Now()
	ok, latencyMS, _, summary := runChannelCLITest(ctx, opts, channelID)
	elapsedMS := int(time.Since(startedAt) / time.Millisecond)

	if !ok {
		t.Fatalf("expected success summary, got %+v", summary)
	}
	if summary.LatencyMS != latencyMS {
		t.Fatalf("expected summary latency to match top-level latency, got summary=%d top=%d", summary.LatencyMS, latencyMS)
	}
	if latencyMS > elapsedMS+100 {
		t.Fatalf("expected latency to track wall-clock time, got latency=%d elapsed=%d", latencyMS, elapsedMS)
	}
}

func TestCLITestDelegation_RunnerFailureDoesNotWaitForProbe(t *testing.T) {
	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "runner boom",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	baseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	channelID := createOpenAIChannelWithCredential(t, baseCtx, st, "https://api.example.com")

	probeCanceled := make(chan struct{})
	opts := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: fakeRunner.URL,
		ChannelTestProbe: fakeChannelTestProber{probe: func(ctx context.Context, _ scheduler.Selection, _ string) (channeltest.Result, error) {
			select {
			case <-ctx.Done():
				close(probeCanceled)
				return channeltest.Result{}, ctx.Err()
			case <-time.After(2 * time.Second):
				return channeltest.Result{}, nil
			}
		}},
	}

	startedAt := time.Now()
	ok, _, message, summary := runChannelCLITest(baseCtx, opts, channelID)
	elapsed := time.Since(startedAt)

	if ok {
		t.Fatalf("expected failure summary, got %+v", summary)
	}
	if !strings.Contains(message, "runner boom") {
		t.Fatalf("expected runner error in message, got %q", message)
	}
	if elapsed >= 700*time.Millisecond {
		t.Fatalf("expected runner failure to return before probe timeout, elapsed=%s summary=%+v", elapsed, summary)
	}
	select {
	case <-probeCanceled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected probe context to be canceled on runner failure")
	}
}

func TestCLITestDelegation_ModelCheckWarning(t *testing.T) {
	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 42,
			"ttft_ms":    9,
			"output":     "partial OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      "alias",
		UpstreamModel: "gpt-5.2",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel: %v", err)
	}

	opts := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: fakeRunner.URL,
		ChannelTestProbe: fakeChannelTestProber{probe: func(_ context.Context, sel scheduler.Selection, model string) (channeltest.Result, error) {
			if sel.CredentialType != scheduler.CredentialTypeOpenAI {
				t.Fatalf("unexpected credential type: %s", sel.CredentialType)
			}
			if model != "gpt-5.2" {
				t.Fatalf("unexpected model: %s", model)
			}
			return channeltest.Result{
				ForwardedModel:        "gpt-5.2",
				UpstreamResponseModel: "gpt-5.2-mini",
				SuccessPath:           "/v1/responses",
			}, nil
		}},
	}

	ok, _, _, summary := runChannelCLITest(ctx, opts, channelID)
	if !ok {
		t.Fatalf("expected test success despite mismatch warning, got %+v", summary)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("expected 1 result, got %+v", summary.Results)
	}
	result := summary.Results[0]
	if result.ForwardedModel != "gpt-5.2" {
		t.Fatalf("expected forwarded_model=gpt-5.2, got %+v", result)
	}
	if result.UpstreamResponseModel != "gpt-5.2-mini" {
		t.Fatalf("expected upstream_response_model=gpt-5.2-mini, got %+v", result)
	}
	if result.ModelCheckStatus != "mismatch" {
		t.Fatalf("expected mismatch model_check_status, got %+v", result)
	}
	if summary.ModelCheckMismatch != 1 {
		t.Fatalf("expected summary mismatch count, got %+v", summary)
	}
	if result.SuccessPath != "/v1/responses" {
		t.Fatalf("expected success_path from probe, got %+v", result)
	}
}

func TestCLITestRunnerURLEmpty(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")

	opts := Options{Store: st, ChannelTestCLIRunnerURL: ""}
	ok, _, message, summary := runChannelCLITest(ctx, opts, channelID)
	if ok {
		t.Fatalf("expected failure when runner URL empty, got %+v", summary)
	}
	if !strings.Contains(message, "CLI runner") {
		t.Fatalf("expected error message to mention CLI runner, got: %s", message)
	}
}

func TestChannelTestHandler_StreamParamStillReturnsJSON(t *testing.T) {
	fakeRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"latency_ms": 12,
			"output":     "OK",
			"error":      "",
		})
	}))
	defer fakeRunner.Close()

	st := openTestStore(t)
	ctx := context.Background()
	channelID := createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")
	_ = ctx

	opts := Options{Store: st, ChannelTestCLIRunnerURL: fakeRunner.URL}

	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/test/%d?stream=1", channelID), nil)
	c.Params = gin.Params{{Key: "channel_id", Value: fmt.Sprintf("%d", channelID)}}

	testChannelHandler(opts)(c)

	resp := w.Result()
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json response, got content-type=%q", got)
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Probe channelProbeSummary `json:"probe"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Probe.Total != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type fakeChannelTestProber struct {
	probe func(ctx context.Context, sel scheduler.Selection, model string) (channeltest.Result, error)
}

func (f fakeChannelTestProber) Probe(ctx context.Context, sel scheduler.Selection, model string) (channeltest.Result, error) {
	return f.probe(ctx, sel, model)
}

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
