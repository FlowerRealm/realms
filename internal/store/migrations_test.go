package store

import (
	"strings"
	"testing"
)

func TestMigration0038_PolicyToFeatureBans(t *testing.T) {
	b, err := migrationsFS.ReadFile("migrations/0038_policy_to_feature_bans.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	text := string(b)
	for _, needle := range []string{
		"feature_disable_billing",
		"feature_disable_models",
		"policy_free_mode",
		"policy_model_passthrough",
		"feature_disable_web_models",
		"feature_disable_admin_models",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}

	stmts := splitSQLStatements(text)
	if len(stmts) != 3 {
		t.Fatalf("unexpected stmt count: %d", len(stmts))
	}
}

func TestMigration0064_UsageSearchIndexes(t *testing.T) {
	b, err := migrationsFS.ReadFile("migrations/0064_usage_search_indexes.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	text := string(b)
	if strings.Contains(text, "(`model`(191))") || strings.Contains(text, "(191)") {
		t.Fatalf("migration must not use prefix index for model")
	}
	if !strings.Contains(text, "CREATE INDEX `idx_usage_events_model` ON `usage_events` (`model`)") {
		t.Fatalf("migration missing idx_usage_events_model full-column index")
	}

	stmts := splitSQLStatements(text)
	if len(stmts) != 20 {
		t.Fatalf("unexpected stmt count: %d", len(stmts))
	}
}

func TestMigration0069_UpstreamChannelsAllowServiceTierDefault(t *testing.T) {
	b, err := migrationsFS.ReadFile("migrations/0069_upstream_channels_allow_service_tier_default.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	text := string(b)
	for _, needle := range []string{
		"MODIFY COLUMN `allow_service_tier` TINYINT NOT NULL DEFAULT 1",
		"UPDATE `upstream_channels`",
		"WHERE `fast_mode` = 1 AND `allow_service_tier` = 0",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}

	stmts := splitSQLStatements(text)
	if len(stmts) != 6 {
		t.Fatalf("unexpected stmt count: %d", len(stmts))
	}
}

func TestMigration0072_UsageEventsGroupPathText(t *testing.T) {
	b, err := migrationsFS.ReadFile("migrations/0072_usage_events_group_path_text.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	text := string(b)
	for _, needle := range []string{
		"column_name = 'price_multiplier_group_name'",
		"DATA_TYPE",
		"MODIFY COLUMN `price_multiplier_group_name` TEXT NULL",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}

	stmts := splitSQLStatements(text)
	if len(stmts) != 5 {
		t.Fatalf("unexpected stmt count: %d", len(stmts))
	}
}
