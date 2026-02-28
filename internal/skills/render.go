package skills

import (
	"fmt"
	"strings"
)

func RenderForTarget(sk SkillV1, t Target) ([]byte, error) {
	prompt := strings.TrimSpace(sk.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("skill[%s] prompt is empty", sk.ID)
	}

	switch t {
	case TargetCodex:
		// Keep codex skills as raw content so scan/import/adopt can roundtrip without rewriting.
		return []byte(prompt + "\n"), nil
	case TargetClaude:
		// Keep claude commands as raw markdown content.
		return []byte(prompt + "\n"), nil
	case TargetGemini:
		title := strings.TrimSpace(sk.Title)
		if title == "" {
			title = sk.ID
		}
		desc := ""
		if sk.Description != nil {
			desc = strings.TrimSpace(*sk.Description)
		}
		// Gemini commands are TOML. Use stable keys.
		// NOTE: keep prompt as multi-line basic string; escape """ if present.
		p := strings.ReplaceAll(prompt, `"""`, `\"\"\"`)
		d := strings.ReplaceAll(desc, `"`, `\"`)
		tit := strings.ReplaceAll(title, `"`, `\"`)
		var b strings.Builder
		b.WriteString("title = \"")
		b.WriteString(tit)
		b.WriteString("\"\n")
		if d != "" {
			b.WriteString("description = \"")
			b.WriteString(d)
			b.WriteString("\"\n")
		}
		b.WriteString("prompt = \"\"\"\n")
		b.WriteString(p)
		if !strings.HasSuffix(p, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\"\"\"\n")
		return []byte(b.String()), nil
	default:
		return nil, fmt.Errorf("unknown target: %s", t)
	}
}
