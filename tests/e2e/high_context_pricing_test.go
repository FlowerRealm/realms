package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/server"
	"realms/internal/store"
	"realms/internal/version"
)

func TestBilling_HighContextPricing_WholeRequest_E2E(t *testing.T) {
	const model = "gpt-5.4"
	fixture := newHighContextFixture(t, model, 300000, 1000)
	ev, got := fixture.callAndFetchDetail(t, "fast")

	if ev.State != store.UsageStateCommitted {
		t.Fatalf("usage_event state mismatch: got=%q want=%q", ev.State, store.UsageStateCommitted)
	}
	if ev.ServiceTier == nil || *ev.ServiceTier != "priority" {
		t.Fatalf("service_tier mismatch: got=%v want=priority", ev.ServiceTier)
	}
	if ev.InputTokens == nil || *ev.InputTokens != 300000 {
		t.Fatalf("input_tokens mismatch: got=%v want=300000", ev.InputTokens)
	}
	if ev.OutputTokens == nil || *ev.OutputTokens != 1000 {
		t.Fatalf("output_tokens mismatch: got=%v want=1000", ev.OutputTokens)
	}

	wantCommitted := decimal.RequireFromString("1.5225")
	if !ev.CommittedUSD.Equal(wantCommitted) {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", ev.CommittedUSD.String(), wantCommitted.String())
	}
	if got.Data.PricingBreakdown.ServiceTier != "priority" {
		t.Fatalf("pricing_breakdown.service_tier=%q, want priority", got.Data.PricingBreakdown.ServiceTier)
	}
	if got.Data.PricingBreakdown.PricingKind != "high_context" {
		t.Fatalf("pricing_kind=%q, want high_context", got.Data.PricingBreakdown.PricingKind)
	}
	if !got.Data.PricingBreakdown.HighContextApplied {
		t.Fatal("expected high_context_applied=true")
	}
	if got.Data.PricingBreakdown.HighContextThresholdTokens != 272000 {
		t.Fatalf("threshold=%d, want 272000", got.Data.PricingBreakdown.HighContextThresholdTokens)
	}
	if got.Data.PricingBreakdown.HighContextTriggerInputTok != 300000 {
		t.Fatalf("trigger_input=%d, want 300000", got.Data.PricingBreakdown.HighContextTriggerInputTok)
	}
	if got.Data.PricingBreakdown.EffectiveServiceTier != "default" {
		t.Fatalf("effective_service_tier=%q, want default", got.Data.PricingBreakdown.EffectiveServiceTier)
	}
	if got.Data.PricingBreakdown.InputUSDPer1M != "5" {
		t.Fatalf("input_usd_per_1m=%q, want 5", got.Data.PricingBreakdown.InputUSDPer1M)
	}
	if got.Data.PricingBreakdown.OutputUSDPer1M != "22.5" {
		t.Fatalf("output_usd_per_1m=%q, want 22.5", got.Data.PricingBreakdown.OutputUSDPer1M)
	}
	if got.Data.PricingBreakdown.FinalCostUSD != "1.5225" {
		t.Fatalf("final_cost_usd=%q, want 1.5225", got.Data.PricingBreakdown.FinalCostUSD)
	}
}

func TestBilling_HighContextPricing_AtThresholdDoesNotTrigger_E2E(t *testing.T) {
	const model = "gpt-5.4"
	fixture := newHighContextFixture(t, model, 272000, 1000)
	ev, got := fixture.callAndFetchDetail(t, "fast")

	wantCommitted := decimal.RequireFromString("2.765")
	if !ev.CommittedUSD.Equal(wantCommitted) {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", ev.CommittedUSD.String(), wantCommitted.String())
	}
	if got.Data.PricingBreakdown.PricingKind != "priority" {
		t.Fatalf("pricing_kind=%q, want priority", got.Data.PricingBreakdown.PricingKind)
	}
	if got.Data.PricingBreakdown.HighContextApplied {
		t.Fatal("expected high_context_applied=false")
	}
	if got.Data.PricingBreakdown.HighContextThresholdTokens != 272000 {
		t.Fatalf("threshold=%d, want 272000", got.Data.PricingBreakdown.HighContextThresholdTokens)
	}
	if got.Data.PricingBreakdown.HighContextTriggerInputTok != 272000 {
		t.Fatalf("trigger_input=%d, want 272000", got.Data.PricingBreakdown.HighContextTriggerInputTok)
	}
	if got.Data.PricingBreakdown.EffectiveServiceTier != "priority" {
		t.Fatalf("effective_service_tier=%q, want priority", got.Data.PricingBreakdown.EffectiveServiceTier)
	}
	if got.Data.PricingBreakdown.InputUSDPer1M != "10" {
		t.Fatalf("input_usd_per_1m=%q, want 10", got.Data.PricingBreakdown.InputUSDPer1M)
	}
	if got.Data.PricingBreakdown.OutputUSDPer1M != "45" {
		t.Fatalf("output_usd_per_1m=%q, want 45", got.Data.PricingBreakdown.OutputUSDPer1M)
	}
	if got.Data.PricingBreakdown.FinalCostUSD != "2.765" {
		t.Fatalf("final_cost_usd=%q, want 2.765", got.Data.PricingBreakdown.FinalCostUSD)
	}
}

func e2eDecimalPtr(v string) *decimal.Decimal {
	d := decimal.RequireFromString(v)
	return &d
}

type highContextFixture struct {
	t        *testing.T
	store    *store.Store
	userID   int64
	rawToken string
	baseURL  string
}

type highContextDetailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		EventID          int64 `json:"event_id"`
		PricingBreakdown struct {
			ServiceTier                string `json:"service_tier"`
			PricingKind                string `json:"pricing_kind"`
			HighContextApplied         bool   `json:"high_context_applied"`
			HighContextThresholdTokens int64  `json:"high_context_threshold_tokens"`
			HighContextTriggerInputTok int64  `json:"high_context_trigger_input_tokens"`
			EffectiveServiceTier       string `json:"effective_service_tier"`
			InputUSDPer1M              string `json:"input_usd_per_1m"`
			OutputUSDPer1M             string `json:"output_usd_per_1m"`
			FinalCostUSD               string `json:"final_cost_usd"`
		} `json:"pricing_breakdown"`
	} `json:"data"`
}

func newHighContextFixture(t *testing.T, model string, inputTokens int64, outputTokens int64) highContextFixture {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = r.Body.Close()

		resp := map[string]any{
			"id":     "resp_high_context_1",
			"object": "response",
			"model":  model,
			"output": []any{
				map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{
						map[string]any{"type": "output_text", "text": "OK"},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			},
			"status": "completed",
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(upstream.Close)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)
	ctx := context.Background()

	const userGroup = "ug1"
	const routeGroup = "rg1"
	if _, err := st.CreateChannelGroup(ctx, routeGroup, nil, 1, store.DefaultGroupPriceMultiplier); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	if err := st.CreateMainGroup(ctx, userGroup, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, userGroup, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ci-upstream", routeGroup, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	epID, err := st.CreateUpstreamEndpoint(ctx, channelID, strings.TrimRight(strings.TrimSpace(upstream.URL), "/")+"/v1", 0)
	if err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if _, _, err := st.CreateOpenAICompatibleCredential(ctx, epID, strPtr("ci"), "sk-upstream-test"); err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:                   model,
		GroupName:                  routeGroup,
		OwnedBy:                    strPtr("openai"),
		InputUSDPer1M:              decimal.RequireFromString("2.5"),
		OutputUSDPer1M:             decimal.RequireFromString("15"),
		CacheInputUSDPer1M:         decimal.RequireFromString("0.25"),
		CacheOutputUSDPer1M:        decimal.Zero,
		PriorityPricingEnabled:     true,
		PriorityInputUSDPer1M:      e2eDecimalPtr("10"),
		PriorityOutputUSDPer1M:     e2eDecimalPtr("45"),
		PriorityCacheInputUSDPer1M: e2eDecimalPtr("1"),
		HighContextPricing: &store.ManagedModelHighContextPricing{
			ThresholdInputTokens: 272000,
			AppliesTo:            store.ManagedModelHighContextAppliesToFullRequest,
			ServiceTierPolicy:    store.ManagedModelHighContextServiceTierPolicyForceStandard,
			InputUSDPer1M:        decimal.RequireFromString("5"),
			OutputUSDPer1M:       decimal.RequireFromString("22.5"),
			CacheInputUSDPer1M:   e2eDecimalPtr("0.5"),
			Source:               "openai_official",
			SourceDetail:         "openai_official_pricing_docs",
		},
		Status: 1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      model,
		UpstreamModel: model,
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel: %v", err)
	}

	userID, err := st.CreateUser(ctx, "ci-high-context@example.com", "cihighctx", []byte("x"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, userGroup); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("20")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	rawToken, err := auth.NewRandomToken("sk_", 32)
	if err != nil {
		t.Fatalf("NewRandomToken: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, strPtr("ci-token"), rawToken)
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{routeGroup}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	appCfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	appCfg.Env = "dev"
	appCfg.Mode = config.ModeBusiness
	appCfg.DB.Driver = "sqlite"
	appCfg.DB.DSN = ""
	appCfg.DB.SQLitePath = dbPath
	appCfg.Billing.EnablePayAsYouGo = true

	app, err := server.NewApp(server.AppOptions{
		Config:  appCfg,
		DB:      db,
		Version: version.Info(),
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)

	return highContextFixture{
		t:        t,
		store:    st,
		userID:   userID,
		rawToken: rawToken,
		baseURL:  ts.URL,
	}
}

func (f highContextFixture) callAndFetchDetail(t *testing.T, serviceTier string) (store.UsageEvent, highContextDetailResponse) {
	t.Helper()

	reqBody := []byte(`{"model":"gpt-5.4","input":"hi","stream":false,"service_tier":"` + serviceTier + `"}`)
	req, err := http.NewRequest(http.MethodPost, f.baseURL+"/v1/responses", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.rawToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(bodyBytes))
	}

	events := waitUsageEventsByUser(t, f.store, context.Background(), f.userID, 1)
	ev := events[0]

	detailReq, err := http.NewRequest(http.MethodGet, f.baseURL+"/v1/usage/events/"+strconv.FormatInt(ev.ID, 10)+"/detail", nil)
	if err != nil {
		t.Fatalf("NewRequest(detail): %v", err)
	}
	detailReq.Header.Set("Authorization", "Bearer "+f.rawToken)

	detailResp, err := client.Do(detailReq)
	if err != nil {
		t.Fatalf("Do(detail): %v", err)
	}
	detailBody, _ := io.ReadAll(io.LimitReader(detailResp.Body, 1<<20))
	_ = detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected detail status: %d body=%s", detailResp.StatusCode, string(detailBody))
	}

	var got highContextDetailResponse
	if err := json.Unmarshal(detailBody, &got); err != nil {
		t.Fatalf("json.Unmarshal(detail): %v", err)
	}
	if !got.Success {
		t.Fatalf("expected detail success, got message=%q", got.Message)
	}
	if got.Data.EventID != ev.ID {
		t.Fatalf("detail event_id mismatch: got=%d want=%d", got.Data.EventID, ev.ID)
	}
	return ev, got
}
