package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
)

func TestEnsureSQLiteSchema_CleansUpStaleChannelModels(t *testing.T) {
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

	channelID, err := st.CreateUpstreamChannel(ctx, UpstreamTypeOpenAICompatible, "cleanup", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if _, err := st.CreateManagedModel(ctx, ManagedModelCreate{
		PublicID:            "gpt-valid",
		GroupName:           "",
		InputUSDPer1M:       decimal.RequireFromString("1"),
		OutputUSDPer1M:      decimal.RequireFromString("2"),
		CacheInputUSDPer1M:  decimal.RequireFromString("0"),
		CacheOutputUSDPer1M: decimal.RequireFromString("0"),
		Status:              1,
	}); err != nil {
		t.Fatalf("CreateManagedModel: %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      "gpt-valid",
		UpstreamModel: "gpt-valid",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(valid): %v", err)
	}
	if _, err := st.CreateChannelModel(ctx, ChannelModelCreate{
		ChannelID:     channelID,
		PublicID:      "ghost-model",
		UpstreamModel: "ghost-model",
		Status:        1,
	}); err != nil {
		t.Fatalf("CreateChannelModel(stale): %v", err)
	}

	if err := EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema(second): %v", err)
	}

	bindings, err := st.ListChannelModelsByChannelID(ctx, channelID)
	if err != nil {
		t.Fatalf("ListChannelModelsByChannelID: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding after cleanup, got %d", len(bindings))
	}
	if bindings[0].PublicID != "gpt-valid" {
		t.Fatalf("binding public_id=%q want=%q", bindings[0].PublicID, "gpt-valid")
	}
}
