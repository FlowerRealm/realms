package skills

import (
	"strings"
	"testing"
)

func TestRenderForTarget(t *testing.T) {
	desc := "d"
	sk := SkillV1{ID: "a", Title: "A", Description: &desc, Prompt: "hello\nworld"}
	for _, tgt := range []Target{TargetCodex, TargetClaude, TargetGemini} {
		root := ""
		if tgt == TargetClaude {
			root = "/tmp/commands"
		}
		b, err := RenderForTargetInDir(sk, tgt, root)
		if err != nil {
			t.Fatalf("render %s err=%v", tgt, err)
		}
		s := string(b)
		if !strings.Contains(s, "hello") {
			t.Fatalf("render %s missing prompt", tgt)
		}
		switch tgt {
		case TargetCodex:
			if !strings.Contains(s, "name:") || !strings.Contains(s, "description:") {
				t.Fatalf("render %s missing frontmatter: %q", tgt, s)
			}
		case TargetClaude:
			if !strings.Contains(s, "description:") {
				t.Fatalf("render %s missing description: %q", tgt, s)
			}
		case TargetGemini:
			if strings.Contains(s, "title =") {
				t.Fatalf("render %s must not include title: %q", tgt, s)
			}
			if !strings.Contains(s, "prompt =") {
				t.Fatalf("render %s missing prompt key: %q", tgt, s)
			}
		}
	}
}
