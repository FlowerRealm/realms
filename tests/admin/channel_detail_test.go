package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/admin"
	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/store"
)

func TestChannelDetail_RendersKeyUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	ctx := context.Background()
	userID, err := st.CreateUser(ctx, "root@example.com", "root", []byte("x"), store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	ep, err := st.SetUpstreamEndpointBaseURL(ctx, channelID, "https://api.openai.com")
	if err != nil {
		t.Fatalf("SetUpstreamEndpointBaseURL: %v", err)
	}
	credID, _, err := st.CreateOpenAICompatibleCredential(ctx, ep.ID, nil, "sk-test-abcdef123456")
	if err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
	}

	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID:        "req_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            func() *string { s := "m1"; return &s }(),
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}
	inTok := int64(10)
	outTok := int64(5)
	if err := st.CommitUsage(ctx, store.CommitUsageInput{
		UsageEventID:      usageID,
		UpstreamChannelID: &channelID,
		InputTokens:       &inTok,
		OutputTokens:      &outTok,
		CommittedUSD:      decimal.Zero,
	}); err != nil {
		t.Fatalf("CommitUsage: %v", err)
	}
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:       usageID,
		Endpoint:           "/v1/responses",
		Method:             "POST",
		StatusCode:         200,
		LatencyMS:          10,
		UpstreamChannelID:  &channelID,
		UpstreamEndpointID: &ep.ID,
		UpstreamCredID:     &credID,
		IsStream:           false,
		RequestBytes:       123,
		ResponseBytes:      456,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent: %v", err)
	}

	// 先确认统计 SQL 在该库上可用（避免模板渲染时才暴露问题）。
	if _, err := st.GetUsageStatsByCredentialForChannelRange(ctx, channelID, time.Now().UTC().Add(-time.Hour), time.Now().UTC()); err != nil {
		t.Fatalf("GetUsageStatsByCredentialForChannelRange: %v", err)
	}

	srv, err := admin.NewServer(
		st,
		nil,
		nil,
		false,
		false,
		config.SMTPConfig{},
		config.BillingConfig{},
		config.PaymentConfig{},
		"http://example.com",
		"UTC",
		false,
		nil,
		config.TicketsConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/channels/"+strconv.FormatInt(channelID, 10)+"/detail?window=1h", nil)
	req.SetPathValue("channel_id", strconv.FormatInt(channelID, 10))
	csrf := "csrf"
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{
		ActorType: auth.ActorTypeSession,
		UserID:    userID,
		CSRFToken: &csrf,
	}))

	rr := httptest.NewRecorder()
	srv.ChannelDetail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "渠道详情") {
		t.Fatalf("expected page title, got body=%s", body)
	}
	if !strings.Contains(body, "...3456") {
		t.Fatalf("expected masked key, got body=%s", body)
	}
	if !strings.Contains(body, "10") || !strings.Contains(body, "5") {
		t.Fatalf("expected token numbers, got body=%s", body)
	}
}
