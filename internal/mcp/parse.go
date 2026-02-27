package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var errInvalidMCPImport = errors.New("invalid mcp import")

// ParseTargetContentToStoreV2 parses user-provided config content into Realms canonical MCP store v2.
//
// Supported sources:
// - "codex": Codex CLI config TOML (expects "mcp_servers" table).
// - "claude": Claude Desktop config JSON (expects {"mcpServers": {...}}).
// - "gemini": Gemini config JSON (expects {"mcpServers": {...}}).
// - "realms": Realms canonical store v2 JSON.
//
// This function is pure parsing/normalization; it does not read/write any files.
func ParseTargetContentToStoreV2(source string, content string) (StoreV2, error) {
	source = strings.TrimSpace(strings.ToLower(source))
	content = strings.TrimSpace(content)
	if source == "" {
		return StoreV2{}, fmt.Errorf("%w: source is empty", errInvalidMCPImport)
	}
	if content == "" {
		return StoreV2{Version: 2, Servers: map[string]ServerV2{}}, nil
	}

	switch source {
	case "realms":
		return ParseStoreV2JSON(content)
	case "claude", "gemini":
		reg, err := parseJSONToRegistry(content)
		if err != nil {
			return StoreV2{}, err
		}
		return StoreV2FromRegistry(reg), nil
	case "codex":
		reg, err := parseCodexTOMLToRegistry(content)
		if err != nil {
			return StoreV2{}, err
		}
		return StoreV2FromRegistry(reg), nil
	default:
		return StoreV2{}, fmt.Errorf("%w: unsupported source: %s", errInvalidMCPImport, source)
	}
}

func parseJSONToRegistry(content string) (Registry, error) {
	var root any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return nil, fmt.Errorf("%w: invalid json: %v", errInvalidMCPImport, err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: root must be an object", errInvalidMCPImport)
	}

	serversAny, hasServers := obj["mcpServers"]
	if !hasServers || serversAny == nil {
		serversAny = obj
	}
	serversObj, ok := serversAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: mcpServers is not an object", errInvalidMCPImport)
	}

	out := make(Registry, len(serversObj))
	for id, specAny := range serversObj {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%w: server id is empty", errInvalidMCPImport)
		}
		spec, err := normalizeSpec(id, specAny)
		if err != nil {
			return nil, err
		}
		out[id] = spec
	}
	return out, nil
}

func parseCodexTOMLToRegistry(content string) (Registry, error) {
	raw := bytes.TrimSpace([]byte(content))
	if len(raw) == 0 {
		return Registry{}, nil
	}
	var root map[string]any
	if err := toml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%w: invalid toml: %v", errInvalidMCPImport, err)
	}
	serversAny, ok := root["mcp_servers"]
	if !ok || serversAny == nil {
		return Registry{}, nil
	}
	serversObj, ok := serversAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: mcp_servers is not an object", errInvalidMCPImport)
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
