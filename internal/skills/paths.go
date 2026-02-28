package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultCodexSkillsRelDir = ".codex/skills"
	DefaultClaudeCommandsRel = ".claude/commands"
	DefaultGeminiCommandsRel = ".gemini/commands"
)

const (
	EnvCodexSkillsDir    = "REALMS_SKILLS_CODEX_DIR"
	EnvClaudeCommandsDir = "REALMS_SKILLS_CLAUDE_DIR"
	EnvGeminiCommandsDir = "REALMS_SKILLS_GEMINI_DIR"
)

func ResolveTargetDir(t Target) (string, error) {
	var envKey string
	var rel string
	switch t {
	case TargetCodex:
		envKey = EnvCodexSkillsDir
		rel = DefaultCodexSkillsRelDir
	case TargetClaude:
		envKey = EnvClaudeCommandsDir
		rel = DefaultClaudeCommandsRel
	case TargetGemini:
		envKey = EnvGeminiCommandsDir
		rel = DefaultGeminiCommandsRel
	default:
		return "", fmt.Errorf("unknown target: %s", t)
	}
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("cannot resolve user home dir")
	}
	return filepath.Join(home, rel), nil
}

func withinDir(root string, p string) bool {
	root = filepath.Clean(root)
	p = filepath.Clean(p)
	if root == p {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	return strings.HasPrefix(p, root)
}
