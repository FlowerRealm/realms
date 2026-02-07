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
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema (2): %v", err)
	}

	var defaultGroupID int64
	if err := db.QueryRow(`SELECT id FROM channel_groups WHERE name='default' LIMIT 1`).Scan(&defaultGroupID); err != nil {
		t.Fatalf("query default group id: %v", err)
	}
	var codexChannelID int64
	if err := db.QueryRow(`SELECT id FROM upstream_channels WHERE type='codex_oauth' LIMIT 1`).Scan(&codexChannelID); err != nil {
		t.Fatalf("query codex_oauth channel id: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM channel_group_members WHERE parent_group_id=? AND member_channel_id=?`, defaultGroupID, codexChannelID).Scan(&n); err != nil {
		t.Fatalf("query channel_group_members: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected default group to include codex_oauth channel as member, got none")
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
	if sum.ChannelsDisabled != 1 {
		t.Fatalf("ChannelsDisabled mismatch: got %d want %d", sum.ChannelsDisabled, 1)
	}

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}
	if ch.Groups != store.DefaultGroupName {
		t.Fatalf("channel groups mismatch: got %q want %q", ch.Groups, store.DefaultGroupName)
	}
	if ch.Status != 0 {
		t.Fatalf("channel status mismatch: got %d want %d", ch.Status, 0)
	}

	_, err = st.GetChannelGroupByID(ctx, groupID)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected deleted group to be missing, got err=%v", err)
	}

	root, err := st.GetChannelGroupByName(ctx, store.DefaultGroupName)
	if err != nil {
		t.Fatalf("GetChannelGroupByName(default): %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM channel_group_members WHERE parent_group_id=? AND member_channel_id=?`, root.ID, channelID).Scan(&n); err != nil {
		t.Fatalf("query channel_group_members: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected channel_group_members to be rebuilt for default group")
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
	defaultChannelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch-default", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(default): %v", err)
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
		PublicID:            "m-default",
		GroupName:           "",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.NewFromInt(0),
		CacheOutputUSDPer1M: decimal.NewFromInt(0),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(default): %v", err)
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
		PublicID:            "m-vip-on-default-channel",
		GroupName:           "vip",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.NewFromInt(1),
		OutputUSDPer1M:      decimal.NewFromInt(1),
		CacheInputUSDPer1M:  decimal.NewFromInt(0),
		CacheOutputUSDPer1M: decimal.NewFromInt(0),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel(vip-on-default-channel): %v", err)
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
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     vipChannelID,
		PublicID:      "m-vip",
		UpstreamModel: "m-vip",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(vip): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     defaultChannelID,
		PublicID:      "m-default",
		UpstreamModel: "m-default",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(default): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     defaultChannelID,
		PublicID:      "m-vip-on-default-channel",
		UpstreamModel: "m-vip-on-default-channel",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(vip-on-default-channel): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
		ChannelID:     adminChannelID,
		PublicID:      "m-admin",
		UpstreamModel: "m-admin",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(admin): %v", err)
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
		t.Fatalf("ListEnabledManagedModelsWithBindingsForGroups(default): %v", err)
	}
	if len(ms3) != 1 || ms3[0].PublicID != "m-default" || ms3[0].GroupName != store.DefaultGroupName {
		t.Fatalf("unexpected managed models (default groups): %+v", ms3)
	}

	ms4, err := st.ListEnabledManagedModelsWithBindingsForGroups(ctx, []string{"vip", store.DefaultGroupName})
	if err != nil {
		t.Fatalf("ListEnabledManagedModelsWithBindingsForGroups(vip+default): %v", err)
	}
	got := make(map[string]string, len(ms4))
	for _, item := range ms4 {
		got[item.PublicID] = item.GroupName
	}
	if len(got) != 3 || got["m-vip"] != "vip" || got["m-default"] != store.DefaultGroupName || got["m-vip-on-default-channel"] != "vip" {
		t.Fatalf("unexpected managed models (vip+default): %+v", ms4)
	}
	if _, ok := got["m-admin"]; ok {
		t.Fatalf("unexpected managed model leaked from foreign group: %+v", ms4)
	}
}
