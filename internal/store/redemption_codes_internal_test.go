package store

import (
	"context"
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

func TestReserveRedemptionSlotTx_DoesNotOverRedeem(t *testing.T) {
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
INSERT INTO redemption_codes(
  id, batch_name, code, distribution_mode, reward_type, subscription_plan_id, balance_usd,
  max_redemptions, redeemed_count, expires_at, status, created_by, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, NULL, 0, ?, 0, NULL, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, 1, "batch", "CODE-1", string(RedemptionCodeDistributionSingle), string(RedemptionCodeRewardBalance), 1, int(RedemptionCodeStatusActive)); err != nil {
		t.Fatalf("insert redemption_code: %v", err)
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	code := RedemptionCode{
		ID:               1,
		DistributionMode: RedemptionCodeDistributionSingle,
		MaxRedemptions:   1,
		RedeemedCount:    0,
	}
	code, err = reserveRedemptionSlotTx(context.Background(), tx, code, 1)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if code.RedeemedCount != 1 {
		t.Fatalf("expected redeemed_count=1 after first reserve, got %d", code.RedeemedCount)
	}

	if _, err := reserveRedemptionSlotTx(context.Background(), tx, code, 2); !errors.Is(err, ErrRedemptionCodeExhausted) {
		t.Fatalf("expected exhausted on second reserve, got %v", err)
	}
}
