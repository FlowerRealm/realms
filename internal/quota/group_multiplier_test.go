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

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("0.8")); err != nil {
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
	if err := st.ReplaceTokenChannelGroups(ctx, tokID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	snap, err := loadTokenGroupMultiplierSnapshot(ctx, st, tokID)
	if err != nil {
		t.Fatalf("loadTokenGroupMultiplierSnapshot: %v", err)
	}
	if got, want := snap.maxGroupMultiplier.StringFixed(6), "1.500000"; got != want {
		t.Fatalf("max group multiplier mismatch: got=%s want=%s", got, want)
	}
}

func TestLoadUserGroupMultiplierSnapshot_UsesMaxReachableNestedPath(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokID := createQuotaTestUser(t, st, ctx, "nested@example.com", "nested")

	parentID, err := st.CreateChannelGroup(ctx, "parent", nil, 1, decimal.RequireFromString("1.2"))
	if err != nil {
		t.Fatalf("CreateChannelGroup(parent): %v", err)
	}
	childID, err := st.CreateChannelGroup(ctx, "child", nil, 1, decimal.RequireFromString("1.5"))
	if err != nil {
		t.Fatalf("CreateChannelGroup(child): %v", err)
	}
	altID, err := st.CreateChannelGroup(ctx, "alt", nil, 1, decimal.RequireFromString("1.6"))
	if err != nil {
		t.Fatalf("CreateChannelGroup(alt): %v", err)
	}
	if err := st.CreateMainGroup(ctx, "ug1", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "ug1", []string{"parent", "alt"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "ug1"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if err := st.ReplaceTokenChannelGroups(ctx, tokID, []string{"parent", "alt"}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "nested-ch", "child", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(child): %v", err)
	}
	if _, err := st.CreateUpstreamEndpoint(ctx, chID, "https://nested.example/v1", 0); err != nil {
		t.Fatalf("CreateUpstreamEndpoint(child): %v", err)
	}
	if err := st.AddChannelGroupMemberGroup(ctx, parentID, childID, 100, false); err != nil {
		t.Fatalf("AddChannelGroupMemberGroup: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, childID, chID, 100, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel(child): %v", err)
	}

	altChID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "alt-ch", "alt", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(alt): %v", err)
	}
	if _, err := st.CreateUpstreamEndpoint(ctx, altChID, "https://alt.example/v1", 0); err != nil {
		t.Fatalf("CreateUpstreamEndpoint(alt): %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, altID, altChID, 100, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel(alt): %v", err)
	}

	snap, err := loadTokenGroupMultiplierSnapshot(ctx, st, tokID)
	if err != nil {
		t.Fatalf("loadTokenGroupMultiplierSnapshot: %v", err)
	}
	if got, want := snap.maxGroupMultiplier.StringFixed(6), "1.800000"; got != want {
		t.Fatalf("max nested group multiplier mismatch: got=%s want=%s", got, want)
	}
}

func TestSubscriptionProviderReserveCommit_MultipliesPlanAndRouteGroup(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokenID := createQuotaTestUser(t, st, ctx, "bob@example.com", "bob")

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
		t.Fatalf("CreateChannelGroup(vip): %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "staff", nil, 1, decimal.RequireFromString("2")); err != nil {
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
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{"vip", "staff"}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
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
	// commit 使用最终成功渠道组 vip(1.5)：1 × 1.1 × 1.5 = 1.65
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

	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, decimal.RequireFromString("1.5")); err != nil {
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
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{"vip"}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
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

func TestHybridProviderCommit_AppliesNestedRouteGroupMultiplier(t *testing.T) {
	st := newQuotaTestStore(t)
	ctx := context.Background()

	userID, tokenID := createQuotaTestUser(t, st, ctx, "dave@example.com", "dave")
	if _, err := st.AddUserBalanceUSD(ctx, userID, decimal.RequireFromString("10")); err != nil {
		t.Fatalf("AddUserBalanceUSD: %v", err)
	}

	parentID, err := st.CreateChannelGroup(ctx, "parent", nil, 1, decimal.RequireFromString("1.2"))
	if err != nil {
		t.Fatalf("CreateChannelGroup(parent): %v", err)
	}
	childID, err := st.CreateChannelGroup(ctx, "child", nil, 1, decimal.RequireFromString("1.5"))
	if err != nil {
		t.Fatalf("CreateChannelGroup(child): %v", err)
	}
	if err := st.CreateMainGroup(ctx, "ug1", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "ug1", []string{"parent"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, "ug1"); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}
	if err := st.ReplaceTokenChannelGroups(ctx, tokenID, []string{"parent"}); err != nil {
		t.Fatalf("ReplaceTokenChannelGroups: %v", err)
	}

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "nested-ch", "child", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if _, err := st.CreateUpstreamEndpoint(ctx, chID, "https://nested.example/v1", 0); err != nil {
		t.Fatalf("CreateUpstreamEndpoint: %v", err)
	}
	if err := st.AddChannelGroupMemberGroup(ctx, parentID, childID, 100, false); err != nil {
		t.Fatalf("AddChannelGroupMemberGroup: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, childID, chID, 100, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel: %v", err)
	}

	modelID := "m-nested-mult"
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            modelID,
		GroupName:           "parent",
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
		RequestID:   "req_nested_mul_1",
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
		RouteGroup:   ptrString("parent/child"),
		InputTokens:  &inTokens,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	ev, err := st.GetUsageEvent(ctx, res.UsageEventID)
	if err != nil {
		t.Fatalf("GetUsageEvent: %v", err)
	}
	if got, want := ev.PriceMultiplierGroup.StringFixed(6), "1.800000"; got != want {
		t.Fatalf("group multiplier mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.PriceMultiplier.StringFixed(6), "1.800000"; got != want {
		t.Fatalf("total multiplier mismatch: got=%s want=%s", got, want)
	}
	if got, want := ev.CommittedUSD.StringFixed(6), "1.800000"; got != want {
		t.Fatalf("committed_usd mismatch: got=%s want=%s", got, want)
	}
	if ev.PriceMultiplierGroupName == nil || *ev.PriceMultiplierGroupName != "parent/child" {
		t.Fatalf("group path mismatch: %+v", ev.PriceMultiplierGroupName)
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
