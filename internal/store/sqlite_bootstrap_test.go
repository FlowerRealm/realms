package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"realms/internal/store"
)

func TestSQLiteBootstrap_CreateUserRoundTrip(t *testing.T) {
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
	// 再跑一次，确保幂等。
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema (2): %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	ctx := context.Background()
	userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, err := st.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if u.ID != userID {
		t.Fatalf("user id mismatch: got %d want %d", u.ID, userID)
	}
	if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
		t.Fatalf("expected created_at/updated_at to be parsed, got created_at=%v updated_at=%v", u.CreatedAt, u.UpdatedAt)
	}
	if len(u.Groups) == 0 {
		t.Fatalf("expected default group, got none")
	}
}
