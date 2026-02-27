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
