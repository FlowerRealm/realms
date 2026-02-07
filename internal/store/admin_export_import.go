package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const AdminConfigExportVersion = 6

type AdminConfigExport struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`

	ChannelGroups       []AdminConfigChannelGroup       `json:"channel_groups"`
	ChannelGroupMembers []AdminConfigChannelGroupMember `json:"channel_group_members"`

	UpstreamChannels  []AdminConfigUpstreamChannel  `json:"upstream_channels"`
	UpstreamEndpoints []AdminConfigUpstreamEndpoint `json:"upstream_endpoints"`

	ManagedModels []AdminConfigManagedModel `json:"managed_models"`
	ChannelModels []AdminConfigChannelModel `json:"channel_models"`
}

type AdminConfigChannelGroup struct {
	Name            string          `json:"name"`
	Description     *string         `json:"description,omitempty"`
	PriceMultiplier decimal.Decimal `json:"price_multiplier"`
	MaxAttempts     int             `json:"max_attempts"`
	Status          int             `json:"status"`
}

type AdminConfigChannelGroupMember struct {
	ParentGroup string `json:"parent_group"`

	MemberGroup *string `json:"member_group,omitempty"`

	MemberChannelType *string `json:"member_channel_type,omitempty"`
	MemberChannelName *string `json:"member_channel_name,omitempty"`

	Priority  int  `json:"priority"`
	Promotion bool `json:"promotion"`
}

type AdminConfigUpstreamChannel struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Groups   string `json:"groups,omitempty"`
	Status   int    `json:"status"`
	Priority int    `json:"priority"`

	Promotion bool `json:"promotion"`

	OpenAIOrganization *string         `json:"openai_organization,omitempty"`
	TestModel          *string         `json:"test_model,omitempty"`
	Tag                *string         `json:"tag,omitempty"`
	Remark             *string         `json:"remark,omitempty"`
	Weight             *int            `json:"weight,omitempty"`
	AutoBan            *bool           `json:"auto_ban,omitempty"`
	Setting            json.RawMessage `json:"setting,omitempty"`

	AllowServiceTier      bool            `json:"allow_service_tier,omitempty"`
	DisableStore          bool            `json:"disable_store,omitempty"`
	AllowSafetyIdentifier bool            `json:"allow_safety_identifier,omitempty"`
	ParamOverride         json.RawMessage `json:"param_override,omitempty"`
	HeaderOverride        json.RawMessage `json:"header_override,omitempty"`
	StatusCodeMapping     json.RawMessage `json:"status_code_mapping,omitempty"`
	ModelSuffixPreserve   json.RawMessage `json:"model_suffix_preserve,omitempty"`
	RequestBodyBlacklist  json.RawMessage `json:"request_body_blacklist,omitempty"`
	RequestBodyWhitelist  json.RawMessage `json:"request_body_whitelist,omitempty"`
}

type AdminConfigUpstreamEndpoint struct {
	ChannelType string `json:"channel_type"`
	ChannelName string `json:"channel_name"`

	BaseURL  string `json:"base_url"`
	Status   int    `json:"status"`
	Priority int    `json:"priority"`
}

type AdminConfigManagedModel struct {
	PublicID            string          `json:"public_id"`
	GroupName           string          `json:"group_name,omitempty"`
	UpstreamModel       *string         `json:"upstream_model,omitempty"`
	OwnedBy             *string         `json:"owned_by,omitempty"`
	InputUSDPer1M       decimal.Decimal `json:"input_usd_per_1m"`
	OutputUSDPer1M      decimal.Decimal `json:"output_usd_per_1m"`
	CacheInputUSDPer1M  decimal.Decimal `json:"cache_input_usd_per_1m"`
	CacheOutputUSDPer1M decimal.Decimal `json:"cache_output_usd_per_1m"`
	Status              int             `json:"status"`
}

type AdminConfigChannelModel struct {
	ChannelType string `json:"channel_type"`
	ChannelName string `json:"channel_name"`

	PublicID      string `json:"public_id"`
	UpstreamModel string `json:"upstream_model"`
	Status        int    `json:"status"`
}

type AdminConfigImportReport struct {
	ChannelGroups       int `json:"channel_groups"`
	ChannelGroupMembers int `json:"channel_group_members"`
	UpstreamChannels    int `json:"upstream_channels"`
	UpstreamEndpoints   int `json:"upstream_endpoints"`
	ManagedModels       int `json:"managed_models"`
	ChannelModels       int `json:"channel_models"`
}

func (s *Store) ExportAdminConfig(ctx context.Context) (AdminConfigExport, error) {
	if s == nil || s.db == nil {
		return AdminConfigExport{}, errors.New("数据库未初始化")
	}

	out := AdminConfigExport{
		Version:    AdminConfigExportVersion,
		ExportedAt: time.Now(),
	}

	groups, err := s.ListChannelGroups(ctx)
	if err != nil {
		return AdminConfigExport{}, err
	}
	out.ChannelGroups = make([]AdminConfigChannelGroup, 0, len(groups))
	for _, g := range groups {
		out.ChannelGroups = append(out.ChannelGroups, AdminConfigChannelGroup{
			Name:            strings.TrimSpace(g.Name),
			Description:     g.Description,
			PriceMultiplier: g.PriceMultiplier,
			MaxAttempts:     g.MaxAttempts,
			Status:          g.Status,
		})
	}

	for _, g := range groups {
		ms, err := s.ListChannelGroupMembers(ctx, g.ID)
		if err != nil {
			return AdminConfigExport{}, err
		}
		for _, m := range ms {
			row := AdminConfigChannelGroupMember{
				ParentGroup: strings.TrimSpace(g.Name),
				Priority:    m.Priority,
				Promotion:   m.Promotion,
			}
			if m.MemberGroupName != nil && strings.TrimSpace(*m.MemberGroupName) != "" {
				v := strings.TrimSpace(*m.MemberGroupName)
				row.MemberGroup = &v
			}
			if m.MemberChannelType != nil && strings.TrimSpace(*m.MemberChannelType) != "" {
				v := strings.TrimSpace(*m.MemberChannelType)
				row.MemberChannelType = &v
			}
			if m.MemberChannelName != nil && strings.TrimSpace(*m.MemberChannelName) != "" {
				v := strings.TrimSpace(*m.MemberChannelName)
				row.MemberChannelName = &v
			}
			// 忽略脏数据：必须且只能存在一种成员类型。
			if row.MemberGroup != nil && row.MemberChannelName != nil {
				continue
			}
			if row.MemberGroup == nil && row.MemberChannelName == nil {
				continue
			}
			out.ChannelGroupMembers = append(out.ChannelGroupMembers, row)
		}
	}

	channels, err := s.ListUpstreamChannels(ctx)
	if err != nil {
		return AdminConfigExport{}, err
	}
	out.UpstreamChannels = make([]AdminConfigUpstreamChannel, 0, len(channels))
	out.UpstreamEndpoints = make([]AdminConfigUpstreamEndpoint, 0, len(channels))
	out.ChannelModels = make([]AdminConfigChannelModel, 0, len(channels))
	for _, ch := range channels {
		var weight *int
		if ch.Weight != 0 {
			v := ch.Weight
			weight = &v
		}
		var autoBan *bool
		if !ch.AutoBan {
			v := ch.AutoBan
			autoBan = &v
		}

		var setting json.RawMessage
		if b, err := json.Marshal(ch.Setting); err == nil {
			raw := strings.TrimSpace(string(b))
			if raw != "" && raw != "{}" {
				setting = json.RawMessage(raw)
			}
		}

		var paramOverride json.RawMessage
		if strings.TrimSpace(ch.ParamOverride) != "" {
			paramOverride = json.RawMessage(strings.TrimSpace(ch.ParamOverride))
		}
		var headerOverride json.RawMessage
		if strings.TrimSpace(ch.HeaderOverride) != "" {
			headerOverride = json.RawMessage(strings.TrimSpace(ch.HeaderOverride))
		}
		var statusCodeMapping json.RawMessage
		if strings.TrimSpace(ch.StatusCodeMapping) != "" {
			statusCodeMapping = json.RawMessage(strings.TrimSpace(ch.StatusCodeMapping))
		}
		var modelSuffixPreserve json.RawMessage
		if strings.TrimSpace(ch.ModelSuffixPreserve) != "" {
			modelSuffixPreserve = json.RawMessage(strings.TrimSpace(ch.ModelSuffixPreserve))
		}
		var requestBodyBlacklist json.RawMessage
		if strings.TrimSpace(ch.RequestBodyBlacklist) != "" {
			requestBodyBlacklist = json.RawMessage(strings.TrimSpace(ch.RequestBodyBlacklist))
		}
		var requestBodyWhitelist json.RawMessage
		if strings.TrimSpace(ch.RequestBodyWhitelist) != "" {
			requestBodyWhitelist = json.RawMessage(strings.TrimSpace(ch.RequestBodyWhitelist))
		}
		out.UpstreamChannels = append(out.UpstreamChannels, AdminConfigUpstreamChannel{
			Type:                  strings.TrimSpace(ch.Type),
			Name:                  strings.TrimSpace(ch.Name),
			Groups:                strings.TrimSpace(ch.Groups),
			Status:                ch.Status,
			Priority:              ch.Priority,
			Promotion:             ch.Promotion,
			OpenAIOrganization:    ch.OpenAIOrganization,
			TestModel:             ch.TestModel,
			Tag:                   ch.Tag,
			Remark:                ch.Remark,
			Weight:                weight,
			AutoBan:               autoBan,
			Setting:               setting,
			AllowServiceTier:      ch.AllowServiceTier,
			DisableStore:          ch.DisableStore,
			AllowSafetyIdentifier: ch.AllowSafetyIdentifier,
			ParamOverride:         paramOverride,
			HeaderOverride:        headerOverride,
			StatusCodeMapping:     statusCodeMapping,
			ModelSuffixPreserve:   modelSuffixPreserve,
			RequestBodyBlacklist:  requestBodyBlacklist,
			RequestBodyWhitelist:  requestBodyWhitelist,
		})

		ep, err := s.GetUpstreamEndpointByChannelID(ctx, ch.ID)
		if err == nil && strings.TrimSpace(ep.BaseURL) != "" {
			out.UpstreamEndpoints = append(out.UpstreamEndpoints, AdminConfigUpstreamEndpoint{
				ChannelType: strings.TrimSpace(ch.Type),
				ChannelName: strings.TrimSpace(ch.Name),
				BaseURL:     strings.TrimSpace(ep.BaseURL),
				Status:      ep.Status,
				Priority:    ep.Priority,
			})
		}

		models, err := s.ListChannelModelsByChannelID(ctx, ch.ID)
		if err != nil {
			return AdminConfigExport{}, err
		}
		for _, m := range models {
			out.ChannelModels = append(out.ChannelModels, AdminConfigChannelModel{
				ChannelType:   strings.TrimSpace(ch.Type),
				ChannelName:   strings.TrimSpace(ch.Name),
				PublicID:      strings.TrimSpace(m.PublicID),
				UpstreamModel: strings.TrimSpace(m.UpstreamModel),
				Status:        m.Status,
			})
		}
	}

	managed, err := s.ListManagedModels(ctx)
	if err != nil {
		return AdminConfigExport{}, err
	}
	out.ManagedModels = make([]AdminConfigManagedModel, 0, len(managed))
	for _, m := range managed {
		out.ManagedModels = append(out.ManagedModels, AdminConfigManagedModel{
			PublicID:            strings.TrimSpace(m.PublicID),
			GroupName:           normalizeManagedModelGroupName(m.GroupName),
			UpstreamModel:       m.UpstreamModel,
			OwnedBy:             m.OwnedBy,
			InputUSDPer1M:       m.InputUSDPer1M,
			OutputUSDPer1M:      m.OutputUSDPer1M,
			CacheInputUSDPer1M:  m.CacheInputUSDPer1M,
			CacheOutputUSDPer1M: m.CacheOutputUSDPer1M,
			Status:              m.Status,
		})
	}

	return out, nil
}

func (s *Store) ImportAdminConfig(ctx context.Context, in AdminConfigExport) (AdminConfigImportReport, error) {
	if s == nil || s.db == nil {
		return AdminConfigImportReport{}, errors.New("数据库未初始化")
	}
	if in.Version < 1 || in.Version > AdminConfigExportVersion {
		return AdminConfigImportReport{}, fmt.Errorf("不支持的导入版本: %d", in.Version)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AdminConfigImportReport{}, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	groupNames := make(map[string]struct{})
	groupNames[DefaultGroupName] = struct{}{}
	for _, g := range in.ChannelGroups {
		n := strings.TrimSpace(g.Name)
		if n == "" {
			continue
		}
		groupNames[n] = struct{}{}
	}
	for _, m := range in.ChannelGroupMembers {
		p := strings.TrimSpace(m.ParentGroup)
		if p != "" {
			groupNames[p] = struct{}{}
		}
		if m.MemberGroup != nil {
			n := strings.TrimSpace(*m.MemberGroup)
			if n != "" {
				groupNames[n] = struct{}{}
			}
		}
	}

	stmtUpsertGroup := `
