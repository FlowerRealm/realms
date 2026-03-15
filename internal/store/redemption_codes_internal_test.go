package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestMapRedemptionRecordInsertError_UniqueConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realms.db")
	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()

	if err := EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO redemption_code_redemptions(code_id, user_id, reward_type, balance_usd, subscription_id, subscription_activation_mode, created_at)
VALUES(?, ?, ?, ?, NULL, NULL, CURRENT_TIMESTAMP)
`, 1, 1, "balance", "1"); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err = db.Exec(`
INSERT INTO redemption_code_redemptions(code_id, user_id, reward_type, balance_usd, subscription_id, subscription_activation_mode, created_at)
VALUES(?, ?, ?, ?, NULL, NULL, CURRENT_TIMESTAMP)
`, 1, 1, "balance", "1")
	if err == nil {
		t.Fatalf("expected unique constraint error")
	}
	if !errors.Is(mapRedemptionRecordInsertError(err), ErrRedemptionCodeAlreadyRedeemed) {
		t.Fatalf("expected already redeemed mapping, got %v", mapRedemptionRecordInsertError(err))
	}
}
