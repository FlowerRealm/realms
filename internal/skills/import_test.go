package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromTarget_CodexSkillParsesFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "skills")
	dir := filepath.Join(root, "skill1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: Skill One\ndescription: desc\n---\n\nline1\nline2\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _, err := ImportFromTarget(TargetCodex, root)
	if err != nil {
		t.Fatal(err)
	}
	sk, ok := s.Skills["skill1"]
	if !ok {
		t.Fatalf("missing skill1: %+v", s.Skills)
	}
	if sk.Title != "Skill One" {
		t.Fatalf("title=%q", sk.Title)
	}
	if sk.Description == nil || *sk.Description != "desc" {
		t.Fatalf("description=%v", sk.Description)
	}
	if sk.Prompt != "line1\nline2" {
		t.Fatalf("prompt=%q", sk.Prompt)
	}
}
