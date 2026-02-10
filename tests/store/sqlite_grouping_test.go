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
