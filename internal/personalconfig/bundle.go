package personalconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"realms/internal/mcp"
	"realms/internal/skills"
	"realms/internal/store"
)

const BundleVersion = 1

// Bundle is the authoritative personal-mode configuration snapshot.
//
// IMPORTANT:
// - This file may contain plaintext upstream API keys (Secrets). Treat it like a password file.
// - Bundle is designed for single-node personal mode. Do not enable it for multi-instance deployments.
type Bundle struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`

	Admin store.AdminConfigExport `json:"admin"`

	// MCPServers is an optional canonical MCP servers registry snapshot.
	// When absent (null/missing), it is treated as "unchanged" during personal-config apply
	// for backward compatibility with older bundles.
	MCPServers json.RawMessage `json:"mcp_servers,omitempty"`

	// MCPStoreV2 is Realms canonical MCP store snapshot (v2).
	// When present, it takes precedence over MCPServers.
	MCPStoreV2 json.RawMessage `json:"mcp_store_v2,omitempty"`

	// SkillsStoreV1 is Realms canonical Skills store snapshot (v1).
	// When absent (null/missing), it is treated as "unchanged" during personal-config apply
	// for backward compatibility with older bundles.
	SkillsStoreV1 json.RawMessage `json:"skills_store_v1,omitempty"`

	// SkillsTargetEnabledV1 stores per-target enablement for skills apply (v1).
	// When absent (null/missing), it is treated as "unchanged".
	SkillsTargetEnabledV1 json.RawMessage `json:"skills_target_enabled_v1,omitempty"`

	Secrets *Secrets `json:"secrets,omitempty"`
}

type Secrets struct {
	OpenAICompatible []EndpointSecrets `json:"openai_compatible_credentials,omitempty"`
	Anthropic        []EndpointSecrets `json:"anthropic_credentials,omitempty"`
}

type EndpointSecrets struct {
	ChannelType string `json:"channel_type"`
	ChannelName string `json:"channel_name"`
	BaseURL     string `json:"base_url,omitempty"`

	Credentials []CredentialSecret `json:"credentials,omitempty"`
}

type CredentialSecret struct {
	Name   *string `json:"name,omitempty"`
	APIKey string  `json:"api_key"`
}

func (b Bundle) Validate() error {
	if b.Version != BundleVersion {
		return fmt.Errorf("unsupported bundle version: %d", b.Version)
	}
	if b.Admin.Version < 1 {
		return errors.New("admin export missing version")
	}
	if len(b.MCPStoreV2) > 0 {
		if _, err := mcp.ParseStoreV2JSON(string(b.MCPStoreV2)); err != nil {
			return fmt.Errorf("invalid mcp_store_v2: %w", err)
		}
	}
	if len(b.MCPServers) > 0 {
		if _, err := mcp.ParseRegistryJSON(string(b.MCPServers)); err != nil {
			return fmt.Errorf("invalid mcp_servers: %w", err)
		}
	}
	if err := validateOptionalRawMessage(b.SkillsStoreV1, "skills_store_v1", func(raw string) error {
		_, err := skills.ParseStoreV1JSON(raw)
		return err
	}); err != nil {
		return err
	}
	if err := validateOptionalRawMessage(b.SkillsTargetEnabledV1, "skills_target_enabled_v1", func(raw string) error {
		_, err := skills.ParseTargetEnabledV1JSON(raw)
		return err
	}); err != nil {
		return err
	}
	// Basic sanity checks; deeper validation happens during rebuild.
	for _, ep := range append(append([]EndpointSecrets{}, b.secretsOrEmpty().OpenAICompatible...), b.secretsOrEmpty().Anthropic...) {
		if strings.TrimSpace(ep.ChannelType) == "" || strings.TrimSpace(ep.ChannelName) == "" {
			return errors.New("secrets endpoint missing channel_type/channel_name")
		}
		for _, c := range ep.Credentials {
			if strings.TrimSpace(c.APIKey) == "" {
				return errors.New("secrets credential api_key is empty")
			}
		}
	}
	return nil
}

func validateOptionalRawMessage(raw json.RawMessage, field string, parse func(string) error) error {
	if len(raw) == 0 {
		return nil
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	if err := parse(s); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

func (b Bundle) secretsOrEmpty() Secrets {
	if b.Secrets == nil {
		return Secrets{}
	}
	return *b.Secrets
}

func decodeBundle(data []byte) (Bundle, string, error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return Bundle{}, "", errors.New("bundle is empty")
	}
	var b Bundle
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return Bundle{}, "", fmt.Errorf("invalid bundle json: %w", err)
	}
	if err := b.Validate(); err != nil {
		return Bundle{}, "", err
	}
	sum := sha256.Sum256([]byte(raw))
	return b, hex.EncodeToString(sum[:]), nil
}

func encodeBundle(b Bundle) ([]byte, string, error) {
	if b.Version == 0 {
		b.Version = BundleVersion
	}
	if b.ExportedAt.IsZero() {
		b.ExportedAt = time.Now()
	}
	if err := b.Validate(); err != nil {
		return nil, "", err
	}
	canon, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal bundle: %w", err)
	}
	raw := strings.TrimSpace(string(canon))
	// Always end with a single newline when writing to disk.
	out := []byte(raw + "\n")
	sum := sha256.Sum256([]byte(raw))
	return out, hex.EncodeToString(sum[:]), nil
}
