package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyStore_ClaudeSkillsLayoutWritesToDir(t *testing.T) {
	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, "skills")
	t.Setenv(EnvClaudeCommandsDir, claudeDir)

	store := StoreV1{
		Version: 1,
		Skills: map[string]SkillV1{
			"skill1": {ID: "skill1", Title: "S1", Prompt: "p1"},
		},
	}

	_, err := ApplyStore(store, ApplyOptions{Targets: []Target{TargetClaude}, TargetEnabled: TargetEnabledV1{}})
	if err != nil {
		t.Fatalf("apply err=%v", err)
	}
	p := filepath.Join(claudeDir, "skill1", "SKILL.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "name:") || !strings.Contains(s, "description:") {
		t.Fatalf("missing frontmatter: %q", s)
	}
	if !strings.Contains(s, "p1") {
		t.Fatalf("missing prompt: %q", s)
	}
}
