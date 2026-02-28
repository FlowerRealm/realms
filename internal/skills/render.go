package skills

import (
	"fmt"
	"sort"
	"strings"
)

func RenderForTarget(sk SkillV1, t Target) ([]byte, error) {
	if t == TargetClaude {
		root, _ := ResolveTargetDir(TargetClaude)
		return RenderForTargetInDir(sk, t, root)
	}
	return RenderForTargetInDir(sk, t, "")
}

func RenderForTargetInDir(sk SkillV1, t Target, root string) ([]byte, error) {
	prompt := strings.TrimSpace(sk.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("skill[%s] prompt is empty", sk.ID)
	}

	switch t {
	case TargetCodex:
		name := sk.ID
		if v := nameForTarget(sk.InstallAs, t); v != nil && strings.TrimSpace(*v) != "" {
			name = strings.TrimSpace(*v)
		}
		desc := ""
		if sk.Description != nil {
			desc = strings.TrimSpace(*sk.Description)
		}
		if desc == "" {
			desc = "Explicit invocation only. (auto-generated)"
		}
		return []byte(renderSkillMarkdown(name, desc, prompt)), nil
	case TargetClaude:
		name := sk.ID
		if v := nameForTarget(sk.InstallAs, t); v != nil && strings.TrimSpace(*v) != "" {
			name = strings.TrimSpace(*v)
		}
		desc := ""
		if sk.Description != nil {
			desc = strings.TrimSpace(*sk.Description)
		}
		if desc == "" {
			desc = "Explicit invocation only. (auto-generated)"
		}
		if claudeUsesSkillsLayout(root) {
			// Legacy compatibility: treat as a Codex-like skill.
			return []byte(renderSkillMarkdown(name, desc, prompt)), nil
		}
		return []byte(renderClaudeCommandMarkdown(desc, prompt, claudeFrontmatter(sk))), nil
	case TargetGemini:
		desc := ""
		if sk.Description != nil {
			desc = strings.TrimSpace(*sk.Description)
		}
		// Gemini commands are TOML.
		// NOTE: keep prompt as multi-line basic string; escape """ if present.
		p := strings.ReplaceAll(prompt, `"""`, `\"\"\"`)
		d := strings.ReplaceAll(desc, `"`, `\"`)
		var b strings.Builder
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

func claudeFrontmatter(sk SkillV1) map[string]any {
	if sk.PerTarget == nil || sk.PerTarget.Claude == nil || len(sk.PerTarget.Claude.Frontmatter) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range sk.PerTarget.Claude.Frontmatter {
		out[k] = v
	}
	return out
}

func renderClaudeCommandMarkdown(description string, body string, fm map[string]any) string {
	description = strings.TrimSpace(description)
	body = strings.TrimSpace(body)
	if description == "" {
		description = "Explicit invocation only. (auto-generated)"
	}

	// Frontmatter is optional, but Claude's SlashCommand tool requires `description`.
	// Keep output stable: write known keys first, then the rest sorted.
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("description: ")
	b.WriteString(yamlScalar(description))
	b.WriteString("\n")

	known := []string{"allowed-tools", "argument-hint", "model", "disable-model-invocation"}
	seen := map[string]bool{"description": true}

	for _, k := range known {
		if fm == nil {
			continue
		}
		v, ok := fm[k]
		if !ok {
			continue
		}
		writeClaudeFrontmatterKV(&b, k, v)
		seen[k] = true
	}

	// Write remaining keys in lexicographic order for stable diffs.
	if fm != nil {
		rest := make([]string, 0, len(fm))
		for k := range fm {
			if seen[k] {
				continue
			}
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range rest {
			writeClaudeFrontmatterKV(&b, k, fm[k])
		}
	}

	b.WriteString("---\n\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func writeClaudeFrontmatterKV(b *strings.Builder, k string, v any) {
	k = strings.TrimSpace(k)
	if k == "" {
		return
	}
	switch x := v.(type) {
	case string:
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(yamlScalar(strings.TrimSpace(x)))
		b.WriteString("\n")
	case bool:
		b.WriteString(k)
		b.WriteString(": ")
		if x {
			b.WriteString("true\n")
		} else {
			b.WriteString("false\n")
		}
	default:
		// best-effort: ignore complex types to avoid emitting invalid YAML
	}
}
