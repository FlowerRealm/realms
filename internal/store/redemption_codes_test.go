package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func newSQLiteStoreForRedemptionTest(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)
	return st
}

func TestRedeemCode_BalanceSingleUse(t *testing.T) {
	st := newSQLiteStoreForRedemptionTest(t)
	ctx := context.Background()

	userID, err := st.CreateUser(ctx, "redeem-balance@example.com", "redeembalance", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.CreateMainGroup(ctx, "default", nil, 1); err != nil && err.Error() != "main_group 名称已存在" {
		// ignore duplicate from bootstrap
	}
	if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	codeID, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "balance-batch",
		Code:             "BAL-ONE",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("5"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}
	if codeID <= 0 {
		t.Fatalf("expected code id > 0")
	}

	res, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: userID, Code: "bal-one"})
	if err != nil {
		t.Fatalf("RedeemCode: %v", err)
	}
	if res.RewardType != store.RedemptionCodeRewardBalance {
		t.Fatalf("reward type mismatch: got %q", res.RewardType)
	}
	if !res.NewBalanceUSD.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("new balance mismatch: got %s want 5", res.NewBalanceUSD.String())
	}
	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: userID, Code: "BAL-ONE"}); err == nil || err != store.ErrRedemptionCodeExhausted {
		t.Fatalf("expected exhausted error, got %v", err)
	}
}

func TestRedeemCode_SharedSinglePerUser(t *testing.T) {
	st := newSQLiteStoreForRedemptionTest(t)
	ctx := context.Background()

	if err := st.CreateMainGroup(ctx, "default", nil, 1); err != nil && err.Error() != "main_group 名称已存在" {
	}
	user1, err := st.CreateUser(ctx, "shared1@example.com", "shared1", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser user1: %v", err)
	}
	user2, err := st.CreateUser(ctx, "shared2@example.com", "shared2", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser user2: %v", err)
	}
	for _, userID := range []int64{user1, user2} {
		if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
			t.Fatalf("SetUserMainGroup(%d): %v", userID, err)
		}
	}

	_, err = st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "shared-batch",
		Code:             "SHARED",
		DistributionMode: store.RedemptionCodeDistributionShared,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("2"),
		MaxRedemptions:   2,
		Status:           store.RedemptionCodeStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: user1, Code: "shared"}); err != nil {
		t.Fatalf("RedeemCode user1 first: %v", err)
	}
	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: user1, Code: "SHARED"}); err == nil || err != store.ErrRedemptionCodeAlreadyRedeemed {
		t.Fatalf("expected already redeemed for user1, got %v", err)
	}
	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: user2, Code: "SHARED"}); err != nil {
		t.Fatalf("RedeemCode user2: %v", err)
	}
	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: user2, Code: "SHARED"}); err == nil || err != store.ErrRedemptionCodeExhausted {
		t.Fatalf("expected exhausted after total usage, got %v", err)
	}
}

func TestRedeemCode_SubscriptionRequiresModeOnlyForSamePlan(t *testing.T) {
	st := newSQLiteStoreForRedemptionTest(t)
	ctx := context.Background()

	if err := st.CreateMainGroup(ctx, "default", nil, 1); err != nil && err.Error() != "main_group 名称已存在" {
	}
	userID, err := st.CreateUser(ctx, "plan-user@example.com", "planuser", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}

	planA, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "plan_a",
		Name:         "Plan A",
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan A: %v", err)
	}
	planB, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "plan_b",
		Name:         "Plan B",
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan B: %v", err)
	}

	if _, _, err := st.PurchaseSubscriptionByPlanID(ctx, userID, planA, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("PurchaseSubscriptionByPlanID: %v", err)
	}

	_, err = st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:          "sub-batch-same",
		Code:               "SAMEPLAN",
		DistributionMode:   store.RedemptionCodeDistributionSingle,
		RewardType:         store.RedemptionCodeRewardSubscription,
		SubscriptionPlanID: &planA,
		MaxRedemptions:     1,
		Status:             store.RedemptionCodeStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode same: %v", err)
	}
	_, err = st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:          "sub-batch-diff",
		Code:               "DIFFPLAN",
		DistributionMode:   store.RedemptionCodeDistributionSingle,
		RewardType:         store.RedemptionCodeRewardSubscription,
		SubscriptionPlanID: &planB,
		MaxRedemptions:     1,
		Status:             store.RedemptionCodeStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode diff: %v", err)
	}

	if _, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: userID, Code: "SAMEPLAN"}); err == nil || err != store.ErrSubscriptionActivationModeRequired {
		t.Fatalf("expected mode required for same plan, got %v", err)
	}
	res, err := st.RedeemCode(ctx, store.RedeemCodeInput{UserID: userID, Code: "DIFFPLAN"})
	if err != nil {
		t.Fatalf("RedeemCode diff plan: %v", err)
	}
	if res.Subscription == nil {
		t.Fatalf("expected subscription result")
	}
	if res.SubscriptionActivationMode == nil || *res.SubscriptionActivationMode != store.SubscriptionActivationModeImmediate {
		t.Fatalf("expected immediate activation for different plan, got %v", res.SubscriptionActivationMode)
	}
}

