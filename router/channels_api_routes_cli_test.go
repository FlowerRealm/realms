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
	"testing"

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
