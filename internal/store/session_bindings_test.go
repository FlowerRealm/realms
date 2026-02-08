package store_test

import (
	"context"
	"testing"
	"time"

	"realms/internal/store"
)

func TestSessionBindings_SQLiteRoundTrip(t *testing.T) {
	db, err := store.OpenSQLite(":memory:")
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
	expiresAt := time.Now().Add(5 * time.Minute)
	payload := `{"channel_id":1,"credential_id":7}`
	if err := st.UpsertSessionBindingPayload(ctx, 9, "rk_hash", payload, expiresAt); err != nil {
		t.Fatalf("UpsertSessionBindingPayload: %v", err)
	}

	got, ok, err := st.GetSessionBindingPayload(ctx, 9, "rk_hash", time.Now())
	if err != nil {
		t.Fatalf("GetSessionBindingPayload: %v", err)
	}
	if !ok {
		t.Fatalf("expected binding to exist")
	}
	if got != payload {
		t.Fatalf("unexpected payload: %s", got)
	}

	if err := st.DeleteSessionBinding(ctx, 9, "rk_hash"); err != nil {
		t.Fatalf("DeleteSessionBinding: %v", err)
	}
	if _, ok, err := st.GetSessionBindingPayload(ctx, 9, "rk_hash", time.Now()); err != nil {
		t.Fatalf("GetSessionBindingPayload after delete: %v", err)
	} else if ok {
		t.Fatalf("expected binding to be deleted")
	}
}

func TestSessionBindings_SQLiteExpiredRecordIgnored(t *testing.T) {
	db, err := store.OpenSQLite(":memory:")
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
	if err := st.UpsertSessionBindingPayload(ctx, 11, "expired", `{"expired":true}`, time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("UpsertSessionBindingPayload: %v", err)
	}

	if _, ok, err := st.GetSessionBindingPayload(ctx, 11, "expired", time.Now()); err != nil {
		t.Fatalf("GetSessionBindingPayload: %v", err)
	} else if ok {
		t.Fatalf("expected expired binding to be ignored")
	}
}
