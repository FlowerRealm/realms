package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestAddUserBalanceUSD_InitializesRowAndAdds(t *testing.T) {
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
	u1, err := st.CreateUser(ctx, "u1@example.com", "u1", []byte("hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(u1): %v", err)
	}
	u2, err := st.CreateUser(ctx, "u2@example.com", "u2", []byte("hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser(u2): %v", err)
	}

	if bal, err := st.GetUserBalanceUSD(ctx, u1); err != nil || !bal.Equal(decimal.Zero) {
		t.Fatalf("GetUserBalanceUSD(u1) = %v, %v; want 0, nil", bal, err)
	}

	delta := decimal.RequireFromString("1.2345678")
	newBal, err := st.AddUserBalanceUSD(ctx, u1, delta)
	if err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}
	if want := decimal.RequireFromString("1.234567"); !newBal.Equal(want) {
		t.Fatalf("new balance = %s; want %s", newBal.String(), want.String())
	}

	newBal2, err := st.AddUserBalanceUSD(ctx, u1, decimal.RequireFromString("0.000001"))
	if err != nil {
		t.Fatalf("AddUserBalanceUSD (2): %v", err)
	}
	if want := decimal.RequireFromString("1.234568"); !newBal2.Equal(want) {
		t.Fatalf("new balance (2) = %s; want %s", newBal2.String(), want.String())
	}

	m, err := st.GetUserBalancesUSD(ctx, []int64{u1, u2})
	if err != nil {
		t.Fatalf("GetUserBalancesUSD: %v", err)
	}
	if got := m[u1]; !got.Equal(decimal.RequireFromString("1.234568")) {
		t.Fatalf("balance map u1 = %s; want %s", got.String(), "1.234568")
	}
	if got := m[u2]; !got.Equal(decimal.Zero) {
		t.Fatalf("balance map u2 = %s; want 0", got.String())
	}
}

func TestAddUserBalanceUSD_InvalidInput(t *testing.T) {
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
	if _, err := st.AddUserBalanceUSD(ctx, 0, decimal.RequireFromString("1")); err == nil {
		t.Fatalf("expected error for user_id=0")
	}
	if _, err := st.AddUserBalanceUSD(ctx, 1, decimal.Zero); err == nil {
		t.Fatalf("expected error for delta=0")
	}
}
