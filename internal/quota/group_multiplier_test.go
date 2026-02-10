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

	userID, tokID := createQuotaTestUser(t, st, ctx, "alice@example.com", "alice")

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("0.8"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.CreateMainGroup(ctx, "ug1", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "ug1", []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "ug1"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if err := st.ReplaceTokenGroups(ctx, tokID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceTokenGroups: %v", err)
	}

	snap, err := loadTokenGroupMultiplierSnapshot(ctx, st, tokID)
	if err != nil {
		t.Fatalf("loadTokenGroupMultiplierSnapshot: %v", err)
	}
	if got, want := snap.maxGroupMultiplier.StringFixed(6), "1.500000"; got != want {
		t.Fatalf("max group multiplier mismatch: got=%s want=%s", got, want)
	}
}

func TestSubscriptionProviderReserveCommit_MultipliesPlanAndRouteGroup(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokenID := createQuotaTestUser(t, st, ctx, "bob@example.com", "bob")

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(staff): %v", err)
	}
	if err := st.CreateMainGroup(ctx, "ug1", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "ug1", []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "ug1"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if err := st.ReplaceTokenGroups(ctx, tokenID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceTokenGroups: %v", err)
	}

	modelID := "m1"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           "vip",
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.Zero,
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:            "vip_plan",
		Name:            "VIP Plan",
		GroupName:       "vip",
		PriceMultiplier: decimal.RequireFromString("1.1"),
		PriceCNY:        decimal.Zero,
		Limit5HUSD:      decimal.RequireFromString("1000"),
		Limit1DUSD:      decimal.RequireFromString("1000"),
		Limit7DUSD:      decimal.RequireFromString("1000"),
		Limit30DUSD:     decimal.RequireFromString("1000"),
		DurationDays:    30,
		Status:          1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}
	if _, _, err := st.PurchaseSubscriptionByPlanID(ctx, userID, planID, time.Now()); err != nil {
		t.Fatalf("PurchaseSubscriptionByPlanID: %v", err)
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
	// reserve 使用 max(group_multiplier)=staff(2) 保守预留：1 × 1.1 × 2 = 2.2
	if got, want := ev.ReservedUSD.StringFixed(6), "2.200000"; got != want {
		t.Fatalf("reserved_usd mismatch: got=%s want=%s", got, want)
	}

	if err := provider.Commit(ctx, CommitInput{
		UsageEventID: res.UsageEventID,
		Model:        &modelID,
		RouteGroup:   ptrString("vip"),
		InputTokens:  &inTokens,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	ev, err = st.GetUsageEvent(ctx, res.UsageEventID)
	if err != nil {
		t.Fatalf("GetUsageEvent(commit): %v", err)
	}
	// commit 使用最终成功分组 vip(1.5)：1 × 1.1 × 1.5 = 1.65
	if got, want := ev.CommittedUSD.StringFixed(6), "1.650000"; got != want {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.PriceMultiplierPayment.StringFixed(6), "1.100000"; got != want {
		t.Fatalf("payment multiplier mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.PriceMultiplierGroup.StringFixed(6), "1.500000"; got != want {
		t.Fatalf("group multiplier mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.PriceMultiplier.StringFixed(6), "1.650000"; got != want {
		t.Fatalf("total multiplier mismatch: got=%s want=%s", got, want)
	}
	if ev.PriceMultiplierGroupName == nil || *ev.PriceMultiplierGroupName != "vip" {
		t.Fatalf("group name mismatch: %+v", ev.PriceMultiplierGroupName)
	}
}

func TestHybridProviderReserveCommit_AppliesRouteGroupMultiplier(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokenID := createQuotaTestUser(t, st, ctx, "carol@example.com", "carol")
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5"), 5); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if err := st.CreateMainGroup(ctx, "ug1", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "ug1", []string{"vip"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "ug1"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if err := st.ReplaceTokenGroups(ctx, tokenID, []string{"vip"}); err != nil {
		t.Fatalf("ReplaceTokenGroups: %v", err)
	}

	modelID := "m-default-mult"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           "vip",
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
		RouteGroup:   ptrString("vip"),
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
	if got, want := ev.PriceMultiplierPayment.StringFixed(6), "1.000000"; got != want {
		t.Fatalf("payment multiplier mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.PriceMultiplierGroup.StringFixed(6), "1.500000"; got != want {
		t.Fatalf("group multiplier mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.PriceMultiplier.StringFixed(6), "1.500000"; got != want {
		t.Fatalf("total multiplier mismatch: got=%s want=%s", got, want)
	}
	if ev.PriceMultiplierGroupName == nil || *ev.PriceMultiplierGroupName != "vip" {
		t.Fatalf("group name mismatch: %+v", ev.PriceMultiplierGroupName)
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

func ptrString(v string) *string {
	return &v
}
