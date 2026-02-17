package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestChannelTypeToCLIType verifies the mapping from channel type to CLI runner cli_type.
func TestChannelTypeToCLIType(t *testing.T) {
	tests := []struct {
		chType string
		want   string
	}{
		{"openai_compatible", "codex"},
		{"anthropic", "claude"},
		{"codex_oauth", ""},
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

// TestChannelsPage_CLITestAvailable verifies that the channel page response
// includes cli_test_available=true when runner URL is configured,
// and cli_test_available=false when not.
func TestChannelsPage_CLITestAvailable(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	_ = createOpenAIChannelWithCredential(t, ctx, st, "https://api.example.com")

	// Test with CLI runner configured.
	opts := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: "http://localhost:3100",
	}
	handler := channelsPageHandler(opts)

	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	handler(c)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			CLITestAvailable bool `json:"cli_test_available"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success=true")
	}
	if !result.Data.CLITestAvailable {
		t.Error("expected cli_test_available=true when runner URL is configured")
	}

	// Test without CLI runner.
	opts2 := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: "",
	}
	handler2 := channelsPageHandler(opts2)

	w2 := httptest.NewRecorder()
	c2, _ := newTestGinContext(w2)
	handler2(c2)

	var result2 struct {
		Success bool `json:"success"`
		Data    struct {
			CLITestAvailable bool `json:"cli_test_available"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&result2); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result2.Success {
		t.Fatal("expected success=true")
	}
	if result2.Data.CLITestAvailable {
		t.Error("expected cli_test_available=false when runner URL is empty")
	}
}

// TestTestAllChannelsHandler_DisabledInCLIMode verifies that testAllChannelsHandler
// returns 405 when CLI runner is configured.
func TestTestAllChannelsHandler_DisabledInCLIMode(t *testing.T) {
	st := openTestStore(t)

	opts := Options{
		Store:                   st,
		ChannelTestCLIRunnerURL: "http://localhost:3100",
	}
	handler := testAllChannelsHandler(opts)

	w := httptest.NewRecorder()
	c, _ := newTestGinContext(w)
	handler(c)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
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
