package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestEnsureSQLiteSchema_BackfillsChannelGroupMembers(t *testing.T) {
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
	vipID, err := st.CreateChannelGroup(ctx, "vip", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-vip", "vip", 3, true, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM channel_group_members WHERE member_channel_id=?`, channelID); err != nil {
		t.Fatalf("delete channel_group_members: %v", err)
	}
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema (2): %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM channel_group_members WHERE parent_group_id=? AND member_channel_id=?`, vipID, channelID).Scan(&n); err != nil {
		t.Fatalf("query channel_group_members: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected EnsureSQLiteSchema to backfill channel_group_members, got none")
	}
}

func TestForceDeleteChannelGroup_SQLite(t *testing.T) {
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
	groupID, err := st.CreateChannelGroup(ctx, "vip", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", "vip", 3, true, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	sum, err := st.ForceDeleteChannelGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("ForceDeleteChannelGroup: %v", err)
	}
	if sum.ChannelsUpdated != 1 {
		t.Fatalf("ChannelsUpdated mismatch: got %d want %d", sum.ChannelsUpdated, 1)
	}
	if sum.ChannelsDisabled != 0 {
		t.Fatalf("ChannelsDisabled mismatch: got %d want %d", sum.ChannelsDisabled, 0)
	}

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}
	if ch.Groups != "" {
		t.Fatalf("channel groups mismatch: got %q want %q", ch.Groups, "")
	}
	if ch.Status != 1 {
		t.Fatalf("channel status mismatch: got %d want %d", ch.Status, 1)
	}

	_, err = st.GetChannelGroupByID(ctx, groupID)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected deleted group to be missing, got err=%v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM channel_group_members WHERE member_channel_id=?`, channelID).Scan(&n); err != nil {
		t.Fatalf("query channel_group_members: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected channel_group_members to be removed after deleting last group, got=%d", n)
	}
}

func TestListEnabledManagedModelsWithBindingsForGroup_SQLite(t *testing.T) {
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
	if _, err := st.CreateChannelGroup(ctx, "vip", nil, 1, store.DefaultGroupPriceMultiplier, 5); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	vipChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-vip", "vip", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if _, err := st.CreateChannelGroup(ctx, "admin", nil, 1, store.DefaultGroupPriceMultiplier, 5); err != nil {
		t.Fatalf("CreateChannelGroup(admin): %v", err)
	}
	adminChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-admin", "admin", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(admin): %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "m-vip",
		GroupName:           "vip",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.NewFromInt(0),
		CacheOutputUSDPer1M: decimal.NewFromInt(0),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(vip): %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "m-admin",
		GroupName:           "admin",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.NewFromInt(0),
		CacheOutputUSDPer1M: decimal.NewFromInt(0),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(admin): %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "m-vip-unbound",
		GroupName:           "vip",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.NewFromInt(0),
		CacheOutputUSDPer1M: decimal.NewFromInt(0),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(vip-unbound): %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "m-vip-on-admin-channel",
		GroupName:           "vip",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.NewFromInt(0),
		CacheOutputUSDPer1M: decimal.NewFromInt(0),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(vip-on-admin-channel): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     vipChannelID,
		PublicID:      "m-vip",
		UpstreamModel: "m-vip",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(vip): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     adminChannelID,
		PublicID:      "m-admin",
		UpstreamModel: "m-admin",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(admin): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     adminChannelID,
		PublicID:      "m-vip-on-admin-channel",
		UpstreamModel: "m-vip-on-admin-channel",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(vip-on-admin-channel): %v", err)
	}

	ms, err := st.ListEnabledManagedModelsWithBindingsForGroup(ctx, "vip")
	if err != nil {
		t.Fatalf("ListEnabledManagedModelsWithBindingsForGroup: %v", err)
	}
	if len(ms) != 1 || ms[0].PublicID != "m-vip" || ms[0].GroupName != "vip" {
		t.Fatalf("unexpected managed models: %+v", ms)
	}

	ms2, err := st.ListEnabledManagedModelsWithBindingsForGroups(ctx, []string{"vip"})
	if err != nil {
		t.Fatalf("ListEnabledManagedModelsWithBindingsForGroups: %v", err)
	}
	if len(ms2) != 1 || ms2[0].PublicID != "m-vip" || ms2[0].GroupName != "vip" {
		t.Fatalf("unexpected managed models (groups): %+v", ms2)
	}

	ms3, err := st.ListEnabledManagedModelsWithBindingsForGroups(ctx, nil)
	if err != nil {
		t.Fatalf("ListEnabledManagedModelsWithBindingsForGroups(empty): %v", err)
	}
	if len(ms3) != 0 {
		t.Fatalf("expected empty managed models for empty groups, got: %+v", ms3)
	}

	ms4, err := st.ListEnabledManagedModelsWithBindingsForGroups(ctx, []string{"vip", "admin"})
	if err != nil {
		t.Fatalf("ListEnabledManagedModelsWithBindingsForGroups(vip+admin): %v", err)
	}
	got := make(map[string]string, len(ms4))
	for _, item := range ms4 {
		got[item.PublicID] = item.GroupName
	}
	if len(got) != 3 || got["m-vip"] != "vip" || got["m-admin"] != "admin" || got["m-vip-on-admin-channel"] != "vip" {
		t.Fatalf("unexpected managed models (vip+admin): %+v", ms4)
	}
}

