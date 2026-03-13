package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"realms/internal/store"
)

func TestUpstreamChannel_FastModeDefaultsTrueAndCanBeDisabled(t *testing.T) {
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
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "fast-default", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}
	if !ch.AllowServiceTier {
		t.Fatalf("expected allow_service_tier=true by default, got false")
	}
	if !ch.FastMode {
		t.Fatalf("expected fast_mode=true by default, got false")
	}

	if err := st.UpdateUpstreamChannelRequestPolicy(ctx, channelID, ch.AllowServiceTier, ch.DisableStore, ch.AllowSafetyIdentifier, false); err != nil {
		t.Fatalf("UpdateUpstreamChannelRequestPolicy: %v", err)
	}

	ch, err = st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID(after): %v", err)
	}
	if ch.FastMode {
		t.Fatalf("expected fast_mode=false after update, got true")
	}
}

func TestUpstreamChannel_FastModeRequiresAllowServiceTier(t *testing.T) {
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
	if _, err := st.CreateUpstreamChannelWithRequestPolicy(ctx, store.UpstreamTypeOpenAICompatible, "bad-fast", "", 0, false, false, false, false, true); err == nil {
		t.Fatalf("expected create to fail when fast_mode=true and allow_service_tier=false")
	} else if err.Error() != store.ErrUpstreamChannelFastModeRequiresServiceTier.Error() {
		t.Fatalf("unexpected create error: %v", err)
	}

	channelID, err := st.CreateUpstreamChannelWithRequestPolicy(ctx, store.UpstreamTypeOpenAICompatible, "good-fast", "", 0, false, true, false, false, true)
	if err != nil {
		t.Fatalf("CreateUpstreamChannelWithRequestPolicy: %v", err)
	}
	if err := st.UpdateUpstreamChannelRequestPolicy(ctx, channelID, false, false, false, true); err == nil {
		t.Fatalf("expected update to fail when fast_mode=true and allow_service_tier=false")
	} else if err.Error() != store.ErrUpstreamChannelFastModeRequiresServiceTier.Error() {
		t.Fatalf("unexpected update error: %v", err)
	}
}

func TestEnsureSQLiteSchema_BackfillsAllowServiceTierForFastMode(t *testing.T) {
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

	if _, err := db.Exec(`UPDATE upstream_channels SET allow_service_tier=0 WHERE 1=0`); err != nil {
		t.Fatalf("warmup update failed: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)
	ctx := context.Background()
	channelID, err := st.CreateUpstreamChannelWithRequestPolicy(ctx, store.UpstreamTypeOpenAICompatible, "needs-backfill", "", 0, false, true, false, false, true)
	if err != nil {
		t.Fatalf("CreateUpstreamChannelWithRequestPolicy: %v", err)
	}
	if _, err := db.Exec(`UPDATE upstream_channels SET allow_service_tier=0 WHERE id=?`, channelID); err != nil {
		t.Fatalf("make contradiction failed: %v", err)
	}

	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema(second): %v", err)
	}

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID: %v", err)
	}
	if !ch.FastMode {
		t.Fatalf("expected fast_mode=true after backfill, got false")
	}
	if !ch.AllowServiceTier {
		t.Fatalf("expected allow_service_tier=true after backfill, got false")
	}
}

func TestUpstreamChannel_DefaultAPISettings_ByChannelType(t *testing.T) {
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

	openaiID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "openai", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(openai): %v", err)
	}
	codexID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "codex", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(codex): %v", err)
	}

	openaiCh, err := st.GetUpstreamChannelByID(ctx, openaiID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID(openai): %v", err)
	}
	if !openaiCh.Setting.ChatCompletionsEnabled || !openaiCh.Setting.ResponsesEnabled {
		t.Fatalf("expected openai channel to enable both APIs, got %+v", openaiCh.Setting)
	}

	codexCh, err := st.GetUpstreamChannelByID(ctx, codexID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID(codex): %v", err)
	}
	if codexCh.Setting.ChatCompletionsEnabled || !codexCh.Setting.ResponsesEnabled {
		t.Fatalf("expected codex channel to only enable responses, got %+v", codexCh.Setting)
	}
}

func TestUpstreamChannel_UpdateSetting_RejectsInvalidAPICapabilities(t *testing.T) {
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

	openaiID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "openai", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(openai): %v", err)
	}
	if err := st.UpdateUpstreamChannelNewAPISetting(ctx, openaiID, store.UpstreamChannelSetting{
		ChatCompletionsEnabled: false,
		ResponsesEnabled:       false,
	}); err == nil {
		t.Fatalf("expected disabling all APIs to fail")
	}

	codexID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "codex", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(codex): %v", err)
	}
	if err := st.UpdateUpstreamChannelNewAPISetting(ctx, codexID, store.UpstreamChannelSetting{
		ChatCompletionsEnabled: true,
		ResponsesEnabled:       true,
	}); err == nil {
		t.Fatalf("expected codex chat/completions enable to fail")
	}
}

func TestUpstreamChannel_AnthropicAllowsEmptyAPISettings(t *testing.T) {
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

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeAnthropic, "anthropic", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel(anthropic): %v", err)
	}

	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID(anthropic): %v", err)
	}
	if ch.Setting.ChatCompletionsEnabled || ch.Setting.ResponsesEnabled {
		t.Fatalf("expected anthropic openai api capabilities to remain disabled, got %+v", ch.Setting)
	}

	if err := st.UpdateUpstreamChannelNewAPISetting(ctx, channelID, store.UpstreamChannelSetting{
		ChatCompletionsEnabled: true,
		ResponsesEnabled:       true,
	}); err != nil {
		t.Fatalf("UpdateUpstreamChannelNewAPISetting(anthropic): %v", err)
	}
	ch, err = st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamChannelByID(anthropic after update): %v", err)
	}
	if ch.Setting.ChatCompletionsEnabled || ch.Setting.ResponsesEnabled {
		t.Fatalf("expected anthropic update to clear openai api capabilities, got %+v", ch.Setting)
	}
}
