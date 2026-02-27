package store_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	rlmcrypto "realms/internal/crypto"
	"realms/internal/store"
)

func TestSetPersonalModeKey_SQLite(t *testing.T) {
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
	if err := st.UpsertAppSetting(ctx, "self_mode_key_hash", "deadbeef"); err != nil {
		t.Fatalf("UpsertAppSetting(self_mode_key_hash): %v", err)
	}

	if err := st.SetPersonalModeKey(ctx, "k_test_1"); err != nil {
		t.Fatalf("SetPersonalModeKey: %v", err)
	}
	gotHash, ok, err := st.GetPersonalModeKeyHash(ctx)
	if err != nil {
		t.Fatalf("GetPersonalModeKeyHash: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	wantHash := rlmcrypto.TokenHash("k_test_1")
	if !bytes.Equal(gotHash, wantHash) {
		t.Fatalf("hash mismatch")
	}

	if _, ok, err := st.GetAppSetting(ctx, "self_mode_key_hash"); err != nil {
		t.Fatalf("GetAppSetting(self_mode_key_hash): %v", err)
	} else if ok {
		t.Fatalf("expected legacy self_mode_key_hash to be deleted")
	}

	if err := st.SetPersonalModeKey(ctx, "k_test_2"); err != nil {
		t.Fatalf("SetPersonalModeKey(2): %v", err)
	}
	gotHash2, ok, err := st.GetPersonalModeKeyHash(ctx)
	if err != nil {
		t.Fatalf("GetPersonalModeKeyHash(2): %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true after overwrite")
	}
	wantHash2 := rlmcrypto.TokenHash("k_test_2")
	if !bytes.Equal(gotHash2, wantHash2) {
		t.Fatalf("hash mismatch after overwrite")
	}
}
