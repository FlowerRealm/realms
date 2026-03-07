package personalconfig

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/skills"
	"realms/internal/store"
)

func TestRebuildRuntimeFromBundle_RoundTripWithSecretsAndDeletions(t *testing.T) {
	ctx := context.Background()

	// Source runtime.
	{
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "src.db") + "?_busy_timeout=1000"
		db, err := store.OpenSQLite(dbPath)
		if err != nil {
			t.Fatalf("OpenSQLite(src): %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := store.EnsureSQLiteSchema(db); err != nil {
			t.Fatalf("EnsureSQLiteSchema(src): %v", err)
		}
		st := store.New(db)
		st.SetDialect(store.DialectSQLite)

		gid, err := st.CreateChannelGroup(ctx, "g1", nil, 1, decimal.NewFromInt(1))
		if err != nil {
			t.Fatalf("CreateChannelGroup: %v", err)
		}

		chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", "g1", 10, false, false, false, false)
		if err != nil {
			t.Fatalf("CreateUpstreamChannel: %v", err)
		}
		if err := st.UpdateUpstreamChannelRequestPolicy(ctx, chID, false, false, false, false); err != nil {
			t.Fatalf("UpdateUpstreamChannelRequestPolicy: %v", err)
		}
		if _, err := st.CreateUpstreamEndpoint(ctx, chID, "https://example.com/v1", 0); err != nil {
			t.Fatalf("CreateUpstreamEndpoint: %v", err)
		}
		ep, err := st.GetUpstreamEndpointByChannelID(ctx, chID)
		if err != nil {
			t.Fatalf("GetUpstreamEndpointByChannelID: %v", err)
		}
		if _, _, err := st.CreateOpenAICompatibleCredential(ctx, ep.ID, strPtr("k1"), "sk-test-abcdef123456"); err != nil {
			t.Fatalf("CreateOpenAICompatibleCredential: %v", err)
		}
		if err := st.AddChannelGroupMemberChannel(ctx, gid, chID, 0, false); err != nil {
			t.Fatalf("AddChannelGroupMemberChannel: %v", err)
		}

		mmID, err := st.CreateManagedModel(ctx, store.ManagedModelCreate{
			PublicID:            "m1",
			GroupName:           "",
			OwnedBy:             strPtr("openai"),
			InputUSDPer1M:       decimal.NewFromInt(1),
			OutputUSDPer1M:      decimal.NewFromInt(2),
			CacheInputUSDPer1M:  decimal.NewFromInt(1),
			CacheOutputUSDPer1M: decimal.NewFromInt(1),
			Status:              1,
		})
		if err != nil || mmID <= 0 {
			t.Fatalf("CreateManagedModel: id=%d err=%v", mmID, err)
		}
		bindID, err := st.CreateChannelModel(ctx, store.ChannelModelCreate{
			ChannelID:     chID,
			PublicID:      "m1",
			UpstreamModel: "m1",
			Status:        1,
		})
		if err != nil || bindID <= 0 {
			t.Fatalf("CreateChannelModel: id=%d err=%v", bindID, err)
		}

		admin, err := st.ExportAdminConfig(ctx)
		if err != nil {
			t.Fatalf("ExportAdminConfig: %v", err)
		}
		sec, err := func() (Secrets, error) {
			creds, err := st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
			if err != nil || len(creds) == 0 {
				return Secrets{}, err
			}
			s, err := st.GetOpenAICompatibleCredentialSecret(ctx, creds[0].ID)
			if err != nil {
				return Secrets{}, err
			}
			return Secrets{
				OpenAICompatible: []EndpointSecrets{{
					ChannelType: store.UpstreamTypeOpenAICompatible,
					ChannelName: "ch1",
					BaseURL:     ep.BaseURL,
					Credentials: []CredentialSecret{{Name: s.Name, APIKey: s.APIKey}},
				}},
			}, nil
		}()
		if err != nil {
			t.Fatalf("export secrets: %v", err)
		}
		bundle := Bundle{
			Version: BundleVersion,
			Admin:   admin,
			Secrets: &sec,
		}

		// Target runtime (with extra junk to be deleted).
		tdir := t.TempDir()
		dstPath := filepath.Join(tdir, "dst.db") + "?_busy_timeout=1000"
		dstDB, err := store.OpenSQLite(dstPath)
		if err != nil {
			t.Fatalf("OpenSQLite(dst): %v", err)
		}
		t.Cleanup(func() { _ = dstDB.Close() })
		if err := store.EnsureSQLiteSchema(dstDB); err != nil {
			t.Fatalf("EnsureSQLiteSchema(dst): %v", err)
		}
		dstStore := store.New(dstDB)
		dstStore.SetDialect(store.DialectSQLite)

		junkChID, err := dstStore.CreateUpstreamChannel(ctx, store.UpstreamTypeAnthropic, "junk", "", 0, false, false, false, false)
		if err != nil {
			t.Fatalf("CreateUpstreamChannel(junk): %v", err)
		}
		_, _ = dstStore.CreateUpstreamEndpoint(ctx, junkChID, "https://junk.example/v1", 0)

		if err := RebuildRuntimeFromBundle(ctx, dstDB, store.DialectSQLite, bundle); err != nil {
			t.Fatalf("RebuildRuntimeFromBundle: %v", err)
		}

		chs, err := dstStore.ListUpstreamChannels(ctx)
		if err != nil {
			t.Fatalf("ListUpstreamChannels(dst): %v", err)
		}
		var ch1 *store.UpstreamChannel
		for i := range chs {
			if chs[i].Type == store.UpstreamTypeAnthropic && chs[i].Name == "junk" {
				t.Fatalf("expected junk channel to be deleted, got %#v", chs[i])
			}
			if chs[i].Type == store.UpstreamTypeOpenAICompatible && chs[i].Name == "ch1" {
				ch1 = &chs[i]
			}
		}
		if ch1 == nil {
			t.Fatalf("expected ch1 to exist after rebuild, got %#v", chs)
		}
		if ch1.FastMode {
			t.Fatalf("expected ch1.fast_mode=false after rebuild, got true")
		}
		ep2, err := dstStore.GetUpstreamEndpointByChannelID(ctx, ch1.ID)
		if err != nil {
			t.Fatalf("GetUpstreamEndpointByChannelID(dst): %v", err)
		}
		if ep2.BaseURL != "https://example.com/v1" {
			t.Fatalf("base_url mismatch: %q", ep2.BaseURL)
		}
		creds2, err := dstStore.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep2.ID)
		if err != nil {
			t.Fatalf("ListOpenAICompatibleCredentialsByEndpoint(dst): %v", err)
		}
		if len(creds2) != 1 {
			t.Fatalf("expected 1 credential, got %d", len(creds2))
		}
		sec2, err := dstStore.GetOpenAICompatibleCredentialSecret(ctx, creds2[0].ID)
		if err != nil {
			t.Fatalf("GetOpenAICompatibleCredentialSecret(dst): %v", err)
		}
		if sec2.APIKey != "sk-test-abcdef123456" {
			t.Fatalf("api key mismatch: %q", sec2.APIKey)
		}
	}
}

