package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyStore_ConflictAndResolution(t *testing.T) {
	tmp := t.TempDir()
	codexDir := filepath.Join(tmp, "codex")
	claudeDir := filepath.Join(tmp, "claude")
	geminiDir := filepath.Join(tmp, "gemini")

	t.Setenv(EnvCodexSkillsDir, codexDir)
	t.Setenv(EnvClaudeCommandsDir, claudeDir)
	t.Setenv(EnvGeminiCommandsDir, geminiDir)

	store := StoreV1{
		Version: 1,
		Skills: map[string]SkillV1{
			"skill1": {ID: "skill1", Title: "S1", Prompt: "p1"},
		},
	}

	out, err := ApplyStore(store, ApplyOptions{TargetEnabled: TargetEnabledV1{}})
	if err != nil {
		t.Fatalf("apply err=%v", err)
	}
	if len(out.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", out.Conflicts)
	}

	// Create conflict by modifying one target file.
	confPath := filepath.Join(claudeDir, "skill1.md")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(confPath, []byte("different\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out2, err2 := ApplyStore(store, ApplyOptions{Targets: []Target{TargetClaude}, TargetEnabled: TargetEnabledV1{}})
	if err2 != ErrConflicts {
		t.Fatalf("expected ErrConflicts, got %v", err2)
	}
	if len(out2.Conflicts) == 0 {
		t.Fatalf("expected conflicts")
	}

	// Resolve by overwrite.
	_, err3 := ApplyStore(store, ApplyOptions{
		Targets:       []Target{TargetClaude},
		TargetEnabled: TargetEnabledV1{},
		Resolutions: []ConflictResolution{
			{ID: "skill1", Target: TargetClaude, Action: ConflictOverwrite},
		},
	})
	if err3 != nil {
		t.Fatalf("apply overwrite err=%v", err3)
	}
	raw, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "p1") {
		t.Fatalf("expected overwritten content, got: %q", string(raw))
	}
}