func TestRedeemCode_SubscriptionDeferredStartsAfterSamePlanEnd(t *testing.T) {
	st := newSQLiteStoreForRedemptionTest(t)
	ctx := context.Background()

	if err := st.CreateMainGroup(ctx, "default", nil, 1); err != nil && err.Error() != "main_group 名称已存在" {
	}
	userID, err := st.CreateUser(ctx, "deferred-user@example.com", "deferreduser", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "plan_deferred",
		Name:         "Plan Deferred",
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	existing, _, err := st.PurchaseSubscriptionByPlanID(ctx, userID, planID, time.Now())
	if err != nil {
		t.Fatalf("PurchaseSubscriptionByPlanID: %v", err)
	}

	_, err = st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:          "sub-batch-deferred",
		Code:               "DEFERME",
		DistributionMode:   store.RedemptionCodeDistributionSingle,
		RewardType:         store.RedemptionCodeRewardSubscription,
		SubscriptionPlanID: &planID,
		MaxRedemptions:     1,
		Status:             store.RedemptionCodeStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}
	mode := store.SubscriptionActivationModeDeferred
	res, err := st.RedeemCode(ctx, store.RedeemCodeInput{
		UserID:                     userID,
		Code:                       "DEFERME",
		SubscriptionActivationMode: &mode,
	})
	if err != nil {
		t.Fatalf("RedeemCode deferred: %v", err)
	}
	if res.Subscription == nil {
		t.Fatalf("expected subscription result")
	}
	if !res.Subscription.StartAt.Equal(existing.EndAt) {
		t.Fatalf("expected deferred start_at=%v got=%v", existing.EndAt, res.Subscription.StartAt)
	}
}

func TestCreateRedemptionCodes_AtomicOnDuplicate(t *testing.T) {
	st := newSQLiteStoreForRedemptionTest(t)
	ctx := context.Background()

	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "dup-existing",
		Code:             "EXISTING-CODE",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("1"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode existing: %v", err)
	}

	_, err := st.CreateRedemptionCodes(ctx, []store.RedemptionCodeCreate{
		{
			BatchName:        "dup-batch",
			Code:             "UNIQUE-ONE",
			DistributionMode: store.RedemptionCodeDistributionSingle,
			RewardType:       store.RedemptionCodeRewardBalance,
			BalanceUSD:       decimal.RequireFromString("1"),
			MaxRedemptions:   1,
			Status:           store.RedemptionCodeStatusActive,
		},
		{
			BatchName:        "dup-batch",
			Code:             "EXISTING-CODE",
			DistributionMode: store.RedemptionCodeDistributionSingle,
			RewardType:       store.RedemptionCodeRewardBalance,
			BalanceUSD:       decimal.RequireFromString("1"),
			MaxRedemptions:   1,
			Status:           store.RedemptionCodeStatusActive,
		},
	})
	if !errors.Is(err, store.ErrRedemptionCodeDuplicate) {
		t.Fatalf("expected duplicate error, got %v", err)
	}

	items, err := st.ListRedemptionCodes(ctx, store.RedemptionCodeListFilter{BatchName: "dup-batch", Limit: 20})
	if err != nil {
		t.Fatalf("ListRedemptionCodes: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected atomic rollback, got %d items", len(items))
	}
}

func TestRedeemCode_RewardTypeMismatch(t *testing.T) {
	st := newSQLiteStoreForRedemptionTest(t)
	ctx := context.Background()

	if err := st.CreateMainGroup(ctx, "default", nil, 1); err != nil && err.Error() != "main_group 名称已存在" {
	}
	userID, err := st.CreateUser(ctx, "mismatch-user@example.com", "mismatchuser", []byte("pw"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "default"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}

	if _, err := st.CreateRedemptionCode(ctx, store.RedemptionCodeCreate{
		BatchName:        "mismatch-batch",
		Code:             "BAL-MISMATCH",
		DistributionMode: store.RedemptionCodeDistributionSingle,
		RewardType:       store.RedemptionCodeRewardBalance,
		BalanceUSD:       decimal.RequireFromString("2"),
		MaxRedemptions:   1,
		Status:           store.RedemptionCodeStatusActive,
	}); err != nil {
		t.Fatalf("CreateRedemptionCode: %v", err)
	}

	_, err = st.RedeemCode(ctx, store.RedeemCodeInput{
		UserID:             userID,
		Code:               "BAL-MISMATCH",
		ExpectedRewardType: store.RedemptionCodeRewardSubscription,
	})
	if !errors.Is(err, store.ErrRedemptionCodeRewardMismatch) {
		t.Fatalf("expected reward mismatch, got %v", err)
	}
}