INSERT INTO channel_groups(name, description, price_multiplier, max_attempts, status, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  description=VALUES(description),
  price_multiplier=VALUES(price_multiplier),
  max_attempts=VALUES(max_attempts),
  status=VALUES(status),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmtUpsertGroup = `
INSERT INTO channel_groups(name, description, price_multiplier, max_attempts, status, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(name) DO UPDATE SET
  description=excluded.description,
  price_multiplier=excluded.price_multiplier,
  max_attempts=excluded.max_attempts,
  status=excluded.status,
  updated_at=CURRENT_TIMESTAMP
`
	}

	for _, g := range in.ChannelGroups {
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
		pm := g.PriceMultiplier.Truncate(PriceMultiplierScale)
		if pm.IsNegative() {
			pm = DefaultGroupPriceMultiplier
		}
		maxAttempts := g.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 5
		}
		status := g.Status
		if status != 0 && status != 1 {
			status = 1
		}

		if _, err := tx.ExecContext(ctx, stmtUpsertGroup, name, desc, pm, maxAttempts, status); err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("导入 channel_groups 失败: %w", err)
		}
	}

	groupIDByName := make(map[string]int64)
	if len(groupNames) > 0 {
		names := make([]string, 0, len(groupNames))
		for n := range groupNames {
			names = append(names, n)
		}
		placeholders := strings.Repeat("?,", len(names))
		placeholders = strings.TrimSuffix(placeholders, ",")
		args := make([]any, 0, len(names))
		for _, n := range names {
			args = append(args, n)
		}
		rows, err := tx.QueryContext(ctx, "SELECT id, name FROM channel_groups WHERE name IN ("+placeholders+")", args...)
		if err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("查询 channel_groups 失败: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return AdminConfigImportReport{}, fmt.Errorf("扫描 channel_groups 失败: %w", err)
			}
			name = strings.TrimSpace(name)
			if id > 0 && name != "" {
				groupIDByName[name] = id
			}
		}
		if err := rows.Err(); err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("遍历 channel_groups 失败: %w", err)
		}
	}

	ensureGroupID := func(name string) (int64, error) {
		name = strings.TrimSpace(name)
		if name == "" {
			return 0, errors.New("group name 不能为空")
		}
		id, ok := groupIDByName[name]
		if !ok || id == 0 {
			return 0, fmt.Errorf("分组不存在: %s", name)
		}
		return id, nil
	}

	channelIDByKey := make(map[string]int64)

	channelKey := func(typ, name string) string {
		typ = strings.TrimSpace(typ)
		return typ + ":" + strings.TrimSpace(name)
	}

	findChannelID := func(typ, name string) (int64, error) {
		typ = strings.TrimSpace(typ)
		name = strings.TrimSpace(name)
		switch typ {
		case "":
			return 0, errors.New("channel_type 不能为空")
		default:
			var ids []int64
			rows, err := tx.QueryContext(ctx, `SELECT id FROM upstream_channels WHERE type=? AND name=?`, typ, name)
			if err != nil {
				return 0, fmt.Errorf("查询 upstream_channels 失败: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return 0, fmt.Errorf("扫描 upstream_channels 失败: %w", err)
				}
				ids = append(ids, id)
				if len(ids) > 1 {
					break
				}
			}
			if err := rows.Err(); err != nil {
				return 0, fmt.Errorf("遍历 upstream_channels 失败: %w", err)
			}
			if len(ids) == 0 {
				return 0, sql.ErrNoRows
			}
			if len(ids) > 1 {
				return 0, fmt.Errorf("存在多个同名 channel（type=%s, name=%s），请先清理重复记录", typ, name)
			}
			return ids[0], nil
		}
	}

	upsertChannel := func(ch AdminConfigUpstreamChannel) (int64, error) {
		typ := strings.TrimSpace(ch.Type)
		name := strings.TrimSpace(ch.Name)
		if typ == "" {
			return 0, errors.New("type 不能为空")
		}
		if name == "" {
			return 0, errors.New("name 不能为空")
		}

		p := 0
		if ch.Promotion {
			p = 1
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
		paramOverride := strings.TrimSpace(string(ch.ParamOverride))
		headerOverride := strings.TrimSpace(string(ch.HeaderOverride))
		statusCodeMapping := strings.TrimSpace(string(ch.StatusCodeMapping))
		modelSuffixPreserve := strings.TrimSpace(string(ch.ModelSuffixPreserve))
		requestBodyBlacklist := strings.TrimSpace(string(ch.RequestBodyBlacklist))
		requestBodyWhitelist := strings.TrimSpace(string(ch.RequestBodyWhitelist))
		openAIOrganization := ""
		if ch.OpenAIOrganization != nil {
			openAIOrganization = strings.TrimSpace(*ch.OpenAIOrganization)
		}
		testModel := ""
		if ch.TestModel != nil {
			testModel = strings.TrimSpace(*ch.TestModel)
		}
		tag := ""
		if ch.Tag != nil {
			tag = strings.TrimSpace(*ch.Tag)
		}
		remark := ""
		if ch.Remark != nil {
			remark = strings.TrimSpace(*ch.Remark)
		}
		weight := 0
		if ch.Weight != nil {
			weight = *ch.Weight
		}
		autoBan := true
		if ch.AutoBan != nil {
			autoBan = *ch.AutoBan
		}
		autoBanInt := 0
		if autoBan {
			autoBanInt = 1
		}
		setting := strings.TrimSpace(string(ch.Setting))
		if setting != "" {
			var parsed UpstreamChannelSetting
			if err := json.Unmarshal([]byte(setting), &parsed); err != nil {
				return 0, fmt.Errorf("setting 不是有效 JSON: %w", err)
			}
		}
		if in.Version >= 5 {
			normalized, err := normalizeStringJSONArray(modelSuffixPreserve)
			if err != nil {
				return 0, fmt.Errorf("model_suffix_preserve 不是有效 JSON 数组: %w", err)
			}
			modelSuffixPreserve = normalized

			normalized, err = normalizeStringJSONArray(requestBodyBlacklist)
			if err != nil {
				return 0, fmt.Errorf("request_body_blacklist 不是有效 JSON 数组: %w", err)
			}
			requestBodyBlacklist = normalized

			normalized, err = normalizeStringJSONArray(requestBodyWhitelist)
			if err != nil {
				return 0, fmt.Errorf("request_body_whitelist 不是有效 JSON 数组: %w", err)
			}
			requestBodyWhitelist = normalized
		}

		id, err := findChannelID(typ, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				stmtInsert := `
INSERT INTO upstream_channels(type, name, ` + "`groups`" + `, status, priority, promotion, created_at, updated_at)
VALUES(?, ?, '', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`
				args := []any{typ, name, ch.Status, ch.Priority, p}
				if in.Version >= 4 {
					stmtInsert = `
INSERT INTO upstream_channels(type, name, ` + "`groups`" + `, status, priority, promotion, allow_service_tier, disable_store, allow_safety_identifier, param_override, header_override, status_code_mapping, created_at, updated_at)
VALUES(?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`
					args = append(args, allowServiceTier, disableStore, allowSafetyIdentifier, nullableString(&paramOverride), nullableString(&headerOverride), nullableString(&statusCodeMapping))
					if in.Version >= 5 {
						stmtInsert = `
INSERT INTO upstream_channels(type, name, ` + "`groups`" + `, status, priority, promotion, allow_service_tier, disable_store, allow_safety_identifier, param_override, header_override, status_code_mapping, model_suffix_preserve, request_body_blacklist, request_body_whitelist, created_at, updated_at)
VALUES(?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`
						args = append(args, nullableString(&modelSuffixPreserve), nullableString(&requestBodyBlacklist), nullableString(&requestBodyWhitelist))
						if in.Version >= 6 {
							stmtInsert = `
INSERT INTO upstream_channels(type, name, ` + "`groups`" + `, status, priority, promotion, openai_organization, test_model, tag, remark, weight, auto_ban, setting, allow_service_tier, disable_store, allow_safety_identifier, param_override, header_override, status_code_mapping, model_suffix_preserve, request_body_blacklist, request_body_whitelist, created_at, updated_at)
VALUES(?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`
							args = []any{typ, name, ch.Status, ch.Priority, p,
								nullableString(&openAIOrganization), nullableString(&testModel), nullableString(&tag), nullableString(&remark), weight, autoBanInt, nullableString(&setting),
								allowServiceTier, disableStore, allowSafetyIdentifier,
								nullableString(&paramOverride), nullableString(&headerOverride), nullableString(&statusCodeMapping),
								nullableString(&modelSuffixPreserve), nullableString(&requestBodyBlacklist), nullableString(&requestBodyWhitelist),
							}
						}
					}
				} else if in.Version >= 3 {
					stmtInsert = `
INSERT INTO upstream_channels(type, name, ` + "`groups`" + `, status, priority, promotion, allow_service_tier, disable_store, allow_safety_identifier, param_override, created_at, updated_at)
VALUES(?, ?, '', ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`
					args = append(args, allowServiceTier, disableStore, allowSafetyIdentifier, nullableString(&paramOverride))
				} else if in.Version >= 2 {
					stmtInsert = `
INSERT INTO upstream_channels(type, name, ` + "`groups`" + `, status, priority, promotion, allow_service_tier, disable_store, allow_safety_identifier, created_at, updated_at)
VALUES(?, ?, '', ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`
					args = append(args, allowServiceTier, disableStore, allowSafetyIdentifier)
				}
				res, err := tx.ExecContext(ctx, stmtInsert, args...)
				if err != nil {
					return 0, fmt.Errorf("创建 upstream_channel 失败: %w", err)
				}
				id, err = res.LastInsertId()
				if err != nil {
					return 0, fmt.Errorf("获取 upstream_channel id 失败: %w", err)
				}
			} else {
				return 0, err
			}
		} else {
			stmtUpdate := `
UPDATE upstream_channels
SET name=COALESCE(NULLIF(?, ''), name),
    status=?, priority=?, promotion=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`
			args := []any{name, ch.Status, ch.Priority, p, id}
			if in.Version >= 4 {
				stmtUpdate = `
UPDATE upstream_channels
SET name=COALESCE(NULLIF(?, ''), name),
    status=?, priority=?, promotion=?,
    allow_service_tier=?, disable_store=?, allow_safety_identifier=?,
    param_override=?,
    header_override=?,
    status_code_mapping=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`
				args = []any{name, ch.Status, ch.Priority, p, allowServiceTier, disableStore, allowSafetyIdentifier, nullableString(&paramOverride), nullableString(&headerOverride), nullableString(&statusCodeMapping), id}
				if in.Version >= 5 {
					stmtUpdate = `
UPDATE upstream_channels
SET name=COALESCE(NULLIF(?, ''), name),
    status=?, priority=?, promotion=?,
    allow_service_tier=?, disable_store=?, allow_safety_identifier=?,
    param_override=?,
    header_override=?,
    status_code_mapping=?,
    model_suffix_preserve=?,
    request_body_blacklist=?,
    request_body_whitelist=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`
					args = []any{name, ch.Status, ch.Priority, p, allowServiceTier, disableStore, allowSafetyIdentifier, nullableString(&paramOverride), nullableString(&headerOverride), nullableString(&statusCodeMapping), nullableString(&modelSuffixPreserve), nullableString(&requestBodyBlacklist), nullableString(&requestBodyWhitelist), id}
					if in.Version >= 6 {
						stmtUpdate = `
UPDATE upstream_channels
SET name=COALESCE(NULLIF(?, ''), name),
    status=?, priority=?, promotion=?,
    openai_organization=?,
    test_model=?,
    tag=?,
    remark=?,
    weight=?,
    auto_ban=?,
    setting=?,
    allow_service_tier=?, disable_store=?, allow_safety_identifier=?,
    param_override=?,
    header_override=?,
    status_code_mapping=?,
    model_suffix_preserve=?,
    request_body_blacklist=?,
    request_body_whitelist=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`
						args = []any{name, ch.Status, ch.Priority, p,
							nullableString(&openAIOrganization), nullableString(&testModel), nullableString(&tag), nullableString(&remark), weight, autoBanInt, nullableString(&setting),
							allowServiceTier, disableStore, allowSafetyIdentifier,
							nullableString(&paramOverride), nullableString(&headerOverride), nullableString(&statusCodeMapping),
							nullableString(&modelSuffixPreserve), nullableString(&requestBodyBlacklist), nullableString(&requestBodyWhitelist),
							id,
						}
					}
				}
			} else if in.Version >= 3 {
				stmtUpdate = `
UPDATE upstream_channels
SET name=COALESCE(NULLIF(?, ''), name),
    status=?, priority=?, promotion=?,
    allow_service_tier=?, disable_store=?, allow_safety_identifier=?,
    param_override=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`
				args = []any{name, ch.Status, ch.Priority, p, allowServiceTier, disableStore, allowSafetyIdentifier, nullableString(&paramOverride), id}
			} else if in.Version >= 2 {
				stmtUpdate = `
UPDATE upstream_channels
SET name=COALESCE(NULLIF(?, ''), name),
    status=?, priority=?, promotion=?,
    allow_service_tier=?, disable_store=?, allow_safety_identifier=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`
				args = []any{name, ch.Status, ch.Priority, p, allowServiceTier, disableStore, allowSafetyIdentifier, id}
			}
			if _, err := tx.ExecContext(ctx, stmtUpdate, args...); err != nil {
				return 0, fmt.Errorf("更新 upstream_channel 失败: %w", err)
			}
		}

		key := channelKey(typ, name)
		channelIDByKey[key] = id
		return id, nil
	}

	for _, ch := range in.UpstreamChannels {
		if _, err := upsertChannel(ch); err != nil {
			return AdminConfigImportReport{}, err
		}
	}

	resolveChannelID := func(typ, name string) (int64, error) {
		key := channelKey(typ, name)
		if id, ok := channelIDByKey[key]; ok && id > 0 {
			return id, nil
		}
		// 兼容：channel 未出现在 upstream_channels 列表，但被成员关系/绑定引用。
		id, err := findChannelID(typ, name)
		if err != nil {
			return 0, err
		}
		channelIDByKey[key] = id
		return id, nil
	}

	stmtUpsertEndpoint := `
INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  base_url=VALUES(base_url),
  status=VALUES(status),
  priority=VALUES(priority),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmtUpsertEndpoint = `
INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(channel_id) DO UPDATE SET
  base_url=excluded.base_url,
  status=excluded.status,
  priority=excluded.priority,
  updated_at=CURRENT_TIMESTAMP
`
	}

	for _, ep := range in.UpstreamEndpoints {
		typ := strings.TrimSpace(ep.ChannelType)
		name := strings.TrimSpace(ep.ChannelName)
		channelID, err := resolveChannelID(typ, name)
		if err != nil {
			return AdminConfigImportReport{}, err
		}
		baseURL := strings.TrimSpace(ep.BaseURL)
		if baseURL == "" {
			continue
		}
		p := ep.Priority
		if _, err := tx.ExecContext(ctx, stmtUpsertEndpoint, channelID, baseURL, ep.Status, p); err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("导入 upstream_endpoints 失败: %w", err)
		}
	}

	stmtUpsertManagedModel := `
INSERT INTO managed_models(public_id, group_name, upstream_model, owned_by, input_usd_per_1m, output_usd_per_1m, cache_input_usd_per_1m, cache_output_usd_per_1m, status, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  group_name=VALUES(group_name),
  upstream_model=VALUES(upstream_model),
  owned_by=VALUES(owned_by),
  input_usd_per_1m=VALUES(input_usd_per_1m),
  output_usd_per_1m=VALUES(output_usd_per_1m),
  cache_input_usd_per_1m=VALUES(cache_input_usd_per_1m),
  cache_output_usd_per_1m=VALUES(cache_output_usd_per_1m),
  status=VALUES(status)
`
	if s.dialect == DialectSQLite {
		stmtUpsertManagedModel = `
INSERT INTO managed_models(public_id, group_name, upstream_model, owned_by, input_usd_per_1m, output_usd_per_1m, cache_input_usd_per_1m, cache_output_usd_per_1m, status, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(public_id) DO UPDATE SET
  group_name=excluded.group_name,
  upstream_model=excluded.upstream_model,
  owned_by=excluded.owned_by,
  input_usd_per_1m=excluded.input_usd_per_1m,
  output_usd_per_1m=excluded.output_usd_per_1m,
  cache_input_usd_per_1m=excluded.cache_input_usd_per_1m,
  cache_output_usd_per_1m=excluded.cache_output_usd_per_1m,
  status=excluded.status
`
	}

	for _, m := range in.ManagedModels {
		publicID := strings.TrimSpace(m.PublicID)
		if publicID == "" {
			continue
		}
		inUSD := m.InputUSDPer1M.Truncate(USDScale)
		outUSD := m.OutputUSDPer1M.Truncate(USDScale)
		cacheInUSD := m.CacheInputUSDPer1M.Truncate(USDScale)
		cacheOutUSD := m.CacheOutputUSDPer1M.Truncate(USDScale)
		if inUSD.IsNegative() || outUSD.IsNegative() || cacheInUSD.IsNegative() || cacheOutUSD.IsNegative() {
			return AdminConfigImportReport{}, fmt.Errorf("managed_models[%s] 定价不合法", publicID)
		}
		status := m.Status
		if status != 0 && status != 1 {
			status = 1
		}
		groupName := normalizeManagedModelGroupName(m.GroupName)
		if _, err := tx.ExecContext(ctx, stmtUpsertManagedModel, publicID, groupName, m.UpstreamModel, m.OwnedBy, inUSD, outUSD, cacheInUSD, cacheOutUSD, status); err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("导入 managed_models 失败: %w", err)
		}
	}

	stmtUpsertChannelModel := `
INSERT INTO channel_models(channel_id, public_id, upstream_model, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  upstream_model=VALUES(upstream_model),
  status=VALUES(status),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmtUpsertChannelModel = `
INSERT INTO channel_models(channel_id, public_id, upstream_model, status, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(channel_id, public_id) DO UPDATE SET
  upstream_model=excluded.upstream_model,
  status=excluded.status,
  updated_at=CURRENT_TIMESTAMP
`
	}

	for _, m := range in.ChannelModels {
		typ := strings.TrimSpace(m.ChannelType)
		name := strings.TrimSpace(m.ChannelName)
		channelID, err := resolveChannelID(typ, name)
		if err != nil {
			return AdminConfigImportReport{}, err
		}
		publicID := strings.TrimSpace(m.PublicID)
		upstreamModel := strings.TrimSpace(m.UpstreamModel)
		if publicID == "" || upstreamModel == "" {
			continue
		}
		status := m.Status
		if status != 0 && status != 1 {
			status = 1
		}
		if _, err := tx.ExecContext(ctx, stmtUpsertChannelModel, channelID, publicID, upstreamModel, status); err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("导入 channel_models 失败: %w", err)
		}
	}

	stmtUpsertMemberGroup := `
INSERT INTO channel_group_members(parent_group_id, member_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, NULL, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  parent_group_id=VALUES(parent_group_id),
  priority=VALUES(priority),
  promotion=VALUES(promotion),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmtUpsertMemberGroup = `
INSERT INTO channel_group_members(parent_group_id, member_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, NULL, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(member_group_id) DO UPDATE SET
  parent_group_id=excluded.parent_group_id,
  priority=excluded.priority,
  promotion=excluded.promotion,
  updated_at=CURRENT_TIMESTAMP
`
	}

	stmtUpsertMemberChannel := `
INSERT INTO channel_group_members(parent_group_id, member_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, NULL, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  priority=VALUES(priority),
  promotion=VALUES(promotion),
  updated_at=CURRENT_TIMESTAMP
`
	if s.dialect == DialectSQLite {
		stmtUpsertMemberChannel = `
INSERT INTO channel_group_members(parent_group_id, member_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, NULL, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(parent_group_id, member_channel_id) DO UPDATE SET
  priority=excluded.priority,
  promotion=excluded.promotion,
  updated_at=CURRENT_TIMESTAMP
`
	}

	for _, m := range in.ChannelGroupMembers {
		parentName := strings.TrimSpace(m.ParentGroup)
		if parentName == "" {
			continue
		}
		parentID, err := ensureGroupID(parentName)
		if err != nil {
			return AdminConfigImportReport{}, err
		}

		p := 0
		if m.Promotion {
			p = 1
		}

		switch {
		case m.MemberGroup != nil && strings.TrimSpace(*m.MemberGroup) != "":
			childName := strings.TrimSpace(*m.MemberGroup)
			childID, err := ensureGroupID(childName)
			if err != nil {
				return AdminConfigImportReport{}, err
			}
			if _, err := tx.ExecContext(ctx, stmtUpsertMemberGroup, parentID, childID, m.Priority, p); err != nil {
				return AdminConfigImportReport{}, fmt.Errorf("导入 channel_group_members(group) 失败: %w", err)
			}
		case m.MemberChannelType != nil && m.MemberChannelName != nil && strings.TrimSpace(*m.MemberChannelType) != "" && strings.TrimSpace(*m.MemberChannelName) != "":
			chType := strings.TrimSpace(*m.MemberChannelType)
			chName := strings.TrimSpace(*m.MemberChannelName)
			channelID, err := resolveChannelID(chType, chName)
			if err != nil {
				return AdminConfigImportReport{}, err
			}
			if _, err := tx.ExecContext(ctx, stmtUpsertMemberChannel, parentID, channelID, m.Priority, p); err != nil {
				return AdminConfigImportReport{}, fmt.Errorf("导入 channel_group_members(channel) 失败: %w", err)
			}
		default:
			continue
		}
	}

	// 以成员关系为 SSOT，回填 upstream_channels.groups 兼容缓存。
	rows, err := tx.QueryContext(ctx, `SELECT id FROM upstream_channels`)
	if err != nil {
		return AdminConfigImportReport{}, fmt.Errorf("查询 upstream_channels 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return AdminConfigImportReport{}, fmt.Errorf("扫描 upstream_channels 失败: %w", err)
		}
		if err := s.syncUpstreamChannelGroupsCacheTx(ctx, tx, id); err != nil {
			return AdminConfigImportReport{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return AdminConfigImportReport{}, fmt.Errorf("遍历 upstream_channels 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AdminConfigImportReport{}, fmt.Errorf("提交事务失败: %w", err)
	}

	return AdminConfigImportReport{
		ChannelGroups:       len(in.ChannelGroups),
		ChannelGroupMembers: len(in.ChannelGroupMembers),
		UpstreamChannels:    len(in.UpstreamChannels),
		UpstreamEndpoints:   len(in.UpstreamEndpoints),
		ManagedModels:       len(in.ManagedModels),
		ChannelModels:       len(in.ChannelModels),
	}, nil
}