func TestRebuildRuntimeFromBundle_AppliesSkillsStoreV1(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dst.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	admin, err := st.ExportAdminConfig(ctx)
	if err != nil {
		t.Fatalf("ExportAdminConfig: %v", err)
	}

	sv1 := skills.StoreV1{
		Version: 1,
		Skills: map[string]skills.SkillV1{
			"skill1": {ID: "skill1", Title: "t", Prompt: "p"},
		},
	}
	sv1JSON, err := skills.PrettyStoreV1JSON(sv1)
	if err != nil {
		t.Fatalf("PrettyStoreV1JSON: %v", err)
	}
	teJSON, err := skills.PrettyTargetEnabledV1JSON(skills.TargetEnabledV1{})
	if err != nil {
		t.Fatalf("PrettyTargetEnabledV1JSON: %v", err)
	}

	bundle := Bundle{
		Version:               BundleVersion,
		Admin:                 admin,
		SkillsStoreV1:         []byte(sv1JSON),
		SkillsTargetEnabledV1: []byte(teJSON),
	}

	if err := RebuildRuntimeFromBundle(ctx, db, store.DialectSQLite, bundle); err != nil {
		t.Fatalf("RebuildRuntimeFromBundle: %v", err)
	}

	raw, ok, err := st.GetStringAppSetting(ctx, store.SettingSkillsStoreV1)
	if err != nil || !ok {
		t.Fatalf("skills store not written: ok=%v err=%v", ok, err)
	}
	if _, err := skills.ParseStoreV1JSON(raw); err != nil {
		t.Fatalf("stored skills json invalid: %v", err)
	}
}

func strPtr(s string) *string { return &s }

func TestRebuildRuntimeFromBundle_NormalizesLegacyFastModeServiceTierConflict(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dst.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}
	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	fastMode := true
	bundle := Bundle{
		Version: BundleVersion,
		Admin: store.AdminConfigExport{
			Version: 8,
			UpstreamChannels: []store.AdminConfigUpstreamChannel{{
				Type:             store.UpstreamTypeOpenAICompatible,
				Name:             "legacy-fast",
				Status:           1,
				Priority:         0,
				AllowServiceTier: false,
				FastMode:         &fastMode,
			}},
			UpstreamEndpoints: []store.AdminConfigUpstreamEndpoint{{
				ChannelType: store.UpstreamTypeOpenAICompatible,
				ChannelName: "legacy-fast",
				BaseURL:     "https://api.openai.com",
			}},
		},
	}

	if err := RebuildRuntimeFromBundle(ctx, db, store.DialectSQLite, bundle); err != nil {
		t.Fatalf("RebuildRuntimeFromBundle: %v", err)
	}
	chs, err := st.ListUpstreamChannels(ctx)
	if err != nil {
		t.Fatalf("ListUpstreamChannels: %v", err)
	}
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}
	if !chs[0].FastMode {
		t.Fatalf("expected fast_mode=true after rebuild, got false")
	}
	if !chs[0].AllowServiceTier {
		t.Fatalf("expected allow_service_tier normalized to true after rebuild, got false")
	}
}
