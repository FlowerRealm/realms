package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestGetUsageStatsByCredentialForChannelRange_SQLite(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	ep, err := st.SetUpstreamEndpointBaseURL(ctx, channelID, "https://api.openai.com")
	if err != nil {
		t.Fatalf("SetUpstreamEndpointBaseURL: %v", err)
	}
	cred1ID, _, err := st.CreateOpenAICompatibleCredential(ctx, ep.ID, nil, "sk-test-cred1-abcdef")
	if err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(1): %v", err)
	}
	cred2ID, _, err := st.CreateOpenAICompatibleCredential(ctx, ep.ID, nil, "sk-test-cred2-uvwxyz")
	if err != nil {
		t.Fatalf("CreateOpenAICompatibleCredential(2): %v", err)
	}

	newUsageEvent := func(reqID string, credID int64, status int, inTok, outTok int64) int64 {
		t.Helper()

		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           userID,
			TokenID:          tokenID,
			Model:            func() *string { s := "m1"; return &s }(),
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", reqID, err)
		}
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID:      usageID,
			UpstreamChannelID: &channelID,
			InputTokens:       &inTok,
			OutputTokens:      &outTok,
			CommittedUSD:      decimal.Zero,
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", reqID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID:       usageID,
			Endpoint:           "/v1/responses",
			Method:             "POST",
			StatusCode:         status,
			LatencyMS:          10,
			UpstreamChannelID:  &channelID,
			UpstreamEndpointID: &ep.ID,
			UpstreamCredID:     &credID,
			IsStream:           false,
			RequestBytes:       123,
			ResponseBytes:      456,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", reqID, err)
		}
		return usageID
	}

	newUsageEvent("req_1", cred1ID, 200, 10, 5)
	newUsageEvent("req_2", cred1ID, 500, 2, 1)
	newUsageEvent("req_3", cred2ID, 201, 7, 3)

	since := time.Now().UTC().Add(-2 * time.Hour)
	until := time.Now().UTC().Add(2 * time.Hour)
	stats, err := st.GetUsageStatsByCredentialForChannelRange(ctx, channelID, since, until)
	if err != nil {
		t.Fatalf("GetUsageStatsByCredentialForChannelRange: %v", err)
	}

	byID := map[int64]store.CredentialUsageStats{}
	for _, row := range stats {
		byID[row.CredentialID] = row
	}

	s1, ok := byID[cred1ID]
	if !ok {
		t.Fatalf("missing stats for cred1")
	}
	if s1.Requests != 2 || s1.Success != 1 || s1.Failure != 1 {
		t.Fatalf("cred1 counts mismatch: got requests=%d success=%d failure=%d", s1.Requests, s1.Success, s1.Failure)
	}
	if s1.InputTokens != 12 || s1.OutputTokens != 6 {
		t.Fatalf("cred1 tokens mismatch: got in=%d out=%d", s1.InputTokens, s1.OutputTokens)
	}
	if s1.LastSeenAt.IsZero() {
		t.Fatalf("cred1 last_seen_at should be set")
	}

	s2, ok := byID[cred2ID]
	if !ok {
		t.Fatalf("missing stats for cred2")
	}
	if s2.Requests != 1 || s2.Success != 1 || s2.Failure != 0 {
		t.Fatalf("cred2 counts mismatch: got requests=%d success=%d failure=%d", s2.Requests, s2.Success, s2.Failure)
	}
	if s2.InputTokens != 7 || s2.OutputTokens != 3 {
		t.Fatalf("cred2 tokens mismatch: got in=%d out=%d", s2.InputTokens, s2.OutputTokens)
	}
	if s2.LastSeenAt.IsZero() {
		t.Fatalf("cred2 last_seen_at should be set")
	}
}
