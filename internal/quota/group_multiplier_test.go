package quota

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestLoadUserGroupMultiplierSnapshot_StacksUserGroups(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, _ := createQuotaTestUser(t, st, ctx, "alice@example.com", "alice")

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("0.8"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.ReplaceUserGroups(ctx, userID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceUserGroups: %v", err)
	}

	snap, err := loadUserGroupMultiplierSnapshot(ctx, st, userID)
	if err != nil {
		t.Fatalf("loadUserGroupMultiplierSnapshot: %v", err)
	}
	if got, want := snap.userMultiplier.StringFixed(6), "1.200000"; got != want {
		t.Fatalf("user multiplier mismatch: got=%s want=%s", got, want)
	}
}

func TestSubscriptionProviderReserveCommit_UsesOnlyUserGroupMultiplier(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokenID := createQuotaTestUser(t, st, ctx, "bob@example.com", "bob")

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.ReplaceUserGroups(ctx, userID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceUserGroups: %v", err)
	}

	modelID := "m1"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           store.DefaultGroupName,
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:         "vip_plan",
		Name:         "VIP Plan",
		GroupName:    "vip",
		PriceCNY:     decimal.Zero,
		Limit5HUSD:   decimal.RequireFromString("1000"),
		Limit1DUSD:   decimal.RequireFromString("1000"),
		Limit7DUSD:   decimal.RequireFromString("1000"),
		Limit30DUSD:  decimal.RequireFromString("1000"),
		DurationDays: 30,
		Status:       1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	if _, _, err := st.PurchaseSubscriptionByPlanID(ctx, userID, planID, time.Now()); err != nil {
		t.Fatalf("PurchaseSubscriptionByPlanID: %v", err)
	}
	// 购买后移除 vip，验证订阅分组不会参与计费倍率，最终只按当前用户分组 staff(2) 计费。
	if err := st.ReplaceUserGroups(ctx, userID, []string{"staff"}); err != nil {
		t.Fatalf("ReplaceUserGroups(after purchase): %v", err)
	}

	inTokens := int64(1_000_000)
	provider := NewSubscriptionProvider(st, time.Minute)
	res, err := provider.Reserve(ctx, ReserveInput{
		RequestID:   "req_sub_mul_1",
		UserID:      userID,
		TokenID:     tokenID,
		Model:       &modelID,
		InputTokens: &inTokens,
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	ev, err := st.GetUsageEvent(ctx, res.UsageEventID)
	if err != nil {
		t.Fatalf("GetUsageEvent(reserve): %v", err)
	}
	if got, want := ev.ReservedUSD.StringFixed(6), "2.000000"; got != want {
		t.Fatalf("reserved_usd mismatch: got=%s want=%s", got, want)
	}

	if err := provider.Commit(ctx, CommitInput{
		UsageEventID: res.UsageEventID,
		Model:        &modelID,
		InputTokens:  &inTokens,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	ev, err = st.GetUsageEvent(ctx, res.UsageEventID)
	if err != nil {
		t.Fatalf("GetUsageEvent(commit): %v", err)
	}
	if got, want := ev.CommittedUSD.StringFixed(6), "2.000000"; got != want {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", got, want)
	}
}

func TestHybridProviderReserveCommit_AppliesDefaultGroupMultiplier(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokenID := createQuotaTestUser(t, st, ctx, "carol@example.com", "carol")
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	defaultGroup, err := st.GetChannelGroupByName(ctx, store.DefaultGroupName)
	if err != nil {
		t.Fatalf("GetChannelGroupByName(default): %v", err)
	}
	if err := st.UpdateChannelGroup(
		ctx,
		defaultGroup.ID,
		defaultGroup.Description,
		defaultGroup.Status,
		decimal.RequireFromString("1.5"),
		defaultGroup.MaxAttempts,
	); err != nil {
		t.Fatalf("UpdateChannelGroup(default): %v", err)
	}

	modelID := "m-default-mult"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           store.DefaultGroupName,
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	inTokens := int64(1_000_000)
	provider := NewHybridProvider(st, time.Minute, true)
	res, err := provider.Reserve(ctx, ReserveInput{
		RequestID:   "req_default_mul_1",
		UserID:      userID,
		TokenID:     tokenID,
		Model:       &modelID,
		InputTokens: &inTokens,
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := provider.Commit(ctx, CommitInput{
		UsageEventID: res.UsageEventID,
		Model:        &modelID,
		InputTokens:  &inTokens,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	ev, err := st.GetUsageEvent(ctx, res.UsageEventID)
	if err != nil {
		t.Fatalf("GetUsageEvent: %v", err)
	}
	if got, want := ev.CommittedUSD.StringFixed(6), "1.500000"; got != want {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", got, want)
	}

	bal, err := st.GetUserBalanceUSD(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserBalanceUSD: %v", err)
	}
	if got, want := bal.StringFixed(6), "8.500000"; got != want {
		t.Fatalf("balance mismatch: got=%s want=%s", got, want)
	}
}

func newQuotaTestStore(t *testing.T) *store.Store {
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

func createQuotaTestUser(t *testing.T, st *store.Store, ctx context.Context, email string, username string) (int64, int64) {
	t.Helper()

	userID, err := st.CreateUser(ctx, email, username, []byte("pw-hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_"+username+"_test")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}
	return userID, tokenID
}
