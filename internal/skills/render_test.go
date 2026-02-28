package skills

import (
	"strings"
	"testing"
)

func TestRenderForTarget(t *testing.T) {
	desc := "d"
	sk := SkillV1{ID: "a", Title: "A", Description: &desc, Prompt: "hello\nworld"}
	for _, tgt := range []Target{TargetCodex, TargetClaude, TargetGemini} {
		b, err := RenderForTarget(sk, tgt)
		if err != nil {
			t.Fatalf("render %s err=%v", tgt, err)
		}
		s := string(b)
		if !strings.Contains(s, "hello") {
			t.Fatalf("render %s missing prompt", tgt)
		}
	}
}
