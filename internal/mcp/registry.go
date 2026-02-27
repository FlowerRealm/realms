package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Registry is a canonical MCP servers mapping: id -> server spec (JSON object).
//
// Canonicalization rules (mirrors cc-switch behavior, with a bit less foot-guns):
// - Accepts UI wrappers like {"server": {...}} and flattens them.
// - Accepts Gemini-style "httpUrl" and converts it to {"url": ..., "type":"http"}.
// - If "type" is missing, infers it from fields:
//   - command => "stdio"
//   - url => "sse"
//
// - Removes UI helper fields (enabled/source/etc.).
type Registry map[string]map[string]any

var errInvalidRegistry = errors.New("invalid mcp registry")

var helperKeysToStrip = map[string]struct{}{
	"enabled":     {},
	"source":      {},
	"id":          {},
	"name":        {},
	"description": {},
	"tags":        {},
	"homepage":    {},
	"docs":        {},
}

func ParseRegistryJSON(raw string) (Registry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Registry{}, nil
	}
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("%w: invalid json: %v", errInvalidRegistry, err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: root must be an object", errInvalidRegistry)
	}

	out := make(Registry, len(obj))
	for id, specAny := range obj {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%w: server id is empty", errInvalidRegistry)
		}
		spec, err := normalizeSpec(id, specAny)
		if err != nil {
			return nil, err
		}
		out[id] = spec
	}
	return out, nil
}

