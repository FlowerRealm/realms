package upstream

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	opencodeCodexHeaderURL = "https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/opencode/src/session/prompt/codex_header.txt"
	codexCacheTTL          = 15 * time.Minute
)

//go:embed prompts/codex_cli_instructions.md
var codexCLIInstructions string

type opencodeCacheMetadata struct {
	ETag        string `json:"etag"`
	LastFetch   string `json:"lastFetch,omitempty"`
	LastChecked int64  `json:"lastChecked"`
}

func getOpenCodeCachedPrompt(url, cacheFileName, metaFileName string) string {
	cacheDir := codexCachePath("")
	if cacheDir == "" {
		return ""
	}
	cacheFile := filepath.Join(cacheDir, cacheFileName)
	metaFile := filepath.Join(cacheDir, metaFileName)

	var cachedContent string
	if content, ok := readFile(cacheFile); ok {
		cachedContent = content
	}

	var meta opencodeCacheMetadata
	if loadJSON(metaFile, &meta) && meta.LastChecked > 0 && cachedContent != "" {
		if time.Since(time.UnixMilli(meta.LastChecked)) < codexCacheTTL {
			return cachedContent
		}
	}

	content, etag, status, err := fetchWithETag(url, meta.ETag)
	if err == nil && status == http.StatusNotModified && cachedContent != "" {
		return cachedContent
	}
	if err == nil && status >= 200 && status < 300 && content != "" {
		_ = writeFile(cacheFile, content)
		meta = opencodeCacheMetadata{
			ETag:        etag,
			LastFetch:   time.Now().UTC().Format(time.RFC3339),
			LastChecked: time.Now().UnixMilli(),
		}
		_ = writeJSON(metaFile, meta)
		return content
	}

	return cachedContent
}

func getOpenCodeCodexHeader() string {
	// 优先从 opencode 仓库缓存获取指令。
	opencodeInstructions := getOpenCodeCachedPrompt(opencodeCodexHeaderURL, "opencode-codex-header.txt", "opencode-codex-header-meta.json")

	// 若 opencode 指令可用，直接返回。
	if opencodeInstructions != "" {
		return opencodeInstructions
	}

	// 否则回退使用本地 Codex CLI 指令。
	return getCodexCLIInstructions()
}

func getCodexCLIInstructions() string {
	return codexCLIInstructions
}

// applyInstructions 处理 instructions 字段
// isCodexCLI=true: 仅补充缺失的 instructions（使用 opencode 指令）
// isCodexCLI=false: 优先使用 opencode 指令覆盖
func applyInstructions(reqBody map[string]any, isCodexCLI bool) bool {
	if isCodexCLI {
		return applyCodexCLIInstructions(reqBody)
	}
	return applyOpenCodeInstructions(reqBody)
}

// applyCodexCLIInstructions 为 Codex CLI 请求补充缺失的 instructions
// 仅在 instructions 为空时添加 opencode 指令
func applyCodexCLIInstructions(reqBody map[string]any) bool {
	if !isInstructionsEmpty(reqBody) {
		return false // 已有有效 instructions，不修改
	}

	instructions := strings.TrimSpace(getOpenCodeCodexHeader())
	if instructions != "" {
		reqBody["instructions"] = instructions
		return true
	}

	return false
}

// applyOpenCodeInstructions 为非 Codex CLI 请求应用 opencode 指令
// 优先使用 opencode 指令覆盖
func applyOpenCodeInstructions(reqBody map[string]any) bool {
	instructions := strings.TrimSpace(getOpenCodeCodexHeader())
	existingInstructions, _ := reqBody["instructions"].(string)
	existingInstructions = strings.TrimSpace(existingInstructions)

	if instructions != "" {
		if existingInstructions != instructions {
			reqBody["instructions"] = instructions
			return true
		}
	} else if existingInstructions == "" {
		codexInstructions := strings.TrimSpace(getCodexCLIInstructions())
		if codexInstructions != "" {
			reqBody["instructions"] = codexInstructions
			return true
		}
	}

	return false
}

// isInstructionsEmpty 检查 instructions 字段是否为空
// 处理以下情况：字段不存在、nil、空字符串、纯空白字符串
func isInstructionsEmpty(reqBody map[string]any) bool {
	val, exists := reqBody["instructions"]
	if !exists {
		return true
	}
	if val == nil {
		return true
	}
	str, ok := val.(string)
	if !ok {
		return true
	}
	return strings.TrimSpace(str) == ""
}

func codexCachePath(filename string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cacheDir := filepath.Join(home, ".opencode", "cache")
	if filename == "" {
		return cacheDir
	}
	return filepath.Join(cacheDir, filename)
}

func readFile(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func writeFile(path, content string) error {
	if path == "" {
		return fmt.Errorf("empty cache path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func loadJSON(path string, target any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false
	}
	return true
}

func writeJSON(path string, value any) error {
	if path == "" {
		return fmt.Errorf("empty json path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func fetchWithETag(url, etag string) (string, string, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("User-Agent", "sub2api-codex")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", resp.StatusCode, err
	}
	return string(body), resp.Header.Get("etag"), resp.StatusCode, nil
}

