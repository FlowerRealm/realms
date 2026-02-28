package personalconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"realms/internal/mcp"
	"realms/internal/skills"
	"realms/internal/store"
)

// RebuildRuntimeFromBundle makes DB config tables exactly match the bundle (including deletions).
// It intentionally does NOT touch runtime-only tables (usage/events/sessions/etc.).
func RebuildRuntimeFromBundle(ctx context.Context, db *sql.DB, dialect store.Dialect, b Bundle) error {
	if db == nil {
		return errors.New("db is nil")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete first (file is authoritative).
	// Order matters to avoid unique constraints and implicit dependencies.
	for _, stmt := range []string{
		`DELETE FROM channel_group_members`,
		`DELETE FROM channel_groups`,
		`DELETE FROM channel_models`,
		`DELETE FROM managed_models`,
		`DELETE FROM openai_compatible_credentials`,
		`DELETE FROM anthropic_credentials`,
		`DELETE FROM upstream_endpoints`,
		`DELETE FROM upstream_channels`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("clear tables: %w", err)
		}
	}

	// Insert channel_groups.
	for _, g := range b.Admin.ChannelGroups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		desc := any(nil)
		if g.Description != nil && strings.TrimSpace(*g.Description) != "" {
			v := strings.TrimSpace(*g.Description)
			if len(v) > 255 {
				v = v[:255]
			}
			desc = v
		}
		pm := g.PriceMultiplier.Truncate(store.PriceMultiplierScale)
		if pm.IsNegative() {
			pm = store.DefaultGroupPriceMultiplier
		}
		status := g.Status
		if status != 0 && status != 1 {
			status = 1
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_groups(name, description, price_multiplier, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, name, desc, pm, status); err != nil {
			return fmt.Errorf("insert channel_groups: %w", err)
		}
	}

	// Resolve group IDs.
	groupIDByName := map[string]int64{}
	{
		rows, err := tx.QueryContext(ctx, `SELECT id, name FROM channel_groups`)
		if err != nil {
			return fmt.Errorf("select channel_groups: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return fmt.Errorf("scan channel_groups: %w", err)
			}
			name = strings.TrimSpace(name)
			if id > 0 && name != "" {
				groupIDByName[name] = id
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate channel_groups: %w", err)
		}
	}

	// Insert upstream_channels.
	for _, ch := range b.Admin.UpstreamChannels {
		typ := strings.TrimSpace(ch.Type)
		name := strings.TrimSpace(ch.Name)
		if typ == "" || name == "" {
			continue
		}
		groups := strings.TrimSpace(ch.Groups)

		status := ch.Status
		if status != 0 && status != 1 {
			status = 1
		}
		promo := 0
		if ch.Promotion {
			promo = 1
		}

		allowServiceTier := 0
		if ch.AllowServiceTier {
			allowServiceTier = 1
		}
		disableStore := 0
		if ch.DisableStore {
			disableStore = 1
		}
		allowSafetyIdentifier := 0
		if ch.AllowSafetyIdentifier {
			allowSafetyIdentifier = 1
		}

		openAIOrg := nullableTrimmedString(ch.OpenAIOrganization)
		testModel := nullableTrimmedString(ch.TestModel)
		tag := nullableTrimmedString(ch.Tag)
		remark := nullableTrimmedString(ch.Remark)
		weight := 0
		if ch.Weight != nil {
			weight = *ch.Weight
		}
		autoBan := 1
		if ch.AutoBan != nil && !*ch.AutoBan {
			autoBan = 0
		}
		setting := nullableTrimmedJSON(ch.Setting)
		paramOverride := nullableTrimmedJSON(ch.ParamOverride)
		headerOverride := nullableTrimmedJSON(ch.HeaderOverride)
		statusCodeMapping := nullableTrimmedJSON(ch.StatusCodeMapping)
		modelSuffixPreserve := nullableTrimmedJSON(ch.ModelSuffixPreserve)
		requestBodyBlacklist := nullableTrimmedJSON(ch.RequestBodyBlacklist)
		requestBodyWhitelist := nullableTrimmedJSON(ch.RequestBodyWhitelist)

		if _, err := tx.ExecContext(ctx, `
INSERT INTO upstream_channels(
  type, name, `+"`groups`"+`, status, priority, promotion,
  allow_service_tier, disable_store, allow_safety_identifier,
  openai_organization, test_model, tag, remark, weight, auto_ban, setting,
  param_override, header_override, status_code_mapping, model_suffix_preserve, request_body_blacklist, request_body_whitelist,
  created_at, updated_at
)
VALUES(?, ?, ?, ?, ?, ?,
       ?, ?, ?,
       ?, ?, ?, ?, ?, ?, ?,
       ?, ?, ?, ?, ?, ?,
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, typ, name, groups, status, ch.Priority, promo,
			allowServiceTier, disableStore, allowSafetyIdentifier,
			openAIOrg, testModel, tag, remark, weight, autoBan, setting,
			paramOverride, headerOverride, statusCodeMapping, modelSuffixPreserve, requestBodyBlacklist, requestBodyWhitelist,
		); err != nil {
			return fmt.Errorf("insert upstream_channels: %w", err)
		}
	}

	// Resolve channel IDs.
	channelIDByKey := map[string]int64{}
	{
		rows, err := tx.QueryContext(ctx, `SELECT id, type, name FROM upstream_channels`)
		if err != nil {
			return fmt.Errorf("select upstream_channels: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var typ, name string
			if err := rows.Scan(&id, &typ, &name); err != nil {
				return fmt.Errorf("scan upstream_channels: %w", err)
			}
			typ = strings.TrimSpace(typ)
			name = strings.TrimSpace(name)
			if id <= 0 || typ == "" || name == "" {
				continue
			}
			channelIDByKey[typ+":"+name] = id
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate upstream_channels: %w", err)
		}
	}

	// Insert upstream_endpoints.
	for _, ep := range b.Admin.UpstreamEndpoints {
		typ := strings.TrimSpace(ep.ChannelType)
		name := strings.TrimSpace(ep.ChannelName)
		baseURL := strings.TrimSpace(ep.BaseURL)
		if typ == "" || name == "" || baseURL == "" {
			continue
		}
		chID, ok := channelIDByKey[typ+":"+name]
		if !ok || chID <= 0 {
			return fmt.Errorf("endpoint references missing channel: %s/%s", typ, name)
		}
		status := ep.Status
		if status != 0 && status != 1 {
			status = 1
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, chID, baseURL, status, ep.Priority); err != nil {
			return fmt.Errorf("insert upstream_endpoints: %w", err)
		}
	}

	// Resolve endpoint IDs.
	endpointIDByChannelID := map[int64]int64{}
	{
		rows, err := tx.QueryContext(ctx, `SELECT id, channel_id FROM upstream_endpoints`)
		if err != nil {
			return fmt.Errorf("select upstream_endpoints: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, chID int64
			if err := rows.Scan(&id, &chID); err != nil {
				return fmt.Errorf("scan upstream_endpoints: %w", err)
			}
			if id > 0 && chID > 0 {
				endpointIDByChannelID[chID] = id
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate upstream_endpoints: %w", err)
		}
	}

	// Insert managed_models.
	for _, m := range b.Admin.ManagedModels {
		publicID := strings.TrimSpace(m.PublicID)
		if publicID == "" {
			continue
		}
		status := m.Status
		if status != 0 && status != 1 {
			status = 1
		}
		groupName := strings.TrimSpace(m.GroupName)
		inUSD := m.InputUSDPer1M.Truncate(store.USDScale)
		outUSD := m.OutputUSDPer1M.Truncate(store.USDScale)
		cacheInUSD := m.CacheInputUSDPer1M.Truncate(store.USDScale)
		cacheOutUSD := m.CacheOutputUSDPer1M.Truncate(store.USDScale)
		if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
			return fmt.Errorf("managed_models[%s] pricing invalid", publicID)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO managed_models(public_id, group_name, upstream_model, owned_by, input_usd_per_1m, output_usd_per_1m, cache_input_usd_per_1m, cache_output_usd_per_1m, status, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, publicID, groupName, nullableTrimmedString(m.UpstreamModel), nullableTrimmedString(m.OwnedBy),
			inUSD, outUSD, cacheInUSD, cacheOutUSD, status); err != nil {
			return fmt.Errorf("insert managed_models: %w", err)
		}
	}

	// Insert channel_models.
	for _, bm := range b.Admin.ChannelModels {
		typ := strings.TrimSpace(bm.ChannelType)
		name := strings.TrimSpace(bm.ChannelName)
		publicID := strings.TrimSpace(bm.PublicID)
		upstreamModel := strings.TrimSpace(bm.UpstreamModel)
		if typ == "" || name == "" || publicID == "" || upstreamModel == "" {
			continue
		}
		chID, ok := channelIDByKey[typ+":"+name]
		if !ok || chID <= 0 {
			return fmt.Errorf("channel_models references missing channel: %s/%s", typ, name)
		}
		status := bm.Status
		if status != 0 && status != 1 {
			status = 1
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_models(channel_id, public_id, upstream_model, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, chID, publicID, upstreamModel, status); err != nil {
			return fmt.Errorf("insert channel_models: %w", err)
		}
	}

	// Insert group members.
	for _, mem := range b.Admin.ChannelGroupMembers {
		parent := strings.TrimSpace(mem.ParentGroup)
		if parent == "" {
			continue
		}
		parentID, ok := groupIDByName[parent]
		if !ok || parentID <= 0 {
			return fmt.Errorf("channel_group_members references missing parent group: %s", parent)
		}
		promo := 0
		if mem.Promotion {
			promo = 1
		}
		switch {
		case mem.MemberGroup != nil && strings.TrimSpace(*mem.MemberGroup) != "":
			child := strings.TrimSpace(*mem.MemberGroup)
			childID, ok := groupIDByName[child]
			if !ok || childID <= 0 {
				return fmt.Errorf("channel_group_members references missing member group: %s", child)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_group_members(parent_group_id, member_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, NULL, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, parentID, childID, mem.Priority, promo); err != nil {
				return fmt.Errorf("insert channel_group_members(group): %w", err)
			}
		case mem.MemberChannelType != nil && mem.MemberChannelName != nil && strings.TrimSpace(*mem.MemberChannelType) != "" && strings.TrimSpace(*mem.MemberChannelName) != "":
			typ := strings.TrimSpace(*mem.MemberChannelType)
			name := strings.TrimSpace(*mem.MemberChannelName)
			chID, ok := channelIDByKey[typ+":"+name]
			if !ok || chID <= 0 {
				return fmt.Errorf("channel_group_members references missing member channel: %s/%s", typ, name)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_group_members(parent_group_id, member_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, NULL, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, parentID, chID, mem.Priority, promo); err != nil {
				return fmt.Errorf("insert channel_group_members(channel): %w", err)
			}
		default:
			// ignore invalid rows
		}
	}

	// Insert credentials (secrets).
	if b.Secrets != nil {
		if err := insertSecrets(ctx, tx, channelIDByKey, endpointIDByChannelID, b.secretsOrEmpty()); err != nil {
			return err
		}
	}

	// Optional MCP registry snapshot (app_settings).
	// Backward-compat: missing/null field means "leave unchanged".
	if b.MCPStoreV2 != nil {
		sv2, err := mcp.ParseStoreV2JSON(string(b.MCPStoreV2))
		if err != nil {
			return err
		}
		if err := applyMCPStoreV2(ctx, tx, dialect, sv2); err != nil {
			return err
		}
	} else if b.MCPServers != nil {
		// Legacy bundles: migrate to v2 and dual-write.
		reg, err := mcp.ParseRegistryJSON(string(b.MCPServers))
		if err != nil {
			return err
		}
		if err := applyMCPStoreV2(ctx, tx, dialect, mcp.StoreV2FromRegistry(reg)); err != nil {
			return err
		}
	}

	// Optional Skills snapshot (app_settings).
	// Backward-compat: missing/null field means "leave unchanged".
	if b.SkillsStoreV1 != nil {
		raw := strings.TrimSpace(string(b.SkillsStoreV1))
		if raw != "" && raw != "null" {
			sv1, err := skills.ParseStoreV1JSON(raw)
			if err != nil {
				return err
			}
			if err := applySkillsStoreV1(ctx, tx, dialect, sv1); err != nil {
				return err
			}
		}
	}
	if b.SkillsTargetEnabledV1 != nil {
		raw := strings.TrimSpace(string(b.SkillsTargetEnabledV1))
		if raw != "" && raw != "null" {
			te, err := skills.ParseTargetEnabledV1JSON(raw)
			if err != nil {
				return err
			}
			if err := applySkillsTargetEnabledV1(ctx, tx, dialect, te); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func applyMCPStoreV2(ctx context.Context, tx *sql.Tx, dialect store.Dialect, sv2 mcp.StoreV2) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	sv2 = sv2.Normalize()
	if err := sv2.Validate(); err != nil {
		return err
	}
	rawV2, err := mcp.PrettyStoreV2JSON(sv2)
	if err != nil {
		return err
	}
	rawLegacy := ""
	reg := mcp.StoreV2ToRegistry(sv2)
	if len(reg) > 0 {
		s, err := mcp.PrettyJSON(reg)
		if err != nil {
			return err
		}
		rawLegacy = s
	}

	stmt := "INSERT INTO app_settings(`key`, value, created_at, updated_at)\n" +
		"VALUES(?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n" +
		"ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=CURRENT_TIMESTAMP"
	if dialect == store.DialectSQLite {
		stmt = "INSERT INTO app_settings(`key`, value, created_at, updated_at)\n" +
			"VALUES(?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n" +
			"ON CONFLICT(`key`) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP"
	}

	if len(sv2.Servers) == 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=?", store.SettingMCPServersStoreV2); err != nil {
			return fmt.Errorf("clear mcp store_v2 app_setting: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=?", store.SettingMCPServersRegistry); err != nil {
			return fmt.Errorf("clear mcp registry app_setting: %w", err)
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, stmt, store.SettingMCPServersStoreV2, rawV2); err != nil {
		return fmt.Errorf("upsert mcp store_v2 app_setting: %w", err)
	}
	if rawLegacy == "" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=?", store.SettingMCPServersRegistry); err != nil {
			return fmt.Errorf("clear mcp registry app_setting: %w", err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, stmt, store.SettingMCPServersRegistry, rawLegacy); err != nil {
		return fmt.Errorf("upsert mcp registry app_setting: %w", err)
	}
	return nil
}

func applySkillsStoreV1(ctx context.Context, tx *sql.Tx, dialect store.Dialect, sv1 skills.StoreV1) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	sv1 = sv1.Normalize()
	if err := sv1.Validate(); err != nil {
		return err
	}
	raw, err := skills.PrettyStoreV1JSON(sv1)
	if err != nil {
		return err
	}

	stmt := "INSERT INTO app_settings(`key`, value, created_at, updated_at)\n" +
		"VALUES(?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n" +
		"ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=CURRENT_TIMESTAMP"
	if dialect == store.DialectSQLite {
		stmt = "INSERT INTO app_settings(`key`, value, created_at, updated_at)\n" +
			"VALUES(?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n" +
			"ON CONFLICT(`key`) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP"
	}

	if len(sv1.Skills) == 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=?", store.SettingSkillsStoreV1); err != nil {
			return fmt.Errorf("clear skills store_v1 app_setting: %w", err)
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, stmt, store.SettingSkillsStoreV1, raw); err != nil {
		return fmt.Errorf("upsert skills store_v1 app_setting: %w", err)
	}
	return nil
}

func applySkillsTargetEnabledV1(ctx context.Context, tx *sql.Tx, dialect store.Dialect, te skills.TargetEnabledV1) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	raw, err := skills.PrettyTargetEnabledV1JSON(te)
	if err != nil {
		return err
	}

	stmt := "INSERT INTO app_settings(`key`, value, created_at, updated_at)\n" +
		"VALUES(?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n" +
		"ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=CURRENT_TIMESTAMP"
	if dialect == store.DialectSQLite {
		stmt = "INSERT INTO app_settings(`key`, value, created_at, updated_at)\n" +
			"VALUES(?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n" +
			"ON CONFLICT(`key`) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP"
	}

	rawTrim := strings.TrimSpace(raw)
	if rawTrim == "" || rawTrim == "{}" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=?", store.SettingSkillsTargetEnabledV1); err != nil {
			return fmt.Errorf("clear skills target_enabled app_setting: %w", err)
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, stmt, store.SettingSkillsTargetEnabledV1, rawTrim); err != nil {
		return fmt.Errorf("upsert skills target_enabled app_setting: %w", err)
	}
	return nil
}

func insertSecrets(ctx context.Context, tx *sql.Tx, channelIDByKey map[string]int64, endpointIDByChannelID map[int64]int64, sec Secrets) error {
	insert := func(table string, endpointID int64, name *string, apiKey string) error {
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return nil
		}
		// NOTE: Despite the `_enc` suffix, this project currently stores API keys as plaintext bytes in
		// `api_key_enc` (app-level encryption was removed; legacy encrypted blobs are rejected elsewhere).
		// Keep rebuild behavior consistent with the rest of the store.
		hint := tokenHint(apiKey)
		n := nullableTrimmedString(name)
		_, err := tx.ExecContext(ctx, `
INSERT INTO `+table+`(endpoint_id, name, api_key_enc, api_key_hint, status, created_at, updated_at)
VALUES(?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, endpointID, n, []byte(apiKey), hint)
		if err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
		return nil
	}

	resolveEndpointID := func(channelType, channelName string) (int64, error) {
		chID, ok := channelIDByKey[strings.TrimSpace(channelType)+":"+strings.TrimSpace(channelName)]
		if !ok || chID <= 0 {
			return 0, fmt.Errorf("secrets references missing channel: %s/%s", channelType, channelName)
		}
		epID, ok := endpointIDByChannelID[chID]
		if !ok || epID <= 0 {
			return 0, fmt.Errorf("secrets references missing endpoint: %s/%s", channelType, channelName)
		}
		return epID, nil
	}

	for _, ep := range sec.OpenAICompatible {
		epID, err := resolveEndpointID(ep.ChannelType, ep.ChannelName)
		if err != nil {
			return err
		}
		for _, c := range ep.Credentials {
			if err := insert("openai_compatible_credentials", epID, c.Name, c.APIKey); err != nil {
				return err
			}
		}
	}
	for _, ep := range sec.Anthropic {
		epID, err := resolveEndpointID(ep.ChannelType, ep.ChannelName)
		if err != nil {
			return err
		}
		for _, c := range ep.Credentials {
			if err := insert("anthropic_credentials", epID, c.Name, c.APIKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func nullableTrimmedString(v *string) any {
	if v == nil {
		return any(nil)
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return any(nil)
	}
	return s
}

func nullableTrimmedJSON(v json.RawMessage) any {
	raw := strings.TrimSpace(string(v))
	if raw == "" || raw == "null" || raw == "{}" || raw == "[]" {
		return any(nil)
	}
	return raw
}

func tokenHint(raw string) *string {
	if raw == "" {
		return nil
	}
	const keep = 6
	if len(raw) <= keep {
		h := raw
		return &h
	}
	h := raw[len(raw)-keep:]
	return &h
}
