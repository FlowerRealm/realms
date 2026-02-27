package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// StoreV2 is Realms canonical MCP storage format.
// It is intentionally target-agnostic; Codex/Claude/Gemini are adapters.
type StoreV2 struct {
	Version int                 `json:"version"`
	Servers map[string]ServerV2 `json:"servers"`
}

type ServerV2 struct {
	Transport string      `json:"transport"` // stdio|http|sse
	Stdio     *StdioV2    `json:"stdio,omitempty"`
	HTTP      *HTTPV2     `json:"http,omitempty"`
	Timeouts  *TimeoutsV2 `json:"timeouts,omitempty"`
	// Targets controls per-target enablement for this server.
	// Semantics: nil (or missing key) means enabled=true. Only explicit false disables.
	Targets *ServerTargetsV2 `json:"targets,omitempty"`
}

type ServerTargetsV2 struct {
	Codex  *bool `json:"codex,omitempty"`
	Claude *bool `json:"claude,omitempty"`
	Gemini *bool `json:"gemini,omitempty"`
}

type StdioV2 struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type HTTPV2 struct {
	URL               string            `json:"url"`
	BearerTokenEnvVar string            `json:"bearer_token_env_var,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
}

type TimeoutsV2 struct {
	StartupMS uint64 `json:"startup_ms,omitempty"`
	ToolMS    uint64 `json:"tool_ms,omitempty"`
}

func (s StoreV2) Normalize() StoreV2 {
	out := s
	if out.Version == 0 {
		out.Version = 2
	}
	if out.Servers == nil {
		out.Servers = map[string]ServerV2{}
	}
	// Ensure stable field shapes.
	for id, sv := range out.Servers {
		id2 := strings.TrimSpace(id)
		if id2 == "" {
			delete(out.Servers, id)
			continue
		}
		sv.Transport = strings.TrimSpace(strings.ToLower(sv.Transport))
		if sv.Stdio != nil {
			sv.Stdio.Command = strings.TrimSpace(sv.Stdio.Command)
			sv.Stdio.Cwd = strings.TrimSpace(sv.Stdio.Cwd)
		}
		if sv.HTTP != nil {
			sv.HTTP.URL = strings.TrimSpace(sv.HTTP.URL)
			sv.HTTP.BearerTokenEnvVar = strings.TrimSpace(sv.HTTP.BearerTokenEnvVar)
		}

		// targets: default is enabled=true, so explicit true is redundant.
		if sv.Targets != nil {
			if sv.Targets.Codex != nil && *sv.Targets.Codex {
				sv.Targets.Codex = nil
			}
			if sv.Targets.Claude != nil && *sv.Targets.Claude {
				sv.Targets.Claude = nil
			}
			if sv.Targets.Gemini != nil && *sv.Targets.Gemini {
				sv.Targets.Gemini = nil
			}
			if sv.Targets.Codex == nil && sv.Targets.Claude == nil && sv.Targets.Gemini == nil {
				sv.Targets = nil
			}
		}

		out.Servers[id2] = sv
		if id2 != id {
			delete(out.Servers, id)
		}
	}
	return out
}

func (sv ServerV2) EnabledFor(t Target) bool {
	// Default: enabled everywhere.
	if sv.Targets == nil {
		return true
	}
	switch t {
	case TargetCodex:
		if sv.Targets.Codex == nil {
			return true
		}
		return *sv.Targets.Codex
	case TargetClaude:
		if sv.Targets.Claude == nil {
			return true
		}
		return *sv.Targets.Claude
	case TargetGemini:
		if sv.Targets.Gemini == nil {
			return true
		}
		return *sv.Targets.Gemini
	default:
		// Unknown target: be conservative and treat as enabled.
		return true
	}
}

func (s StoreV2) Validate() error {
	if s.Version != 2 {
		return fmt.Errorf("unsupported store version: %d", s.Version)
	}
	for id, sv := range s.Servers {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("server id is empty")
		}
		switch sv.Transport {
		case "stdio":
			if sv.Stdio == nil || strings.TrimSpace(sv.Stdio.Command) == "" {
				return fmt.Errorf("server '%s' stdio missing command", id)
			}
		case "http", "sse":
			if sv.HTTP == nil || strings.TrimSpace(sv.HTTP.URL) == "" {
				return fmt.Errorf("server '%s' %s missing url", id, sv.Transport)
			}
		default:
			return fmt.Errorf("server '%s' invalid transport: %s", id, sv.Transport)
		}
	}
	return nil
}

func ParseStoreV2JSON(raw string) (StoreV2, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return StoreV2{Version: 2, Servers: map[string]ServerV2{}}, nil
	}
	var s StoreV2
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return StoreV2{}, err
	}
	s = s.Normalize()
	if s.Version == 0 {
		s.Version = 2
	}
	if err := s.Validate(); err != nil {
		return StoreV2{}, err
	}
	return s, nil
}

func PrettyStoreV2JSON(s StoreV2) (string, error) {
	s = s.Normalize()
	if s.Version == 0 {
		s.Version = 2
	}
	if s.Servers == nil {
		s.Servers = map[string]ServerV2{}
	}
	if err := s.Validate(); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// StoreV2FromRegistry converts legacy registry view into canonical store v2.
func StoreV2FromRegistry(reg Registry) StoreV2 {
	out := StoreV2{Version: 2, Servers: map[string]ServerV2{}}
	for id, spec := range reg {
		id = strings.TrimSpace(id)
		if id == "" || spec == nil {
			continue
		}
		typ := strings.TrimSpace(stringFromAny(spec["type"]))
		if typ == "" {
			if strings.TrimSpace(stringFromAny(spec["command"])) != "" {
				typ = "stdio"
			} else if strings.TrimSpace(stringFromAny(spec["url"])) != "" {
				typ = "sse"
			}
		}
		typ = strings.ToLower(typ)

		sv := ServerV2{Transport: typ}
		switch typ {
		case "stdio":
			sv.Stdio = &StdioV2{
				Command: strings.TrimSpace(stringFromAny(spec["command"])),
				Args:    toStringSlice(spec["args"]),
				Cwd:     strings.TrimSpace(stringFromAny(spec["cwd"])),
				Env:     toStringMapString(spec["env"]),
			}
		case "http", "sse":
			sv.HTTP = &HTTPV2{
				URL:               strings.TrimSpace(stringFromAny(spec["url"])),
				BearerTokenEnvVar: strings.TrimSpace(stringFromAny(spec["bearer_token_env_var"])),
				Headers:           toStringMapString(spec["http_headers"]),
			}
		default:
			continue
		}

		t := parseTimeoutsFromLegacySpec(spec)
		if t.StartupMS > 0 || t.ToolMS > 0 {
			sv.Timeouts = &t
		}
		out.Servers[id] = sv
	}
	return out.Normalize()
}

func serverV2ToLegacySpec(sv ServerV2) map[string]any {
	spec := map[string]any{}
	switch sv.Transport {
	case "stdio":
		spec["type"] = "stdio"
		if sv.Stdio != nil {
			spec["command"] = sv.Stdio.Command
			if len(sv.Stdio.Args) > 0 {
				args := make([]any, 0, len(sv.Stdio.Args))
				for _, a := range sv.Stdio.Args {
					args = append(args, a)
				}
				spec["args"] = args
			}
			if strings.TrimSpace(sv.Stdio.Cwd) != "" {
				spec["cwd"] = sv.Stdio.Cwd
			}
			if len(sv.Stdio.Env) > 0 {
				env := map[string]any{}
				for k, v := range sv.Stdio.Env {
					env[k] = v
				}
				spec["env"] = env
			}
		}
	case "http", "sse":
		spec["type"] = sv.Transport
		if sv.HTTP != nil {
			spec["url"] = sv.HTTP.URL
			if strings.TrimSpace(sv.HTTP.BearerTokenEnvVar) != "" {
				spec["bearer_token_env_var"] = sv.HTTP.BearerTokenEnvVar
			}
			if len(sv.HTTP.Headers) > 0 {
				h := map[string]any{}
				for k, v := range sv.HTTP.Headers {
					h[k] = v
				}
				spec["http_headers"] = h
			}
		}
	default:
		return nil
	}

	// Only export timeouts when explicitly set in canonical.
	if sv.Timeouts != nil {
		if sv.Timeouts.StartupMS > 0 {
			spec["startup_timeout_ms"] = sv.Timeouts.StartupMS
		}
		if sv.Timeouts.ToolMS > 0 {
			spec["tool_timeout_ms"] = sv.Timeouts.ToolMS
		}
	}
	return spec
}

// StoreV2ToRegistry converts canonical store v2 to legacy registry view for applying/exporting.
func StoreV2ToRegistry(s StoreV2) Registry {
	s = s.Normalize()
	out := make(Registry, len(s.Servers))
	for id, sv := range s.Servers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		spec := serverV2ToLegacySpec(sv)
		if spec == nil {
			continue
		}
		out[id] = spec
	}
	return out
}

func parseTimeoutsFromLegacySpec(spec map[string]any) TimeoutsV2 {
	var out TimeoutsV2
	if v, ok := numberToUint64Any(spec["startup_timeout_ms"]); ok && v > 0 {
		out.StartupMS = v
	}
	if v, ok := numberToUint64Any(spec["tool_timeout_ms"]); ok && v > 0 {
		out.ToolMS = v
	}
	if v, ok := numberToUint64Any(spec["startup_timeout_sec"]); ok && v > 0 {
		out.StartupMS = v * 1000
	}
	if v, ok := numberToUint64Any(spec["tool_timeout_sec"]); ok && v > 0 {
		out.ToolMS = v * 1000
	}
	// Gemini stores timeout(ms).
	if v, ok := numberToUint64Any(spec["timeout"]); ok && v > 0 {
		if out.ToolMS == 0 {
			out.ToolMS = v
		}
	}
	return out
}

func numberToUint64Any(v any) (uint64, bool) {
	switch t := v.(type) {
	case uint64:
		return t, true
	case uint32:
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

func toStringMapString(v any) map[string]string {
	obj, ok := v.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, vv := range obj {
		vs, ok := vv.(string)
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = vs
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// StoreV2ToRegistryForTarget is like StoreV2ToRegistry, but only includes servers enabled for the given target.
func StoreV2ToRegistryForTarget(s StoreV2, t Target) Registry {
	s = s.Normalize()
	out := make(Registry, len(s.Servers))
	for id, sv := range s.Servers {
		if !sv.EnabledFor(t) {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if spec := serverV2ToLegacySpec(sv); spec != nil {
			out[id] = spec
		}
	}
	return out
}

// DisabledServerIDsForTarget returns server IDs explicitly disabled for the given target.
func DisabledServerIDsForTarget(s StoreV2, t Target) []string {
	s = s.Normalize()
	out := make([]string, 0)
	for id, sv := range s.Servers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if sv.EnabledFor(t) {
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func StableStoreV2ServerIDs(s StoreV2) []string {
	s = s.Normalize()
	ids := make([]string, 0, len(s.Servers))
	for id := range s.Servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