func TestUpdateChannelGroupWithRename_Cascades_SQLite(t *testing.T) {
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

	vipID, err := st.CreateChannelGroup(ctx, "vip", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	if err := st.CreateMainGroup(ctx, "team_a", nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, "team_a", []string{"vip"}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO token_groups(token_id, group_name, priority, created_at, updated_at) VALUES(?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, 100, "vip", 1); err != nil {
		t.Fatalf("insert token_groups: %v", err)
	}

	planID, err := st.CreateSubscriptionPlan(ctx, store.SubscriptionPlanCreate{
		Code:            "p1",
		Name:            "plan1",
		GroupName:       "vip",
		PriceMultiplier: store.DefaultGroupPriceMultiplier,
		PriceCNY:        decimal.NewFromInt(0),
		Limit5HUSD:      decimal.Zero,
		Limit1DUSD:      decimal.Zero,
		Limit7DUSD:      decimal.Zero,
		Limit30DUSD:     decimal.Zero,
		DurationDays:    30,
		Status:          1,
	})
	if err != nil {
		t.Fatalf("CreateSubscriptionPlan: %v", err)
	}

	if _, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
		PublicID:            "m-vip",
		GroupName:           "vip",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.Zero,
		CacheOutputUSDPer1M: decimal.Zero,
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-vip", "vip", 3, true, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	old, err := st.GetChannelGroupByID(ctx, vipID)
	if err != nil {
		t.Fatalf("GetChannelGroupByID: %v", err)
	}

	newName := "vip2"
	if _, err := st.UpdateChannelGroupWithRename(ctx, vipID, &newName, old.Description, old.Status, old.PriceMultiplier, old.MaxAttempts); err != nil {
		t.Fatalf("UpdateChannelGroupWithRename: %v", err)
	}

	got, err := st.GetChannelGroupByID(ctx, vipID)
	if err != nil {
		t.Fatalf("GetChannelGroupByID (2): %v", err)
	}
	if got.Name != "vip2" {
		t.Fatalf("channel_groups.name mismatch: got %q want %q", got.Name, "vip2")
	}

	subgroups, err := st.ListMainGroupSubgroups(ctx, "team_a")
	if err != nil {
		t.Fatalf("ListMainGroupSubgroups: %v", err)
	}
	if len(subgroups) != 1 || subgroups[0].Subgroup != "vip2" {
		t.Fatalf("main_group_subgroups.subgroup mismatch: %+v", subgroups)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM token_groups WHERE token_id=? AND group_name=?`, 100, "vip2").Scan(&n); err != nil {
		t.Fatalf("query token_groups (vip2): %v", err)
	}
	if n != 1 {
		t.Fatalf("token_groups vip2 mismatch: got %d want %d", n, 1)
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM token_groups WHERE token_id=? AND group_name=?`, 100, "vip").Scan(&n); err != nil {
		t.Fatalf("query token_groups (vip): %v", err)
	}
	if n != 0 {
		t.Fatalf("token_groups vip should be renamed, got %d", n)
	}

	plan, err := st.GetSubscriptionPlanByID(ctx, planID)
	if err != nil {
		t.Fatalf("GetSubscriptionPlanByID: %v", err)
	}
	if plan.GroupName != "vip2" {
		t.Fatalf("subscription_plans.group_name mismatch: got %q want %q", plan.GroupName, "vip2")
	}

	var modelGroup string
	if err := db.QueryRow(`SELECT group_name FROM managed_models WHERE public_id=?`, "m-vip").Scan(&modelGroup); err != nil {
		t.Fatalf("query managed_models.group_name: %v", err)
	}
	if modelGroup != "vip2" {
		t.Fatalf("managed_models.group_name mismatch: got %q want %q", modelGroup, "vip2")
	}

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}
	if ch.Groups != "vip2" {
		t.Fatalf("upstream_channels.groups mismatch: got %q want %q", ch.Groups, "vip2")
	}
}

func TestDefaultChannelGroup_CanDisableAndDelete_ClearsSetting_SQLite(t *testing.T) {
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

	g1ID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup(g1): %v", err)
	}
	g2ID, err := st.CreateChannelGroup(ctx, "g2", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup(g2): %v", err)
	}

	if err := st.SetDefaultChannelGroupID(ctx, g1ID); err != nil {
		t.Fatalf("SetDefaultChannelGroupID(g1): %v", err)
	}

	g1, err := st.GetChannelGroupByID(ctx, g1ID)
	if err != nil {
		t.Fatalf("GetChannelGroupByID(g1): %v", err)
	}

	if _, err := st.UpdateChannelGroupWithRename(ctx, g1ID, nil, g1.Description, 0, g1.PriceMultiplier, g1.MaxAttempts); err != nil {
		t.Fatalf("disable default group should succeed: %v", err)
	}
	if id, ok, err := st.GetDefaultChannelGroupID(ctx); err != nil || ok || id != 0 {
		t.Fatalf("expected default setting to be cleared after disabling default group, got id=%d ok=%v err=%v", id, ok, err)
	}
	if err := st.SetDefaultChannelGroupID(ctx, g1ID); err == nil {
		t.Fatalf("expected disabled group to be rejected as default, got nil")
	}

	if err := st.SetDefaultChannelGroupID(ctx, g2ID); err != nil {
		t.Fatalf("SetDefaultChannelGroupID(g2): %v", err)
	}
	if _, err := st.ForceDeleteChannelGroup(ctx, g2ID); err != nil {
		t.Fatalf("ForceDeleteChannelGroup(g2): %v", err)
	}
	if id, ok, err := st.GetDefaultChannelGroupID(ctx); err != nil || ok || id != 0 {
		t.Fatalf("expected default setting to be cleared after deleting default group, got id=%d ok=%v err=%v", id, ok, err)
	}
}
