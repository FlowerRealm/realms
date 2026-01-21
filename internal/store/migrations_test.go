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
