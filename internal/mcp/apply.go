package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultCodexConfigRelPath  = ".codex/config.toml"
	DefaultClaudeConfigRelPath = ".claude.json"
	DefaultGeminiConfigRelPath = ".gemini/settings.json"
)

const (
	EnvCodexConfigPath  = "REALMS_MCP_CODEX_CONFIG_PATH"
	EnvClaudeConfigPath = "REALMS_MCP_CLAUDE_CONFIG_PATH"
	EnvGeminiConfigPath = "REALMS_MCP_GEMINI_CONFIG_PATH"
)

type Target string

const (
	TargetCodex  Target = "codex"
	TargetClaude Target = "claude"
	TargetGemini Target = "gemini"
)

type ApplyResult struct {
	Target  Target `json:"target"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
	Changed bool   `json:"changed"`
	Exists  bool   `json:"exists"`
	Error   string `json:"error,omitempty"`
}

func ResolveTargetPath(t Target) (string, error) {
	var envKey string
	var rel string
	switch t {
	case TargetCodex:
		envKey = EnvCodexConfigPath
		rel = DefaultCodexConfigRelPath
	case TargetClaude:
		envKey = EnvClaudeConfigPath
		rel = DefaultClaudeConfigRelPath
	case TargetGemini:
		envKey = EnvGeminiConfigPath
		rel = DefaultGeminiConfigRelPath
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

func ApplyTarget(t Target, path string, reg Registry, force bool) (changed bool, err error) {
	return ApplyTargetWithRemovals(t, path, reg, nil, force)
}

func ApplyTargetWithRemovals(t Target, path string, reg Registry, removeIDs []string, force bool) (changed bool, err error) {
	if strings.TrimSpace(path) == "" {
		return false, errors.New("path is empty")
	}
	switch t {
	case TargetCodex:
		return ApplyCodexConfig(path, reg, removeIDs, runtime.GOOS, force)
	case TargetClaude:
		return ApplyClaudeConfig(path, reg, removeIDs, runtime.GOOS, force)
	case TargetGemini:
		return ApplyGeminiConfig(path, reg, removeIDs, force)
	default:
		return false, fmt.Errorf("unknown target: %s", t)
	}
}

func mergeServersMap(curAny any, removeIDs []string, newServers map[string]any) map[string]any {
	merged := map[string]any{}
	if curAny != nil {
		if cur, ok := curAny.(map[string]any); ok {
			for k, v := range cur {
				merged[k] = v
			}
		}
	}
	for _, id := range removeIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			delete(merged, id)
		}
	}
	for k, v := range newServers {
		merged[k] = v
	}
	return merged
}

func applyJSONConfigServersKey(path string, serverKey string, newServers map[string]any, removeIDs []string, force bool, invalidErrPrefix string) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("path is empty")
	}

	exists := fileExists(path)
	root := map[string]any{}
	if exists {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &root); err != nil {
				if !force {
					return false, fmt.Errorf("%s: %w", invalidErrPrefix, err)
				}
				root = map[string]any{}
			}
		}
	}

	before, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}

	root[serverKey] = mergeServersMap(root[serverKey], removeIDs, newServers)

	after, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	changed := !bytes.Equal(bytes.TrimSpace(before), bytes.TrimSpace(after))

	if err := writeFileAtomic(path, append(bytes.TrimSpace(after), '\n'), 0o600); err != nil {
		return false, err
	}
	return changed, nil
}

func ApplyCodexConfig(path string, reg Registry, removeIDs []string, platform string, force bool) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("path is empty")
	}

	exists := fileExists(path)
	var root map[string]any
	if exists {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) > 0 {
			if err := toml.Unmarshal(raw, &root); err != nil {
				if !force {
					return false, fmt.Errorf("invalid codex config toml: %w", err)
				}
				root = map[string]any{}
			}
		}
	}
	if root == nil {
		root = map[string]any{}
	}

	newServers := make(map[string]any, len(reg))
	isWSLTarget := isWSLPathForWindows(platform, path)
	for id, spec := range reg {
		copied, err := deepCopyObject(spec)
		if err != nil {
			return false, err
		}
		if strings.EqualFold(platform, "windows") && !isWSLTarget {
			wrapCommandForWindows(copied)
		}
		out, err := codexServerFromSpec(copied)
		if err != nil {
			return false, err
		}
		newServers[id] = out
	}

	before, err := toml.Marshal(root)
	if err != nil {
		return false, err
	}
	root["mcp_servers"] = mergeServersMap(root["mcp_servers"], removeIDs, newServers)
	after, err := toml.Marshal(root)
	if err != nil {
		return false, err
	}
	changed := !bytes.Equal(bytes.TrimSpace(before), bytes.TrimSpace(after))

	if err := writeFileAtomic(path, append(bytes.TrimSpace(after), '\n'), 0o600); err != nil {
		return false, err
	}
	return changed, nil
}

func codexServerFromSpec(spec map[string]any) (map[string]any, error) {
	out := map[string]any{}

	typ := strings.TrimSpace(stringFromAny(spec["type"]))
	if typ == "" {
		if strings.TrimSpace(stringFromAny(spec["command"])) != "" {
			typ = "stdio"
		} else if strings.TrimSpace(stringFromAny(spec["url"])) != "" {
			typ = "sse"
		}
	}

	switch typ {
	case "stdio":
		cmd := strings.TrimSpace(stringFromAny(spec["command"]))
		if cmd == "" {
			return nil, errors.New("codex stdio server missing command")
		}
		out["command"] = cmd
		if args, ok := toStringArray(spec["args"]); ok {
			out["args"] = args
		}
		if cwd := strings.TrimSpace(stringFromAny(spec["cwd"])); cwd != "" {
			out["cwd"] = cwd
		}
		if env, ok := toStringMap(spec["env"]); ok && len(env) > 0 {
			out["env"] = env
		}
		if envVars, ok := toStringArray(spec["env_vars"]); ok && len(envVars) > 0 {
			out["env_vars"] = envVars
		}
	case "http", "sse":
		url := strings.TrimSpace(stringFromAny(spec["url"]))
		if url == "" {
			return nil, errors.New("codex http/sse server missing url")
		}
		out["url"] = url
		if v := strings.TrimSpace(stringFromAny(spec["bearer_token"])); v != "" {
			out["bearer_token"] = v
		}
		if v := strings.TrimSpace(stringFromAny(spec["bearer_token_env_var"])); v != "" {
			out["bearer_token_env_var"] = v
		}
		if hdrs, ok := toStringMap(spec["http_headers"]); ok && len(hdrs) > 0 {
			out["http_headers"] = hdrs
		}
		if hdrs, ok := toStringMap(spec["env_http_headers"]); ok && len(hdrs) > 0 {
			out["env_http_headers"] = hdrs
		}
		if scopes, ok := toStringArray(spec["scopes"]); ok && len(scopes) > 0 {
			out["scopes"] = scopes
		}
	default:
		return nil, fmt.Errorf("unsupported server type for codex: %s", typ)
	}

	for _, k := range []string{"enabled", "required"} {
		if v, ok := spec[k].(bool); ok {
			out[k] = v
		}
	}
	for _, k := range []string{"enabled_tools", "disabled_tools"} {
		if v, ok := toStringArray(spec[k]); ok && len(v) > 0 {
			out[k] = v
		}
	}
	for _, k := range []string{"startup_timeout_ms", "startup_timeout_sec", "tool_timeout_ms", "tool_timeout_sec"} {
		if v, ok := numberToInt64(spec[k]); ok && v > 0 {
			out[k] = v
		}
	}

	return out, nil
}

func ApplyClaudeConfig(path string, reg Registry, removeIDs []string, platform string, force bool) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("path is empty")
	}

	isWSLTarget := isWSLPathForWindows(platform, path)
	cfg, err := ExportClaudeConfig(reg, platform)
	if err != nil {
		return false, err
	}
	serversAny := cfg["mcpServers"]
	if strings.EqualFold(platform, "windows") && isWSLTarget {
		// If target is WSL path, ExportClaudeConfig will have wrapped; undo by re-export without wrapping.
		// Simpler: rebuild servers without wrapping.
		serversAny, err = exportClaudeServersNoWrap(reg)
		if err != nil {
			return false, err
		}
	}
	servers := map[string]any{}
	if ns, ok := serversAny.(map[string]any); ok && len(ns) > 0 {
		servers = ns
	}
	return applyJSONConfigServersKey(path, "mcpServers", servers, removeIDs, force, "invalid claude config json")
}

func exportClaudeServersNoWrap(reg Registry) (map[string]any, error) {
	servers := make(map[string]any, len(reg))
	for id, spec := range reg {
		copied, err := deepCopyObject(spec)
		if err != nil {
			return nil, err
		}
		servers[id] = copied
	}
	return servers, nil
}

func ApplyGeminiConfig(path string, reg Registry, removeIDs []string, force bool) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("path is empty")
	}

	cfg, err := ExportGeminiConfig(reg)
	if err != nil {
		return false, err
	}
	servers := map[string]any{}
	if ns, ok := cfg["mcpServers"].(map[string]any); ok && len(ns) > 0 {
		servers = ns
	}
	return applyJSONConfigServersKey(path, "mcpServers", servers, removeIDs, force, "invalid gemini config json")
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st != nil && !st.IsDir()
}

// isWSLPathForWindows detects WSL UNC paths; non-windows platforms always return false.
func isWSLPathForWindows(platform string, path string) bool {
	if !strings.EqualFold(platform, "windows") {
		return false
	}
	p := filepath.Clean(path)
	pl := strings.ToLower(p)
	// UNC prefix; filepath on Windows uses backslashes.
	if strings.HasPrefix(pl, `\\wsl$\`) {
		return true
	}
	if strings.HasPrefix(pl, `\\wsl.localhost\`) {
		return true
	}
	return false
}
