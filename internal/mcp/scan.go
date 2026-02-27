package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type ScanResult struct {
	Target      Target   `json:"target"`
	Path        string   `json:"path"`
	Exists      bool     `json:"exists"`
	ServerCount int      `json:"server_count"`
	ParseError  string   `json:"parse_error,omitempty"`
	Servers     Registry `json:"servers,omitempty"`
}

func ScanTarget(t Target, path string) ScanResult {
	path = strings.TrimSpace(path)
	res := ScanResult{Target: t, Path: path}
	if path == "" {
		res.ParseError = "path is empty"
		return res
	}
	st, err := os.Stat(path)
	if err != nil || st == nil || st.IsDir() {
		return res
	}
	res.Exists = true

	switch t {
	case TargetCodex:
		reg, err := scanCodex(path)
		if err != nil {
			res.ParseError = err.Error()
			return res
		}
		res.Servers = reg
		res.ServerCount = len(reg)
		return res
	case TargetClaude:
		reg, err := scanClaude(path)
		if err != nil {
			res.ParseError = err.Error()
			return res
		}
		res.Servers = reg
		res.ServerCount = len(reg)
		return res
	case TargetGemini:
		reg, err := scanGemini(path)
		if err != nil {
			res.ParseError = err.Error()
			return res
		}
		res.Servers = reg
		res.ServerCount = len(reg)
		return res
	default:
		res.ParseError = fmt.Sprintf("unknown target: %s", t)
		return res
	}
}

func scanCodex(path string) (Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Registry{}, nil
	}
	var root map[string]any
	if err := toml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid toml: %w", err)
	}
	serversAny, ok := root["mcp_servers"]
	if !ok || serversAny == nil {
		return Registry{}, nil
	}
	serversObj, ok := serversAny.(map[string]any)
	if !ok {
		return nil, errors.New("mcp_servers is not an object")
	}
	out := make(Registry, len(serversObj))
	for id, specAny := range serversObj {
		spec, ok := specAny.(map[string]any)
		if !ok {
			continue
		}
		// Codex streamable HTTP uses url; canonicalize as type=http.
		if strings.TrimSpace(stringFromAny(spec["url"])) != "" && strings.TrimSpace(stringFromAny(spec["type"])) == "" {
			spec["type"] = "http"
		}
		norm, err := normalizeSpec(id, spec)
		if err != nil {
			return nil, err
		}
		out[id] = norm
	}
	return out, nil
}

func scanClaude(path string) (Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Registry{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	serversAny, ok := root["mcpServers"]
	if !ok || serversAny == nil {
		return Registry{}, nil
	}
	serversObj, ok := serversAny.(map[string]any)
	if !ok {
		return nil, errors.New("mcpServers is not an object")
	}
	out := make(Registry, len(serversObj))
	for id, specAny := range serversObj {
		spec, ok := specAny.(map[string]any)
		if !ok {
			continue
		}
		norm, err := normalizeSpec(id, spec)
		if err != nil {
			return nil, err
		}
		out[id] = norm
	}
	return out, nil
}

func scanGemini(path string) (Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Registry{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	serversAny, ok := root["mcpServers"]
	if !ok || serversAny == nil {
		return Registry{}, nil
	}
	serversObj, ok := serversAny.(map[string]any)
	if !ok {
		return nil, errors.New("mcpServers is not an object")
	}
	out := make(Registry, len(serversObj))
	for id, specAny := range serversObj {
		spec, ok := specAny.(map[string]any)
		if !ok {
			continue
		}
		// Gemini style: httpUrl/url/command without type.
		if _, ok := spec["httpUrl"]; ok {
			spec["type"] = "http"
			spec["url"] = spec["httpUrl"]
			delete(spec, "httpUrl")
		} else if strings.TrimSpace(stringFromAny(spec["url"])) != "" && strings.TrimSpace(stringFromAny(spec["type"])) == "" {
			spec["type"] = "sse"
		} else if strings.TrimSpace(stringFromAny(spec["command"])) != "" && strings.TrimSpace(stringFromAny(spec["type"])) == "" {
			spec["type"] = "stdio"
		}
		norm, err := normalizeSpec(id, spec)
		if err != nil {
			return nil, err
		}
		out[id] = norm
	}
	return out, nil
}
