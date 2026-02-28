package skills

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// parseFrontmatter parses YAML frontmatter at the top of a markdown string.
// It only parses if the first non-empty characters begin with a standalone "---" line.
// Returns meta, body, ok.
func parseFrontmatter(md string) (map[string]any, string, bool) {
	s := strings.ReplaceAll(md, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", false
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 3 {
		return nil, "", false
	}
	if strings.TrimSpace(lines[0]) != "---" {
		return nil, "", false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, "", false
	}
	yamlPart := strings.Join(lines[1:end], "\n")
	bodyPart := strings.Join(lines[end+1:], "\n")
	bodyPart = strings.TrimLeft(bodyPart, "\n")
	bodyPart = strings.TrimSpace(bodyPart)

	meta := map[string]any{}
	if err := yaml.Unmarshal([]byte(yamlPart), &meta); err != nil {
		return nil, strings.TrimSpace(s), false
	}
	return meta, bodyPart, true
}

func stringFromMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return ""
	}
}

func renderSkillMarkdown(name string, description string, body string) string {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	body = strings.TrimSpace(body)
	if name == "" {
		name = "skill"
	}
	if description == "" {
		description = "Explicit invocation only. (auto-generated)"
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(yamlScalar(name))
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(yamlScalar(description))
	b.WriteString("\n")
	b.WriteString("---\n\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func yamlScalar(s string) string {
	// Minimal escaping: YAML plain scalars break on ':' in ambiguous positions, newlines, or leading/trailing spaces.
	// Use double-quoted scalars with JSON-style escapes for safety.
	if s == "" {
		return "\"\""
	}
	needQuote := strings.ContainsAny(s, ":\n\r\t\"\\") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ")
	if !needQuote {
		return s
	}
	esc := strings.ReplaceAll(s, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	esc = strings.ReplaceAll(esc, "\n", `\n`)
	esc = strings.ReplaceAll(esc, "\r", `\r`)
	esc = strings.ReplaceAll(esc, "\t", `\t`)
	return `"` + esc + `"`
}
