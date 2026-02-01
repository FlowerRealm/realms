package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestSQLiteUsageBalance_CommitRefundsWhenCommittedLessThanReserved(t *testing.T) {
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
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	model := "m1"
	usageID, err := st.ReserveUsageAndDebitBalance(ctx, store.ReserveUsageInput{
		RequestID:        "req_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            &model,
		ReservedUSD:      decimal.RequireFromString("1"),
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsageAndDebitBalance: %v", err)
	}

	if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	} else if got.String() != "9" {
		t.Fatalf("balance after reserve: got %s want %s", got.String(), "9")
	}

	inTok := int64(100)
	outTok := int64(50)
	if err := st.CommitUsageAndRefundBalance(ctx, store.CommitUsageInput{
		UsageEventID: usageID,
		InputTokens:  &inTok,
		OutputTokens: &outTok,
		CommittedUSD: decimal.RequireFromString("0.4"),
	}); err != nil {
		t.Fatalf("CommitUsageAndRefundBalance: %v", err)
	}

	if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	} else if got.String() != "9.6" {
		t.Fatalf("balance after commit refund: got %s want %s", got.String(), "9.6")
	}

	ev, err := st.GetUsageEvent(ctx, usageID)
	if err != nil {
		t.Fatalf("GetUsageEvent: %v", err)
	}
	if ev.State != store.UsageStateCommitted {
		t.Fatalf("state mismatch: got %q want %q", ev.State, store.UsageStateCommitted)
	}
	if ev.ReservedUSD.String() != "1" {
		t.Fatalf("reserved_usd mismatch: got %s want %s", ev.ReservedUSD.String(), "1")
	}
	if ev.CommittedUSD.String() != "0.4" {
		t.Fatalf("committed_usd mismatch: got %s want %s", ev.CommittedUSD.String(), "0.4")
	}
	if ev.InputTokens == nil || *ev.InputTokens != inTok {
		t.Fatalf("input_tokens mismatch: got=%v want=%d", ev.InputTokens, inTok)
	}
	if ev.OutputTokens == nil || *ev.OutputTokens != outTok {
		t.Fatalf("output_tokens mismatch: got=%v want=%d", ev.OutputTokens, outTok)
	}
}

func TestSQLiteUsageBalance_CommitDebitsExtraWhenCommittedGreaterThanReserved(t *testing.T) {
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
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	model := "m1"
	usageID, err := st.ReserveUsageAndDebitBalance(ctx, store.ReserveUsageInput{
		RequestID:        "req_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            &model,
		ReservedUSD:      decimal.RequireFromString("1"),
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsageAndDebitBalance: %v", err)
	}

	if err := st.CommitUsageAndRefundBalance(ctx, store.CommitUsageInput{
		UsageEventID: usageID,
		CommittedUSD: decimal.RequireFromString("2.5"),
	}); err != nil {
		t.Fatalf("CommitUsageAndRefundBalance: %v", err)
	}

	if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	} else if got.String() != "7.5" {
		t.Fatalf("balance after commit extra debit: got %s want %s", got.String(), "7.5")
	}

	ev, err := st.GetUsageEvent(ctx, usageID)
	if err != nil {
		t.Fatalf("GetUsageEvent: %v", err)
	}
	if ev.CommittedUSD.String() != "2.5" {
		t.Fatalf("committed_usd mismatch: got %s want %s", ev.CommittedUSD.String(), "2.5")
	}
}

func TestSQLiteUsageBalance_CommitDebitsExtraUpToZeroWhenInsufficientBalance(t *testing.T) {
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
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("0.5")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	model := "m1"
	usageID, err := st.ReserveUsageAndDebitBalance(ctx, store.ReserveUsageInput{
		RequestID:        "req_1",
		UserID:           userID,
		TokenID:          tokenID,
		Model:            &model,
		ReservedUSD:      decimal.RequireFromString("0.2"),
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsageAndDebitBalance: %v", err)
	}

	if err := st.CommitUsageAndRefundBalance(ctx, store.CommitUsageInput{
		UsageEventID: usageID,
		CommittedUSD: decimal.RequireFromString("0.9"),
	}); err != nil {
		t.Fatalf("CommitUsageAndRefundBalance: %v", err)
	}

	if got, err := st.GetUserBalanceUSD(ctx, userID); err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	} else if got.String() != "0" {
		t.Fatalf("balance after commit extra debit: got %s want %s", got.String(), "0")
	}

	ev, err := st.GetUsageEvent(ctx, usageID)
	if err != nil {
		t.Fatalf("GetUsageEvent: %v", err)
	}
	if ev.CommittedUSD.String() != "0.5" {
		t.Fatalf("committed_usd mismatch: got %s want %s", ev.CommittedUSD.String(), "0.5")
	}
}