func PrettyJSON(reg Registry) (string, error) {
	if reg == nil {
		reg = Registry{}
	}
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func ExportClaudeConfig(reg Registry, platform string) (map[string]any, error) {
	servers := make(map[string]any, len(reg))
	for id, spec := range reg {
		copied, err := deepCopyObject(spec)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(platform, "windows") {
			wrapCommandForWindows(copied)
		}
		servers[id] = copied
	}
	return map[string]any{"mcpServers": servers}, nil
}

func ExportGeminiConfig(reg Registry) (map[string]any, error) {
	servers := make(map[string]any, len(reg))
	for id, spec := range reg {
		copied, err := deepCopyObject(spec)
		if err != nil {
			return nil, err
		}
		toGeminiServerSpec(copied)
		servers[id] = copied
	}
	return map[string]any{"mcpServers": servers}, nil
}

// ExportCodexConfigTOML exports registry to Codex CLI's `~/.codex/config.toml` format.
//
// Ref: https://developers.openai.com/codex/mcp
func ExportCodexConfigTOML(reg Registry, platform string) (string, error) {
	if reg == nil {
		reg = Registry{}
	}
	ids := make([]string, 0, len(reg))
	for id := range reg {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	for i, id := range ids {
		spec := reg[id]
		copied, err := deepCopyObject(spec)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(platform, "windows") {
			wrapCommandForWindows(copied)
		}
		if err := writeCodexServerTOML(&b, id, copied, i != 0); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func normalizeSpec(id string, specAny any) (map[string]any, error) {
	spec, ok := specAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: server '%s' must be an object", errInvalidRegistry, id)
	}

	// Flatten {"server": {...}} if present (cc-switch UI helper).
	if serverAny, ok := spec["server"]; ok {
		serverObj, ok := serverAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: server '%s' field 'server' must be an object", errInvalidRegistry, id)
		}
		spec = serverObj
	}

	// Strip UI helper keys.
	for k := range helperKeysToStrip {
		delete(spec, k)
	}

	// Gemini-style: httpUrl -> url + type=http
	if httpURL, ok := spec["httpUrl"]; ok {
		delete(spec, "httpUrl")
		spec["url"] = httpURL
		spec["type"] = "http"
	}

	typ := strings.TrimSpace(stringFromAny(spec["type"]))
	if typ == "" {
		// Infer "type" to avoid the worst special-case: url-only servers accidentally treated as stdio.
		if strings.TrimSpace(stringFromAny(spec["command"])) != "" {
			typ = "stdio"
		} else if strings.TrimSpace(stringFromAny(spec["url"])) != "" {
			typ = "sse"
		} else {
			return nil, fmt.Errorf("%w: server '%s' missing 'type' and has neither 'command' nor 'url'", errInvalidRegistry, id)
		}
	}

	switch typ {
	case "stdio", "http", "sse":
	default:
		return nil, fmt.Errorf("%w: server '%s' invalid type: %s", errInvalidRegistry, id, typ)
	}

	// Minimal validation (be permissive about unknown fields).
	switch typ {
	case "stdio":
		if strings.TrimSpace(stringFromAny(spec["command"])) == "" {
			return nil, fmt.Errorf("%w: server '%s' stdio missing 'command'", errInvalidRegistry, id)
		}
	case "http", "sse":
		if strings.TrimSpace(stringFromAny(spec["url"])) == "" {
			return nil, fmt.Errorf("%w: server '%s' %s missing 'url'", errInvalidRegistry, id, typ)
		}
	}

	spec["type"] = typ
	return spec, nil
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func deepCopyObject(in map[string]any) (map[string]any, error) {
	if in == nil {
		return map[string]any{}, nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var windowsWrapCommands = map[string]struct{}{
	"npx":  {},
	"npm":  {},
	"yarn": {},
	"pnpm": {},
	"node": {},
	"bun":  {},
	"deno": {},
}

// wrapCommandForWindows transforms `command: npx` into `command: cmd, args: ["/c","npx",...]`.
// It only applies to stdio servers, and skips when command is already cmd/cmd.exe.
func wrapCommandForWindows(spec map[string]any) {
	typ := strings.TrimSpace(stringFromAny(spec["type"]))
	if typ == "" {
		typ = "stdio"
	}
	if typ != "stdio" {
		return
	}

	cmd := strings.TrimSpace(stringFromAny(spec["command"]))
	if cmd == "" {
		return
	}
	if strings.EqualFold(cmd, "cmd") || strings.EqualFold(cmd, "cmd.exe") {
		return
	}

	base := filepath.Base(cmd)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = base
	}
	if _, ok := windowsWrapCommands[strings.ToLower(stem)]; !ok {
		return
	}

	origArgs := toStringSlice(spec["args"])
	newArgs := make([]any, 0, 2+len(origArgs))
	newArgs = append(newArgs, "/c", cmd)
	for _, a := range origArgs {
		newArgs = append(newArgs, a)
	}
	spec["command"] = "cmd"
	spec["args"] = newArgs
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toGeminiServerSpec(spec map[string]any) {
	// Transport conversion.
	typ := strings.TrimSpace(stringFromAny(spec["type"]))
	if strings.EqualFold(typ, "http") {
		if url, ok := spec["url"]; ok {
			delete(spec, "url")
			spec["httpUrl"] = url
		}
	}
	delete(spec, "type") // Gemini infers transport from field names.

	// Timeout conversion.
	// - Accept existing "timeout" (ms) if present.
	// - Otherwise compute from startup/tool timeouts (sec/ms).
	// IMPORTANT: Do NOT inject default timeout when no timeouts are provided,
	// otherwise scan/apply will drift forever (Gemini adds timeout, others don't).
	if _, ok := numberToUint64(spec["timeout"]); ok {
		// Keep as-is.
		return
	}

	startupMs := extractTimeoutMs(spec, "startup_timeout_sec", 1000)
	if startupMs == 0 {
		startupMs = extractTimeoutMs(spec, "startup_timeout_ms", 1)
	}

	toolMs := extractTimeoutMs(spec, "tool_timeout_sec", 1000)
	if toolMs == 0 {
		toolMs = extractTimeoutMs(spec, "tool_timeout_ms", 1)
	}
	if startupMs == 0 && toolMs == 0 {
		return
	}

	if startupMs > toolMs {
		spec["timeout"] = startupMs
	} else {
		spec["timeout"] = toolMs
	}
}

func extractTimeoutMs(obj map[string]any, key string, multiplier uint64) uint64 {
	v, ok := obj[key]
	if !ok {
		return 0
	}
	delete(obj, key)
	n, ok := numberToUint64(v)
	if !ok {
		return 0
	}
	return n * multiplier
}

func numberToUint64(v any) (uint64, bool) {
	switch t := v.(type) {
	case float64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case float32:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case int:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case int64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case json.Number:
		u, err := t.Int64()
		if err != nil || u < 0 {
			return 0, false
		}
		return uint64(u), true
	default:
		return 0, false
	}
}

var tomlBareKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func tomlKey(k string) string {
	if tomlBareKeyRe.MatchString(k) {
		return k
	}
	return strconv.Quote(k)
}

func tomlString(s string) string {
	// Basic string quoting. Keep it simple and predictable.
	return strconv.Quote(s)
}

func tomlBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func numberToInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case int:
		return int64(t), true
	case int64:
		return t, true
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, false
		}
		if math.Trunc(t) != t {
			return 0, false
		}
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func toStringArray(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		if s, ok := v.([]string); ok {
			return append([]string{}, s...), true
		}
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		s, ok := x.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func tomlStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range arr {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tomlString(s))
	}
	b.WriteString("]")
	return b.String()
}

func toStringMap(v any) (map[string]string, bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		if s, ok := v.(map[string]string); ok {
			out := make(map[string]string, len(s))
			for k, v := range s {
				out[k] = v
			}
			return out, true
		}
		return nil, false
	}
	out := make(map[string]string, len(obj))
	for k, vv := range obj {
		s, ok := vv.(string)
		if !ok {
			return nil, false
		}
		out[k] = s
	}
	return out, true
}

func tomlInlineStringMap(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{ ")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tomlString(k))
		b.WriteString(" = ")
		b.WriteString(tomlString(m[k]))
	}
	b.WriteString(" }")
	return b.String()
}

func writeCodexServerTOML(w *strings.Builder, id string, spec map[string]any, addLeadingBlank bool) error {
	if addLeadingBlank {
		w.WriteString("\n")
	}
	w.WriteString("[mcp_servers.")
	w.WriteString(tomlKey(id))
	w.WriteString("]\n")

	// Allowlist: keep export predictable and aligned with Codex docs.
	// https://developers.openai.com/codex/mcp
	if cmd := strings.TrimSpace(stringFromAny(spec["command"])); cmd != "" {
		w.WriteString("command = ")
		w.WriteString(tomlString(cmd))
		w.WriteString("\n")
	}
	if cwd := strings.TrimSpace(stringFromAny(spec["cwd"])); cwd != "" {
		w.WriteString("cwd = ")
		w.WriteString(tomlString(cwd))
		w.WriteString("\n")
	}
	if url := strings.TrimSpace(stringFromAny(spec["url"])); url != "" {
		w.WriteString("url = ")
		w.WriteString(tomlString(url))
		w.WriteString("\n")
	}
	if v := strings.TrimSpace(stringFromAny(spec["bearer_token_env_var"])); v != "" {
		w.WriteString("bearer_token_env_var = ")
		w.WriteString(tomlString(v))
		w.WriteString("\n")
	}

	if args, ok := toStringArray(spec["args"]); ok {
		w.WriteString("args = ")
		w.WriteString(tomlStringArray(args))
		w.WriteString("\n")
	}

	for _, k := range []string{"startup_timeout_sec", "tool_timeout_sec"} {
		if n, ok := numberToInt64(spec[k]); ok && n > 0 {
			w.WriteString(k)
			w.WriteString(" = ")
			w.WriteString(strconv.FormatInt(n, 10))
			w.WriteString("\n")
		}
	}

	for _, k := range []string{"enabled", "required"} {
		if v, ok := spec[k].(bool); ok {
			w.WriteString(k)
			w.WriteString(" = ")
			w.WriteString(tomlBool(v))
			w.WriteString("\n")
		}
	}

	for _, k := range []string{"enabled_tools", "disabled_tools", "env_vars"} {
		if arr, ok := toStringArray(spec[k]); ok {
			w.WriteString(k)
			w.WriteString(" = ")
			w.WriteString(tomlStringArray(arr))
			w.WriteString("\n")
		}
	}

	if hdrs, ok := toStringMap(spec["http_headers"]); ok {
		w.WriteString("http_headers = ")
		w.WriteString(tomlInlineStringMap(hdrs))
		w.WriteString("\n")
	}
	if hdrs, ok := toStringMap(spec["env_http_headers"]); ok {
		w.WriteString("env_http_headers = ")
		w.WriteString(tomlInlineStringMap(hdrs))
		w.WriteString("\n")
	}

	if env, ok := toStringMap(spec["env"]); ok && len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteString("\n[mcp_servers.")
		w.WriteString(tomlKey(id))
		w.WriteString(".env]\n")
		for _, k := range keys {
			w.WriteString(tomlKey(k))
			w.WriteString(" = ")
			w.WriteString(tomlString(env[k]))
			w.WriteString("\n")
		}
	}

	return nil
}
