package upstream

import (
	_ "embed"
	"strings"
)

// 说明：Codex 上游（chatgpt.com/backend-api/codex）会对 instructions 做校验。
// 这里内置一组与 Codex CLI/CLIProxyAPI 对齐的官方 prompt，用于请求兜底与兼容转换。
//
// Prompts 来源：router-for-me/CLIProxyAPI（MIT License）。为避免上游校验失败，内容需保持原样。

//go:embed codex_instructions/gpt_5_2_prompt.md
var codexPromptGPT52 string

//go:embed codex_instructions/gpt-5.2-codex_prompt.md
var codexPromptGPT52Codex string

func codexInstructionsForModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "5.2-codex"):
		if codexPromptGPT52Codex != "" {
			return codexPromptGPT52Codex
		}
	case strings.Contains(m, "5.2"):
		if codexPromptGPT52 != "" {
			return codexPromptGPT52
		}
	}
	// 兜底：即便模型不匹配，也返回一个已知可用的 prompt，避免上游返回 instructions 错误。
	if codexPromptGPT52 != "" {
		return codexPromptGPT52
	}
	return codexPromptGPT52Codex
}

// CodexInstructionsForModel returns the official Codex instructions prompt for the given model.
// It is used by internal components (e.g. channel health checks) to generate requests accepted by
// the Codex OAuth upstream.
func CodexInstructionsForModel(model string) string {
	return codexInstructionsForModel(model)
}

func isOfficialCodexInstructions(instructions string) bool {
	if strings.TrimSpace(instructions) == "" {
		return false
	}
	instructions = strings.TrimSpace(instructions)
	if codexPromptGPT52 != "" && strings.HasPrefix(instructions, strings.TrimSpace(codexPromptGPT52)) {
		return true
	}
	if codexPromptGPT52Codex != "" && strings.HasPrefix(instructions, strings.TrimSpace(codexPromptGPT52Codex)) {
		return true
	}
	return false
}
