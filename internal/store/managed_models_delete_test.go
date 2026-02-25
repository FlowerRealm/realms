package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
)

func TestDeleteManagedModel_CascadesChannelModelBindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := New(db)
	st.SetDialect(DialectSQLite)
	ctx := context.Background()

	ch1, err := st.CreateUpstreamChannel(ctx, UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(c1): %v", err)
	}
	ch2, err := st.CreateUpstreamChannel(ctx, UpstreamTypeOpenAICompatible, "c2", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(c2): %v", err)
	}

	modelID, err := st.CreateManagedModel(ctx, ManagedModelCreate{
		PublicID:            "gpt-cascade",
		GroupName:           "",
		OwnedBy:             nil,
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.RequireFromString("2"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0"),
		Status:              1,
	})
	if err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}

	if _, err := st.CreateChannelModel(ctx, ChannelModelCreate{
		ChannelID:     ch1,
		PublicID:      "gpt-cascade",
		UpstreamModel: "gpt-cascade",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(c1): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, ChannelModelCreate{
		ChannelID:     ch2,
		PublicID:      "gpt-cascade",
		UpstreamModel: "gpt-cascade",
		Status:        0,
	}); err != nil {
		t.Fatalf("CreateChannelModel(c2): %v", err)
	}

	if err := st.DeleteManagedModel(ctx, modelID); err != nil {
		t.Fatalf("DeleteManagedModel: %v", err)
	}

	got1, err := st.ListChannelModelsByChannelID(ctx, ch1)
	if err != nil {
		t.Fatalf("ListChannelModelsByChannelID(c1): %v", err)
	}
	if len(got1) != 0 {
		t.Fatalf("expected 0 bindings for c1, got %d", len(got1))
	}
	got2, err := st.ListChannelModelsByChannelID(ctx, ch2)
	if err != nil {
		t.Fatalf("ListChannelModelsByChannelID(c2): %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 bindings for c2, got %d", len(got2))
	}
}

