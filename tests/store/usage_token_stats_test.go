package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestUsageStatsByToken_SQLite(t *testing.T) {
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

	t1Name := "t1"
	t1ID, _, err := st.CreateUserToken(ctx, userID, &t1Name, "sk-test-t1")
	if err != nil {
		t.Fatalf("CreateUserToken(t1): %v", err)
	}
	t2Name := "t2"
	t2ID, _, err := st.CreateUserToken(ctx, userID, &t2Name, "sk-test-t2")
	if err != nil {
		t.Fatalf("CreateUserToken(t2): %v", err)
	}

	now := time.Now().UTC()
	since := now.Add(-2 * time.Hour)
	until := now.Add(2 * time.Hour)

	newUsageEvent := func(reqID string, tokenID int64, committedUSD string, inTok, outTok int64) int64 {
		t.Helper()

		usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
			RequestID:        reqID,
			UserID:           userID,
			SubscriptionID:   nil,
			TokenID:          tokenID,
			Model:            func() *string { s := "m1"; return &s }(),
			ReservedUSD:      decimal.Zero,
			ReserveExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("ReserveUsage(%s): %v", reqID, err)
		}
		if err := st.CommitUsage(ctx, store.CommitUsageInput{
			UsageEventID: usageID,
			InputTokens:  &inTok,
			OutputTokens: &outTok,
			CommittedUSD: decimal.RequireFromString(committedUSD),
		}); err != nil {
			t.Fatalf("CommitUsage(%s): %v", reqID, err)
		}
		if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
			UsageEventID: usageID,
			Endpoint:     "/v1/responses",
			Method:       "POST",
			StatusCode:   200,
			LatencyMS:    10,
			IsStream:     false,
			RequestBytes: 123,
			ResponseBytes: 456,
		}); err != nil {
			t.Fatalf("FinalizeUsageEvent(%s): %v", reqID, err)
		}
		return usageID
	}

	ev1 := newUsageEvent("req_t1_1", t1ID, "1.23", 10, 5)
	_ = ev1
	newUsageEvent("req_t2_1", t2ID, "9.99", 7, 3)

	committed, reserved, err := st.SumCommittedAndReservedUSDRangeByToken(ctx, store.UsageSumWithReservedRangeByTokenInput{
		TokenID: t1ID,
		Since:   since,
		Until:   until,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("SumCommittedAndReservedUSDRangeByToken: %v", err)
	}
	if got := committed.StringFixed(2); got != "1.23" {
		t.Fatalf("committed mismatch: got=%s want=%s", got, "1.23")
	}
	if !reserved.Equal(decimal.Zero) {
		t.Fatalf("reserved mismatch: got=%s want=0", reserved.String())
	}

	stats, err := st.GetUsageTokenStatsByTokenRange(ctx, t1ID, since, until)
	if err != nil {
		t.Fatalf("GetUsageTokenStatsByTokenRange: %v", err)
	}
	if stats.Requests != 1 {
		t.Fatalf("requests mismatch: got=%d want=%d", stats.Requests, 1)
	}
	if stats.InputTokens != 10 || stats.OutputTokens != 5 || stats.Tokens != 15 {
		t.Fatalf("token stats mismatch: got in=%d out=%d tokens=%d", stats.InputTokens, stats.OutputTokens, stats.Tokens)
	}

	events, err := st.ListUsageEventsByTokenRange(ctx, t1ID, since, until, 50, nil, nil)
	if err != nil {
		t.Fatalf("ListUsageEventsByTokenRange: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events count mismatch: got=%d want=%d", len(events), 1)
	}
	if events[0].TokenID != t1ID {
		t.Fatalf("expected token_id=%d, got=%d", t1ID, events[0].TokenID)
	}

	series, err := st.GetTokenUsageTimeSeriesRange(ctx, t1ID, since, until, "hour")
	if err != nil {
		t.Fatalf("GetTokenUsageTimeSeriesRange: %v", err)
	}
	if len(series) == 0 {
		t.Fatalf("expected non-empty series")
	}
}

