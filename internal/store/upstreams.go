// Package store 封装上游资源（channel/endpoint/credential/account）的读写，用于调度与管理面配置。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	UpstreamTypeOpenAICompatible = "openai_compatible"
	UpstreamTypeCodexOAuth       = "codex_oauth"
	UpstreamTypeAnthropic        = "anthropic"
)

func (s *Store) ListUpstreamChannels(ctx context.Context) ([]UpstreamChannel, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, type, name, `groups`, status, priority, promotion,\n"+
			"       allow_service_tier, disable_store, allow_safety_identifier,\n"+
			"       openai_organization, test_model, tag, remark, weight, auto_ban, setting,\n"+
			"       param_override,\n"+
			"       header_override,\n"+
			"       status_code_mapping,\n"+
			"       model_suffix_preserve,\n"+
			"       request_body_blacklist,\n"+
			"       request_body_whitelist,\n"+
			"       last_test_at, last_test_latency_ms, last_test_ok,\n"+
			"       created_at, updated_at\n"+
			"FROM upstream_channels\n"+
			"ORDER BY promotion DESC, priority DESC, id DESC\n",
	)
	if err != nil {
		return nil, fmt.Errorf("查询 upstream_channels 失败: %w", err)
	}
	defer rows.Close()

	var out []UpstreamChannel
	for rows.Next() {
		var c UpstreamChannel
		var promotion int
		var allowServiceTier int
		var disableStore int
		var allowSafetyIdentifier int
		var openAIOrganization sql.NullString
		var testModel sql.NullString
		var tag sql.NullString
		var remark sql.NullString
		var weight int
		var autoBan int
		var setting sql.NullString
		var paramOverride sql.NullString
		var headerOverride sql.NullString
		var statusCodeMapping sql.NullString
		var modelSuffixPreserve sql.NullString
		var requestBodyBlacklist sql.NullString
		var requestBodyWhitelist sql.NullString
		var lastOK int
		if err := rows.Scan(&c.ID, &c.Type, &c.Name, &c.Groups, &c.Status, &c.Priority, &promotion,
			&allowServiceTier, &disableStore, &allowSafetyIdentifier,
			&openAIOrganization, &testModel, &tag, &remark, &weight, &autoBan, &setting,
			&paramOverride,
			&headerOverride,
			&statusCodeMapping,
			&modelSuffixPreserve,
			&requestBodyBlacklist,
			&requestBodyWhitelist,
			&c.LastTestAt, &c.LastTestLatencyMS, &lastOK,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 upstream_channels 失败: %w", err)
		}
		c.Promotion = promotion != 0
		c.AllowServiceTier = allowServiceTier != 0
		c.DisableStore = disableStore != 0
		c.AllowSafetyIdentifier = allowSafetyIdentifier != 0
		if openAIOrganization.Valid && strings.TrimSpace(openAIOrganization.String) != "" {
			v := strings.TrimSpace(openAIOrganization.String)
			c.OpenAIOrganization = &v
		}
		if testModel.Valid && strings.TrimSpace(testModel.String) != "" {
			v := strings.TrimSpace(testModel.String)
			c.TestModel = &v
		}
		if tag.Valid && strings.TrimSpace(tag.String) != "" {
			v := strings.TrimSpace(tag.String)
			c.Tag = &v
		}
		if remark.Valid && strings.TrimSpace(remark.String) != "" {
			v := strings.TrimSpace(remark.String)
			c.Remark = &v
		}
		c.Weight = weight
		c.AutoBan = autoBan != 0
		if setting.Valid && strings.TrimSpace(setting.String) != "" && strings.TrimSpace(setting.String) != "{}" {
			if err := json.Unmarshal([]byte(setting.String), &c.Setting); err != nil {
				return nil, fmt.Errorf("解析 upstream_channels.setting 失败: %w", err)
			}
		}
		if paramOverride.Valid {
			c.ParamOverride = strings.TrimSpace(paramOverride.String)
		}
		if headerOverride.Valid {
			c.HeaderOverride = strings.TrimSpace(headerOverride.String)
		}
		if statusCodeMapping.Valid {
			c.StatusCodeMapping = strings.TrimSpace(statusCodeMapping.String)
		}
		if modelSuffixPreserve.Valid {
			c.ModelSuffixPreserve = strings.TrimSpace(modelSuffixPreserve.String)
		}
		if requestBodyBlacklist.Valid {
			c.RequestBodyBlacklist = strings.TrimSpace(requestBodyBlacklist.String)
		}
		if requestBodyWhitelist.Valid {
			c.RequestBodyWhitelist = strings.TrimSpace(requestBodyWhitelist.String)
		}
		c.LastTestOK = lastOK != 0
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 upstream_channels 失败: %w", err)
	}
	return out, nil
}

func (s *Store) CountUpstreamChannels(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM upstream_channels`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计 upstream_channels 失败: %w", err)
	}
	return n, nil
}

func (s *Store) CreateUpstreamChannel(ctx context.Context, typ, name, groups string, priority int, promotion bool, allowServiceTier bool, disableStore bool, allowSafetyIdentifier bool) (int64, error) {
	p := 0
	if promotion {
		p = 1
	}
	allowServiceTierInt := 0
	if allowServiceTier {
		allowServiceTierInt = 1
	}
	disableStoreInt := 0
	if disableStore {
		disableStoreInt = 1
	}
	allowSafetyIdentifierInt := 0
	if allowSafetyIdentifier {
		allowSafetyIdentifierInt = 1
	}
	names := splitUpstreamChannelGroupsCSV(groups)
	groups = strings.Join(names, ",")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		"INSERT INTO upstream_channels(type, name, `groups`, status, priority, promotion, allow_service_tier, disable_store, allow_safety_identifier, created_at, updated_at)\n"+
			"VALUES(?, ?, ?, 1, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)\n",
		typ, name, groups, priority, p,
		allowServiceTierInt,
		disableStoreInt,
		allowSafetyIdentifierInt,
	)
	if err != nil {
		return 0, fmt.Errorf("创建 upstream_channel 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 upstream_channel id 失败: %w", err)
	}

	if len(names) > 0 {
		var b strings.Builder
		b.WriteString("SELECT id, name FROM channel_groups WHERE name IN (")
		for i := range names {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("?")
		}
		b.WriteString(")")
		args := make([]any, 0, len(names))
		for _, n := range names {
			args = append(args, n)
		}
		rows, err := tx.QueryContext(ctx, b.String(), args...)
		if err != nil {
			return 0, fmt.Errorf("查询 channel_groups 失败: %w", err)
		}
		defer rows.Close()

		idByName := make(map[string]int64, len(names))
		for rows.Next() {
			var gid int64
			var gname string
			if err := rows.Scan(&gid, &gname); err != nil {
				return 0, fmt.Errorf("扫描 channel_groups 失败: %w", err)
			}
			gname = strings.TrimSpace(gname)
			if gname == "" || gid == 0 {
				continue
			}
			idByName[gname] = gid
		}
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("遍历 channel_groups 失败: %w", err)
		}

		for _, gname := range names {
			gid, ok := idByName[gname]
			if !ok || gid == 0 {
				return 0, fmt.Errorf("分组不存在：%s", gname)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, gid, id, priority, p); err != nil {
				return 0, fmt.Errorf("创建 channel_group_members 失败: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateUpstreamChannelBasics(ctx context.Context, channelID int64, name string, status int, priority int, promotion bool, allowServiceTier bool, disableStore bool, allowSafetyIdentifier bool) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name 不能为空")
	}

	promotionInt := 0
	if promotion {
		promotionInt = 1
	}
	allowServiceTierInt := 0
	if allowServiceTier {
		allowServiceTierInt = 1
	}
	disableStoreInt := 0
	if disableStore {
		disableStoreInt = 1
	}
	allowSafetyIdentifierInt := 0
	if allowSafetyIdentifier {
		allowSafetyIdentifierInt = 1
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
UPDATE upstream_channels
SET name=?,
    status=?,
    priority=?,
    promotion=?,
    allow_service_tier=?,
    disable_store=?,
    allow_safety_identifier=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, name, status, priority, promotionInt, allowServiceTierInt, disableStoreInt, allowSafetyIdentifierInt, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel 失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	// 同步 channel_group_members（SSOT）：组内排序依赖 priority/promotion。
	if _, err := tx.ExecContext(ctx, `
UPDATE channel_group_members
SET priority=?, promotion=?, updated_at=CURRENT_TIMESTAMP
WHERE member_channel_id=?
`, priority, promotionInt, channelID); err != nil {
		return fmt.Errorf("更新 channel_group_members 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) GetUpstreamChannelByID(ctx context.Context, id int64) (UpstreamChannel, error) {
	var c UpstreamChannel
	var promotion int
	var allowServiceTier int
	var disableStore int
	var allowSafetyIdentifier int
	var openAIOrganization sql.NullString
	var testModel sql.NullString
	var tag sql.NullString
	var remark sql.NullString
	var weight int
	var autoBan int
	var setting sql.NullString
	var paramOverride sql.NullString
	var headerOverride sql.NullString
	var statusCodeMapping sql.NullString
	var modelSuffixPreserve sql.NullString
	var requestBodyBlacklist sql.NullString
	var requestBodyWhitelist sql.NullString
	var lastOK int
	err := s.db.QueryRowContext(ctx,
		"SELECT id, type, name, `groups`, status, priority, promotion,\n"+
			"       allow_service_tier, disable_store, allow_safety_identifier,\n"+
			"       openai_organization, test_model, tag, remark, weight, auto_ban, setting,\n"+
			"       param_override,\n"+
			"       header_override,\n"+
			"       status_code_mapping,\n"+
			"       model_suffix_preserve,\n"+
			"       request_body_blacklist,\n"+
			"       request_body_whitelist,\n"+
			"       last_test_at, last_test_latency_ms, last_test_ok,\n"+
			"       created_at, updated_at\n"+
			"FROM upstream_channels\n"+
			"WHERE id=?\n",
		id,
	).Scan(&c.ID, &c.Type, &c.Name, &c.Groups, &c.Status, &c.Priority, &promotion,
		&allowServiceTier, &disableStore, &allowSafetyIdentifier,
		&openAIOrganization, &testModel, &tag, &remark, &weight, &autoBan, &setting,
		&paramOverride,
		&headerOverride,
		&statusCodeMapping,
		&modelSuffixPreserve,
		&requestBodyBlacklist,
		&requestBodyWhitelist,
		&c.LastTestAt, &c.LastTestLatencyMS, &lastOK,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpstreamChannel{}, sql.ErrNoRows
		}
		return UpstreamChannel{}, fmt.Errorf("查询 upstream_channel 失败: %w", err)
	}
	c.Promotion = promotion != 0
	c.AllowServiceTier = allowServiceTier != 0
	c.DisableStore = disableStore != 0
	c.AllowSafetyIdentifier = allowSafetyIdentifier != 0
	if openAIOrganization.Valid && strings.TrimSpace(openAIOrganization.String) != "" {
		v := strings.TrimSpace(openAIOrganization.String)
		c.OpenAIOrganization = &v
	}
	if testModel.Valid && strings.TrimSpace(testModel.String) != "" {
		v := strings.TrimSpace(testModel.String)
		c.TestModel = &v
	}
	if tag.Valid && strings.TrimSpace(tag.String) != "" {
		v := strings.TrimSpace(tag.String)
		c.Tag = &v
	}
	if remark.Valid && strings.TrimSpace(remark.String) != "" {
		v := strings.TrimSpace(remark.String)
		c.Remark = &v
	}
	c.Weight = weight
	c.AutoBan = autoBan != 0
	if setting.Valid && strings.TrimSpace(setting.String) != "" && strings.TrimSpace(setting.String) != "{}" {
		if err := json.Unmarshal([]byte(setting.String), &c.Setting); err != nil {
			return UpstreamChannel{}, fmt.Errorf("解析 upstream_channels.setting 失败: %w", err)
		}
	}
	if paramOverride.Valid {
		c.ParamOverride = strings.TrimSpace(paramOverride.String)
	}
	if headerOverride.Valid {
		c.HeaderOverride = strings.TrimSpace(headerOverride.String)
	}
	if statusCodeMapping.Valid {
		c.StatusCodeMapping = strings.TrimSpace(statusCodeMapping.String)
	}
	if modelSuffixPreserve.Valid {
		c.ModelSuffixPreserve = strings.TrimSpace(modelSuffixPreserve.String)
	}
	if requestBodyBlacklist.Valid {
		c.RequestBodyBlacklist = strings.TrimSpace(requestBodyBlacklist.String)
	}
	if requestBodyWhitelist.Valid {
		c.RequestBodyWhitelist = strings.TrimSpace(requestBodyWhitelist.String)
	}
	c.LastTestOK = lastOK != 0
	return c, nil
}

func (s *Store) UpdateUpstreamChannelRequestPolicy(ctx context.Context, channelID int64, allowServiceTier bool, disableStore bool, allowSafetyIdentifier bool) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}
	allowServiceTierInt := 0
	if allowServiceTier {
		allowServiceTierInt = 1
	}
	disableStoreInt := 0
	if disableStore {
		disableStoreInt = 1
	}
	allowSafetyIdentifierInt := 0
	if allowSafetyIdentifier {
		allowSafetyIdentifierInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET allow_service_tier=?, disable_store=?, allow_safety_identifier=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, allowServiceTierInt, disableStoreInt, allowSafetyIdentifierInt, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel request policy 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelParamOverride(ctx context.Context, channelID int64, paramOverride string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	paramOverride = strings.TrimSpace(paramOverride)
	if paramOverride != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(paramOverride), &parsed); err != nil {
			return fmt.Errorf("param_override 不是有效 JSON: %w", err)
		}
	}

	var v any
	if paramOverride != "" {
		v = paramOverride
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET param_override=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, v, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel param_override 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelHeaderOverride(ctx context.Context, channelID int64, headerOverride string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	headerOverride = strings.TrimSpace(headerOverride)
	if headerOverride != "" {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(headerOverride), &parsed); err != nil {
			return fmt.Errorf("header_override 不是有效 JSON: %w", err)
		}
	}

	var v any
	if headerOverride != "" {
		v = headerOverride
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET header_override=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, v, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel header_override 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelStatusCodeMapping(ctx context.Context, channelID int64, statusCodeMapping string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	statusCodeMapping = strings.TrimSpace(statusCodeMapping)
	if statusCodeMapping == "{}" {
		statusCodeMapping = ""
	}

	if statusCodeMapping != "" {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(statusCodeMapping), &parsed); err != nil {
			return fmt.Errorf("status_code_mapping 不是有效 JSON: %w", err)
		}
		for k, v := range parsed {
			if _, err := strconv.Atoi(strings.TrimSpace(k)); err != nil {
				return fmt.Errorf("status_code_mapping key 不合法: %s", k)
			}
			if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return fmt.Errorf("status_code_mapping value 不合法: %s", v)
			}
		}
	}

	var vv any
	if statusCodeMapping != "" {
		vv = statusCodeMapping
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET status_code_mapping=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, vv, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel status_code_mapping 失败: %w", err)
	}
	return nil
}

func normalizeStringJSONArray(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return "", nil
	}

	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", err
	}
	for i := range parsed {
		parsed[i] = strings.TrimSpace(parsed[i])
		if parsed[i] == "" {
			return "", errors.New("数组元素不能为空")
		}
	}
	b, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(string(b))
	if out == "[]" {
		return "", nil
	}
	return out, nil
}

func (s *Store) UpdateUpstreamChannelModelSuffixPreserve(ctx context.Context, channelID int64, modelSuffixPreserve string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	normalized, err := normalizeStringJSONArray(modelSuffixPreserve)
	if err != nil {
		return fmt.Errorf("model_suffix_preserve 不是有效 JSON 数组: %w", err)
	}

	var v any
	if normalized != "" {
		v = normalized
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET model_suffix_preserve=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, v, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel model_suffix_preserve 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelRequestBodyBlacklist(ctx context.Context, channelID int64, requestBodyBlacklist string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	normalized, err := normalizeStringJSONArray(requestBodyBlacklist)
	if err != nil {
		return fmt.Errorf("request_body_blacklist 不是有效 JSON 数组: %w", err)
	}

	var v any
	if normalized != "" {
		v = normalized
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET request_body_blacklist=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, v, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel request_body_blacklist 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelRequestBodyWhitelist(ctx context.Context, channelID int64, requestBodyWhitelist string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	normalized, err := normalizeStringJSONArray(requestBodyWhitelist)
	if err != nil {
		return fmt.Errorf("request_body_whitelist 不是有效 JSON 数组: %w", err)
	}

	var v any
	if normalized != "" {
		v = normalized
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET request_body_whitelist=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, v, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel request_body_whitelist 失败: %w", err)
	}
	return nil
}

func (s *Store) SetUpstreamChannelGroups(ctx context.Context, channelID int64, groups string) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	names := splitUpstreamChannelGroupsCSV(groups)
	groups = strings.Join(names, ",")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var chPriority int
	var chPromotion int
	if err := tx.QueryRowContext(ctx, `SELECT priority, promotion FROM upstream_channels WHERE id=?`, channelID).Scan(&chPriority, &chPromotion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("查询 upstream_channel 失败: %w", err)
	}

	// SSOT：同步更新成员关系（组 -> 渠道），并回填 upstream_channels.groups 兼容缓存。
	if _, err := tx.ExecContext(ctx, `DELETE FROM channel_group_members WHERE member_channel_id=?`, channelID); err != nil {
		return fmt.Errorf("清理 channel_group_members 失败: %w", err)
	}

	if len(names) > 0 {
		var b strings.Builder
		b.WriteString("SELECT id, name FROM channel_groups WHERE name IN (")
		for i := range names {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("?")
		}
		b.WriteString(")")
		args := make([]any, 0, len(names))
		for _, n := range names {
			args = append(args, n)
		}
		rows, err := tx.QueryContext(ctx, b.String(), args...)
		if err != nil {
			return fmt.Errorf("查询 channel_groups 失败: %w", err)
		}
		defer rows.Close()

		idByName := make(map[string]int64, len(names))
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return fmt.Errorf("扫描 channel_groups 失败: %w", err)
			}
			name = strings.TrimSpace(name)
			if name == "" || id == 0 {
				continue
			}
			idByName[name] = id
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("遍历 channel_groups 失败: %w", err)
		}

		memberUpsert := `
INSERT INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE priority=VALUES(priority), promotion=VALUES(promotion), updated_at=CURRENT_TIMESTAMP
`
		if s.dialect == DialectSQLite {
			memberUpsert = `
INSERT INTO channel_group_members(parent_group_id, member_channel_id, priority, promotion, created_at, updated_at)
VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(parent_group_id, member_channel_id) DO UPDATE SET priority=excluded.priority, promotion=excluded.promotion, updated_at=CURRENT_TIMESTAMP
`
		}

		for _, name := range names {
			id, ok := idByName[name]
			if !ok || id == 0 {
				return fmt.Errorf("分组不存在：%s", name)
			}
			_, err := tx.ExecContext(ctx, memberUpsert, id, channelID, chPriority, chPromotion)
			if err != nil {
				return fmt.Errorf("写入 channel_group_members 失败: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, "UPDATE upstream_channels SET `groups`=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", groups, channelID); err != nil {
		return fmt.Errorf("更新 upstream_channel groups 失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelTest(ctx context.Context, channelID int64, ok bool, latencyMS int) error {
	okInt := 0
	if ok {
		okInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET last_test_at=CURRENT_TIMESTAMP, last_test_latency_ms=?, last_test_ok=?, updated_at=updated_at
WHERE id=?
`, latencyMS, okInt, channelID)
	if err != nil {
		return fmt.Errorf("更新 upstream_channel 测试结果失败: %w", err)
	}
	return nil
}

func (s *Store) ListUpstreamEndpointsByChannel(ctx context.Context, channelID int64) ([]UpstreamEndpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel_id, base_url, status, priority, created_at, updated_at
FROM upstream_endpoints
WHERE channel_id=?
ORDER BY priority DESC, id DESC
`, channelID)
	if err != nil {
		return nil, fmt.Errorf("查询 upstream_endpoints 失败: %w", err)
	}
	defer rows.Close()

	var out []UpstreamEndpoint
	for rows.Next() {
		var e UpstreamEndpoint
		if err := rows.Scan(&e.ID, &e.ChannelID, &e.BaseURL, &e.Status, &e.Priority, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 upstream_endpoints 失败: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 upstream_endpoints 失败: %w", err)
	}
	return out, nil
}

// GetUpstreamEndpointByChannelID 返回指定 channel 的“主” endpoint。
// 在单 endpoint 模式下，理论上每个 channel 仅有一条 endpoint 记录；这里仍保留 ORDER BY 以兼容历史数据。
func (s *Store) GetUpstreamEndpointByChannelID(ctx context.Context, channelID int64) (UpstreamEndpoint, error) {
	var e UpstreamEndpoint
	err := s.db.QueryRowContext(ctx, `
SELECT id, channel_id, base_url, status, priority, created_at, updated_at
FROM upstream_endpoints
WHERE channel_id=?
ORDER BY priority DESC, id DESC
LIMIT 1
`, channelID).Scan(&e.ID, &e.ChannelID, &e.BaseURL, &e.Status, &e.Priority, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpstreamEndpoint{}, sql.ErrNoRows
		}
		return UpstreamEndpoint{}, fmt.Errorf("查询 upstream_endpoint 失败: %w", err)
	}
	return e, nil
}

func (s *Store) CountUpstreamEndpoints(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM upstream_endpoints`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计 upstream_endpoints 失败: %w", err)
	}
	return n, nil
}

func (s *Store) CreateUpstreamEndpoint(ctx context.Context, channelID int64, baseURL string, priority int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
VALUES(?, ?, 1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, channelID, baseURL, priority)
	if err != nil {
		return 0, fmt.Errorf("创建 upstream_endpoint 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 upstream_endpoint id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) SetUpstreamEndpointBaseURL(ctx context.Context, channelID int64, baseURL string) (UpstreamEndpoint, error) {
	ep, err := s.GetUpstreamEndpointByChannelID(ctx, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := s.CreateUpstreamEndpoint(ctx, channelID, baseURL, 0); err != nil {
				return UpstreamEndpoint{}, err
			}
			return s.GetUpstreamEndpointByChannelID(ctx, channelID)
		}
		return UpstreamEndpoint{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE upstream_endpoints
SET base_url=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, baseURL, ep.ID); err != nil {
		return UpstreamEndpoint{}, fmt.Errorf("更新 upstream_endpoint base_url 失败: %w", err)
	}
	return s.GetUpstreamEndpointByID(ctx, ep.ID)
}

func (s *Store) GetUpstreamEndpointByID(ctx context.Context, id int64) (UpstreamEndpoint, error) {
	var e UpstreamEndpoint
	err := s.db.QueryRowContext(ctx, `
SELECT id, channel_id, base_url, status, priority, created_at, updated_at
FROM upstream_endpoints
WHERE id=?
`, id).Scan(&e.ID, &e.ChannelID, &e.BaseURL, &e.Status, &e.Priority, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpstreamEndpoint{}, sql.ErrNoRows
		}
		return UpstreamEndpoint{}, fmt.Errorf("查询 upstream_endpoint 失败: %w", err)
	}
	return e, nil
}

func (s *Store) ListOpenAICompatibleCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]OpenAICompatibleCredential, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, endpoint_id, name, api_key_enc, api_key_hint, status,
       last_used_at, created_at, updated_at
FROM openai_compatible_credentials
WHERE endpoint_id=?
ORDER BY id DESC
`, endpointID)
	if err != nil {
		return nil, fmt.Errorf("查询 openai_compatible_credentials 失败: %w", err)
	}
	defer rows.Close()

	var out []OpenAICompatibleCredential
	for rows.Next() {
		var c OpenAICompatibleCredential
		if err := rows.Scan(&c.ID, &c.EndpointID, &c.Name, &c.APIKeyEnc, &c.APIKeyHint, &c.Status,
			&c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 openai_compatible_credentials 失败: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 openai_compatible_credentials 失败: %w", err)
	}
	return out, nil
}

type OpenAICredentialSecret struct {
	ID         int64
	EndpointID int64
	Name       *string
	APIKey     string
	APIKeyHint *string
	Status     int
}

func (s *Store) CreateOpenAICompatibleCredential(ctx context.Context, endpointID int64, name *string, apiKey string) (int64, *string, error) {
	enc := []byte(apiKey)
	hint := tokenHint(apiKey)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO openai_compatible_credentials(endpoint_id, name, api_key_enc, api_key_hint, status, created_at, updated_at)
VALUES(?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, endpointID, name, enc, hint)
	if err != nil {
		return 0, nil, fmt.Errorf("创建 openai_compatible_credential 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, nil, fmt.Errorf("获取 openai_compatible_credential id 失败: %w", err)
	}
	return id, hint, nil
}

func (s *Store) GetOpenAICompatibleCredentialSecret(ctx context.Context, credentialID int64) (OpenAICredentialSecret, error) {
	var c OpenAICompatibleCredential
	err := s.db.QueryRowContext(ctx, `
SELECT id, endpoint_id, name, api_key_enc, api_key_hint, status
FROM openai_compatible_credentials
WHERE id=?
	`, credentialID).Scan(&c.ID, &c.EndpointID, &c.Name, &c.APIKeyEnc, &c.APIKeyHint, &c.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OpenAICredentialSecret{}, sql.ErrNoRows
		}
		return OpenAICredentialSecret{}, fmt.Errorf("查询 openai_compatible_credential 失败: %w", err)
	}
	if looksLikeLegacyEncryptedBlob(c.APIKeyEnc) {
		return OpenAICredentialSecret{}, errors.New("该 credential 为旧版加密格式，当前已禁用应用层加密；请删除并重新录入 api_key")
	}
	plain := c.APIKeyEnc
	return OpenAICredentialSecret{
		ID:         c.ID,
		EndpointID: c.EndpointID,
		Name:       c.Name,
		APIKey:     string(plain),
		APIKeyHint: c.APIKeyHint,
		Status:     c.Status,
	}, nil
}

func (s *Store) TouchOpenAICompatibleCredential(ctx context.Context, credentialID int64) {
	_, _ = s.db.ExecContext(ctx, `UPDATE openai_compatible_credentials SET last_used_at=CURRENT_TIMESTAMP, updated_at=updated_at WHERE id=?`, credentialID)
}

func (s *Store) CreateCodexOAuthPending(ctx context.Context, state string, endpointID, actorUserID int64, codeVerifier string, createdAt time.Time) error {
	if strings.TrimSpace(state) == "" {
		return fmt.Errorf("state 不能为空")
	}
	if strings.TrimSpace(codeVerifier) == "" {
		return fmt.Errorf("code_verifier 不能为空")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO codex_oauth_pending(state, endpoint_id, actor_user_id, code_verifier, created_at)
VALUES(?, ?, ?, ?, ?)
`, state, endpointID, actorUserID, codeVerifier, createdAt)
	if err != nil {
		return fmt.Errorf("创建 codex_oauth_pending 失败: %w", err)
	}
	return nil
}

func (s *Store) GetCodexOAuthPending(ctx context.Context, state string) (CodexOAuthPending, bool, error) {
	var p CodexOAuthPending
	err := s.db.QueryRowContext(ctx, `
SELECT state, endpoint_id, actor_user_id, code_verifier, created_at
FROM codex_oauth_pending
WHERE state=?
`, state).Scan(&p.State, &p.EndpointID, &p.ActorUserID, &p.CodeVerifier, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CodexOAuthPending{}, false, nil
		}
		return CodexOAuthPending{}, false, fmt.Errorf("查询 codex_oauth_pending 失败: %w", err)
	}
	return p, true, nil
}

func (s *Store) DeleteCodexOAuthPending(ctx context.Context, state string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM codex_oauth_pending WHERE state=?`, state)
	if err != nil {
		return fmt.Errorf("删除 codex_oauth_pending 失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteCodexOAuthPendingBefore(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM codex_oauth_pending WHERE created_at < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("清理 codex_oauth_pending 失败: %w", err)
	}
	return nil
}

func (s *Store) ListCodexOAuthAccountsByEndpoint(ctx context.Context, endpointID int64) ([]CodexOAuthAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, endpoint_id, account_id, email, access_token_enc, refresh_token_enc, id_token_enc,
       expires_at, last_refresh_at, status,
       cooldown_until, last_used_at,
       balance_total_granted_usd, balance_total_used_usd, balance_total_available_usd,
       balance_updated_at, balance_error,
       quota_credits_has_credits, quota_credits_unlimited, quota_credits_balance,
       quota_primary_used_percent, quota_primary_reset_at,
       quota_secondary_used_percent, quota_secondary_reset_at,
       quota_updated_at, quota_error,
       created_at, updated_at
FROM codex_oauth_accounts
WHERE endpoint_id=?
ORDER BY id DESC
`, endpointID)
	if err != nil {
		return nil, fmt.Errorf("查询 codex_oauth_accounts 失败: %w", err)
	}
	defer rows.Close()

	var out []CodexOAuthAccount
	for rows.Next() {
		var a CodexOAuthAccount
		var idTokenEnc []byte
		var balGranted decimal.NullDecimal
		var balUsed decimal.NullDecimal
		var balAvail decimal.NullDecimal
		var balUpdatedAt sql.NullTime
		var balErr sql.NullString
		var quotaHasCredits sql.NullBool
		var quotaUnlimited sql.NullBool
		var quotaBalance sql.NullString
		var quotaPrimaryUsed sql.NullInt64
		var quotaPrimaryResetAt sql.NullTime
		var quotaSecondaryUsed sql.NullInt64
		var quotaSecondaryResetAt sql.NullTime
		var quotaUpdatedAt sql.NullTime
		var quotaErr sql.NullString
		if err := rows.Scan(&a.ID, &a.EndpointID, &a.AccountID, &a.Email, &a.AccessTokenEnc, &a.RefreshTokenEnc, &idTokenEnc,
			&a.ExpiresAt, &a.LastRefreshAt, &a.Status,
			&a.CooldownUntil, &a.LastUsedAt,
			&balGranted, &balUsed, &balAvail,
			&balUpdatedAt, &balErr,
			&quotaHasCredits, &quotaUnlimited, &quotaBalance,
			&quotaPrimaryUsed, &quotaPrimaryResetAt,
			&quotaSecondaryUsed, &quotaSecondaryResetAt,
			&quotaUpdatedAt, &quotaErr,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 codex_oauth_accounts 失败: %w", err)
		}
		a.IDTokenEnc = idTokenEnc
		if balGranted.Valid {
			v := balGranted.Decimal.Truncate(USDScale)
			a.BalanceTotalGrantedUSD = &v
		}
		if balUsed.Valid {
			v := balUsed.Decimal.Truncate(USDScale)
			a.BalanceTotalUsedUSD = &v
		}
		if balAvail.Valid {
			v := balAvail.Decimal.Truncate(USDScale)
			a.BalanceTotalAvailableUSD = &v
		}
		if balUpdatedAt.Valid {
			t := balUpdatedAt.Time
			a.BalanceUpdatedAt = &t
		}
		if balErr.Valid {
			msg := balErr.String
			if strings.TrimSpace(msg) != "" {
				a.BalanceError = &msg
			}
		}
		if quotaHasCredits.Valid {
			v := quotaHasCredits.Bool
			a.QuotaCreditsHasCredits = &v
		}
		if quotaUnlimited.Valid {
			v := quotaUnlimited.Bool
			a.QuotaCreditsUnlimited = &v
		}
		if quotaBalance.Valid {
			msg := strings.TrimSpace(quotaBalance.String)
			if msg != "" {
				a.QuotaCreditsBalance = &msg
			}
		}
		if quotaPrimaryUsed.Valid {
			v := int(quotaPrimaryUsed.Int64)
			a.QuotaPrimaryUsedPercent = &v
		}
		if quotaPrimaryResetAt.Valid {
			t := quotaPrimaryResetAt.Time
			a.QuotaPrimaryResetAt = &t
		}
		if quotaSecondaryUsed.Valid {
			v := int(quotaSecondaryUsed.Int64)
			a.QuotaSecondaryUsedPercent = &v
		}
		if quotaSecondaryResetAt.Valid {
			t := quotaSecondaryResetAt.Time
			a.QuotaSecondaryResetAt = &t
		}
		if quotaUpdatedAt.Valid {
			t := quotaUpdatedAt.Time
			a.QuotaUpdatedAt = &t
		}
		if quotaErr.Valid {
			msg := strings.TrimSpace(quotaErr.String)
			if msg != "" {
				a.QuotaError = &msg
			}
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 codex_oauth_accounts 失败: %w", err)
	}
	return out, nil
}

func (s *Store) ListCodexOAuthAccountRefs(ctx context.Context) ([]CodexOAuthAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, endpoint_id, status
FROM codex_oauth_accounts
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 codex_oauth_accounts 失败: %w", err)
	}
	defer rows.Close()

	var out []CodexOAuthAccount
	for rows.Next() {
		var a CodexOAuthAccount
		if err := rows.Scan(&a.ID, &a.EndpointID, &a.Status); err != nil {
			return nil, fmt.Errorf("扫描 codex_oauth_accounts 失败: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 codex_oauth_accounts 失败: %w", err)
	}
	return out, nil
}

type CodexOAuthSecret struct {
	ID           int64
	EndpointID   int64
	AccountID    string
	Email        *string
	AccessToken  string
	RefreshToken string
	IDToken      *string
	ExpiresAt    *time.Time
	Status       int
}

func (s *Store) CreateCodexOAuthAccount(ctx context.Context, endpointID int64, accountID string, email *string, accessToken, refreshToken string, idToken *string, expiresAt *time.Time) (int64, error) {
	accessEnc := []byte(accessToken)
	refreshEnc := []byte(refreshToken)
	var idTokenEnc []byte
	if idToken != nil && *idToken != "" {
		idTokenEnc = []byte(*idToken)
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO codex_oauth_accounts(endpoint_id, account_id, email, access_token_enc, refresh_token_enc, id_token_enc,
                               expires_at, last_refresh_at, status, cooldown_until, last_used_at, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, NULL, 1, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, endpointID, accountID, email, accessEnc, refreshEnc, nullableBytes(idTokenEnc), expiresAt)
	if err != nil {
		return 0, fmt.Errorf("创建 codex_oauth_account 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 codex_oauth_account id 失败: %w", err)
	}
	return id, nil
}

func (s *Store) GetCodexOAuthSecret(ctx context.Context, accountID int64) (CodexOAuthSecret, error) {
	var a CodexOAuthAccount
	var idTokenEnc []byte
	err := s.db.QueryRowContext(ctx, `
SELECT id, endpoint_id, account_id, email, access_token_enc, refresh_token_enc, id_token_enc, expires_at, status
FROM codex_oauth_accounts
WHERE id=?
	`, accountID).Scan(&a.ID, &a.EndpointID, &a.AccountID, &a.Email, &a.AccessTokenEnc, &a.RefreshTokenEnc, &idTokenEnc, &a.ExpiresAt, &a.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CodexOAuthSecret{}, sql.ErrNoRows
		}
		return CodexOAuthSecret{}, fmt.Errorf("查询 codex_oauth_account 失败: %w", err)
	}
	if looksLikeLegacyEncryptedBlob(a.AccessTokenEnc) || looksLikeLegacyEncryptedBlob(a.RefreshTokenEnc) || looksLikeLegacyEncryptedBlob(idTokenEnc) {
		return CodexOAuthSecret{}, errors.New("该 Codex OAuth account 为旧版加密格式，当前已禁用应用层加密；请删除账号并重新授权")
	}
	accessPlain := a.AccessTokenEnc
	refreshPlain := a.RefreshTokenEnc
	var idTokenPlain *string
	if len(idTokenEnc) > 0 {
		s := string(idTokenEnc)
		idTokenPlain = &s
	}
	return CodexOAuthSecret{
		ID:           a.ID,
		EndpointID:   a.EndpointID,
		AccountID:    a.AccountID,
		Email:        a.Email,
		AccessToken:  string(accessPlain),
		RefreshToken: string(refreshPlain),
		IDToken:      idTokenPlain,
		ExpiresAt:    a.ExpiresAt,
		Status:       a.Status,
	}, nil
}

func (s *Store) UpdateCodexOAuthAccountTokens(ctx context.Context, accountID int64, accessToken, refreshToken string, idToken *string, expiresAt *time.Time) error {
	accessEnc := []byte(accessToken)
	refreshEnc := []byte(refreshToken)
	var idTokenEnc []byte
	if idToken != nil && *idToken != "" {
		idTokenEnc = []byte(*idToken)
	}
	_, err := s.db.ExecContext(ctx, `
	UPDATE codex_oauth_accounts
	SET access_token_enc=?, refresh_token_enc=?, id_token_enc=?, expires_at=?, last_refresh_at=CURRENT_TIMESTAMP, cooldown_until=NULL, updated_at=CURRENT_TIMESTAMP
	WHERE id=?
	`, accessEnc, refreshEnc, nullableBytes(idTokenEnc), expiresAt, accountID)
	if err != nil {
		return fmt.Errorf("更新 codex_oauth_account token 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateCodexOAuthAccountBalance(ctx context.Context, accountID int64, grantedUSD, usedUSD, availableUSD *decimal.Decimal, updatedAt time.Time, errMsg *string) error {
	var errVal any
	if errMsg != nil && strings.TrimSpace(*errMsg) != "" {
		msg := strings.TrimSpace(*errMsg)
		if len(msg) > 255 {
			msg = msg[:255]
		}
		errVal = msg
	}

	var grantedVal any
	if grantedUSD != nil {
		grantedVal = grantedUSD.Truncate(USDScale)
	}
	var usedVal any
	if usedUSD != nil {
		usedVal = usedUSD.Truncate(USDScale)
	}
	var availableVal any
	if availableUSD != nil {
		availableVal = availableUSD.Truncate(USDScale)
	}

	_, err := s.db.ExecContext(ctx, `
	UPDATE codex_oauth_accounts
	SET balance_total_granted_usd=?,
	    balance_total_used_usd=?,
	    balance_total_available_usd=?,
	    balance_updated_at=?,
	    balance_error=?,
	    updated_at=updated_at
	WHERE id=?
	`, grantedVal, usedVal, availableVal, updatedAt, errVal, accountID)
	if err != nil {
		return fmt.Errorf("更新 codex_oauth_account balance 失败: %w", err)
	}
	return nil
}

type CodexOAuthQuota struct {
	CreditsHasCredits    *bool
	CreditsUnlimited     *bool
	CreditsBalance       *string
	PrimaryUsedPercent   *int
	PrimaryResetAt       *time.Time
	SecondaryUsedPercent *int
	SecondaryResetAt     *time.Time
}

func (s *Store) UpdateCodexOAuthAccountQuota(ctx context.Context, accountID int64, q CodexOAuthQuota, updatedAt time.Time, errMsg *string) error {
	var errVal any
	if errMsg != nil && strings.TrimSpace(*errMsg) != "" {
		msg := strings.TrimSpace(*errMsg)
		if len(msg) > 255 {
			msg = msg[:255]
		}
		errVal = msg
	}

	var balanceVal any
	if q.CreditsBalance != nil && strings.TrimSpace(*q.CreditsBalance) != "" {
		balanceVal = strings.TrimSpace(*q.CreditsBalance)
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE codex_oauth_accounts
SET quota_credits_has_credits=?,
    quota_credits_unlimited=?,
    quota_credits_balance=?,
    quota_primary_used_percent=?,
    quota_primary_reset_at=?,
    quota_secondary_used_percent=?,
    quota_secondary_reset_at=?,
    quota_updated_at=?,
    quota_error=?,
    updated_at=updated_at
WHERE id=?
`, nullableBoolPtr(q.CreditsHasCredits), nullableBoolPtr(q.CreditsUnlimited), balanceVal,
		nullableIntPtr(q.PrimaryUsedPercent), q.PrimaryResetAt,
		nullableIntPtr(q.SecondaryUsedPercent), q.SecondaryResetAt,
		updatedAt, errVal, accountID)
	if err != nil {
		return fmt.Errorf("更新 codex_oauth_account quota 失败: %w", err)
	}
	return nil
}

func (s *Store) SetCodexOAuthAccountCooldown(ctx context.Context, accountID int64, cooldownUntil time.Time) error {
	_, err := s.db.ExecContext(ctx, `
	UPDATE codex_oauth_accounts
	SET cooldown_until=?, updated_at=CURRENT_TIMESTAMP
	WHERE id=?
	`, cooldownUntil, accountID)
	if err != nil {
		return fmt.Errorf("更新 codex_oauth_account cooldown 失败: %w", err)
	}
	return nil
}

func (s *Store) SetCodexOAuthAccountStatus(ctx context.Context, accountID int64, status int) error {
	_, err := s.db.ExecContext(ctx, `
	UPDATE codex_oauth_accounts
	SET status=?, updated_at=CURRENT_TIMESTAMP
	WHERE id=?
	`, status, accountID)
	if err != nil {
		return fmt.Errorf("更新 codex_oauth_account status 失败: %w", err)
	}
	return nil
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullableBoolPtr(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func nullableIntPtr(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func looksLikeLegacyEncryptedBlob(b []byte) bool {
	return len(b) >= 1+12 && b[0] == 1
}

func (s *Store) ReorderUpstreamChannels(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	count := len(ids)
	for i, id := range ids {
		priority := count - i
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_channels SET priority=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, priority, id); err != nil {
			return fmt.Errorf("更新 channel(%d) priority 失败: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteUpstreamChannel(ctx context.Context, channelID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
DELETE FROM openai_compatible_credentials
WHERE endpoint_id IN (SELECT id FROM upstream_endpoints WHERE channel_id=?)
`, channelID); err != nil {
		return fmt.Errorf("删除 openai_compatible_credentials 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM anthropic_credentials
WHERE endpoint_id IN (SELECT id FROM upstream_endpoints WHERE channel_id=?)
`, channelID); err != nil {
		return fmt.Errorf("删除 anthropic_credentials 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM codex_oauth_accounts
WHERE endpoint_id IN (SELECT id FROM upstream_endpoints WHERE channel_id=?)
`, channelID); err != nil {
		return fmt.Errorf("删除 codex_oauth_accounts 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_endpoints WHERE channel_id=?`, channelID); err != nil {
		return fmt.Errorf("删除 upstream_endpoints 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_channels WHERE id=?`, channelID); err != nil {
		return fmt.Errorf("删除 upstream_channels 失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteUpstreamEndpoint(ctx context.Context, endpointID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM openai_compatible_credentials WHERE endpoint_id=?`, endpointID); err != nil {
		return fmt.Errorf("删除 openai_compatible_credentials 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM anthropic_credentials WHERE endpoint_id=?`, endpointID); err != nil {
		return fmt.Errorf("删除 anthropic_credentials 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM codex_oauth_accounts WHERE endpoint_id=?`, endpointID); err != nil {
		return fmt.Errorf("删除 codex_oauth_accounts 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_endpoints WHERE id=?`, endpointID); err != nil {
		return fmt.Errorf("删除 upstream_endpoints 失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func (s *Store) GetOpenAICompatibleCredentialByID(ctx context.Context, credentialID int64) (OpenAICompatibleCredential, error) {
	var c OpenAICompatibleCredential
	row := s.db.QueryRowContext(ctx, `
SELECT id, endpoint_id, name, api_key_enc, api_key_hint, status,
       last_used_at, created_at, updated_at
FROM openai_compatible_credentials
WHERE id=?
`, credentialID)
	err := row.Scan(&c.ID, &c.EndpointID, &c.Name, &c.APIKeyEnc, &c.APIKeyHint, &c.Status,
		&c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OpenAICompatibleCredential{}, sql.ErrNoRows
		}
		return OpenAICompatibleCredential{}, fmt.Errorf("查询 openai_compatible_credential 失败: %w", err)
	}
	return c, nil
}

func (s *Store) DeleteOpenAICompatibleCredential(ctx context.Context, credentialID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM openai_compatible_credentials WHERE id=?`, credentialID)
	if err != nil {
		return fmt.Errorf("删除 openai_compatible_credential 失败: %w", err)
	}
	return nil
}

func (s *Store) ListAnthropicCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]AnthropicCredential, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, endpoint_id, name, api_key_enc, api_key_hint, status,
       last_used_at, created_at, updated_at
FROM anthropic_credentials
WHERE endpoint_id=?
ORDER BY id DESC
`, endpointID)
	if err != nil {
		return nil, fmt.Errorf("查询 anthropic_credentials 失败: %w", err)
	}
	defer rows.Close()

	var out []AnthropicCredential
	for rows.Next() {
		var c AnthropicCredential
		if err := rows.Scan(&c.ID, &c.EndpointID, &c.Name, &c.APIKeyEnc, &c.APIKeyHint, &c.Status,
			&c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 anthropic_credentials 失败: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 anthropic_credentials 失败: %w", err)
	}
	return out, nil
}

type AnthropicCredentialSecret struct {
	ID         int64
	EndpointID int64
	Name       *string
	APIKey     string
	APIKeyHint *string
	Status     int
}

func (s *Store) CreateAnthropicCredential(ctx context.Context, endpointID int64, name *string, apiKey string) (int64, *string, error) {
	enc := []byte(apiKey)
	hint := tokenHint(apiKey)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO anthropic_credentials(endpoint_id, name, api_key_enc, api_key_hint, status, created_at, updated_at)
VALUES(?, ?, ?, ?, 1, NOW(), NOW())
`, endpointID, name, enc, hint)
	if err != nil {
		return 0, nil, fmt.Errorf("创建 anthropic_credential 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, nil, fmt.Errorf("获取 anthropic_credential id 失败: %w", err)
	}
	return id, hint, nil
}

func (s *Store) GetAnthropicCredentialByID(ctx context.Context, credentialID int64) (AnthropicCredential, error) {
	var c AnthropicCredential
	row := s.db.QueryRowContext(ctx, `
SELECT id, endpoint_id, name, api_key_enc, api_key_hint, status,
       last_used_at, created_at, updated_at
FROM anthropic_credentials
WHERE id=?
`, credentialID)
	err := row.Scan(&c.ID, &c.EndpointID, &c.Name, &c.APIKeyEnc, &c.APIKeyHint, &c.Status,
		&c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AnthropicCredential{}, sql.ErrNoRows
		}
		return AnthropicCredential{}, fmt.Errorf("查询 anthropic_credential 失败: %w", err)
	}
	return c, nil
}

func (s *Store) GetAnthropicCredentialSecret(ctx context.Context, credentialID int64) (AnthropicCredentialSecret, error) {
	var c AnthropicCredential
	err := s.db.QueryRowContext(ctx, `
SELECT id, endpoint_id, name, api_key_enc, api_key_hint, status
FROM anthropic_credentials
WHERE id=?
`, credentialID).Scan(&c.ID, &c.EndpointID, &c.Name, &c.APIKeyEnc, &c.APIKeyHint, &c.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AnthropicCredentialSecret{}, sql.ErrNoRows
		}
		return AnthropicCredentialSecret{}, fmt.Errorf("查询 anthropic_credential 失败: %w", err)
	}
	if looksLikeLegacyEncryptedBlob(c.APIKeyEnc) {
		return AnthropicCredentialSecret{}, errors.New("该 credential 为旧版加密格式，当前已禁用应用层加密；请删除并重新录入 api_key")
	}
	plain := c.APIKeyEnc
	return AnthropicCredentialSecret{
		ID:         c.ID,
		EndpointID: c.EndpointID,
		Name:       c.Name,
		APIKey:     string(plain),
		APIKeyHint: c.APIKeyHint,
		Status:     c.Status,
	}, nil
}

func (s *Store) DeleteAnthropicCredential(ctx context.Context, credentialID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM anthropic_credentials WHERE id=?`, credentialID)
	if err != nil {
		return fmt.Errorf("删除 anthropic_credential 失败: %w", err)
	}
	return nil
}

func (s *Store) GetCodexOAuthAccountByID(ctx context.Context, accountID int64) (CodexOAuthAccount, error) {
	var a CodexOAuthAccount
	var idTokenEnc []byte
	row := s.db.QueryRowContext(ctx, `
SELECT id, endpoint_id, account_id, email, access_token_enc, refresh_token_enc, id_token_enc,
       expires_at, last_refresh_at, status,
       cooldown_until, last_used_at,
       balance_total_granted_usd, balance_total_used_usd, balance_total_available_usd,
       balance_updated_at, balance_error,
       quota_credits_has_credits, quota_credits_unlimited, quota_credits_balance,
       quota_primary_used_percent, quota_primary_reset_at,
       quota_secondary_used_percent, quota_secondary_reset_at,
       quota_updated_at, quota_error,
       created_at, updated_at
FROM codex_oauth_accounts
WHERE id=?
`, accountID)
	var balGranted decimal.NullDecimal
	var balUsed decimal.NullDecimal
	var balAvail decimal.NullDecimal
	var balUpdatedAt sql.NullTime
	var balErr sql.NullString
	var quotaHasCredits sql.NullBool
	var quotaUnlimited sql.NullBool
	var quotaBalance sql.NullString
	var quotaPrimaryUsed sql.NullInt64
	var quotaPrimaryResetAt sql.NullTime
	var quotaSecondaryUsed sql.NullInt64
	var quotaSecondaryResetAt sql.NullTime
	var quotaUpdatedAt sql.NullTime
	var quotaErr sql.NullString
	err := row.Scan(&a.ID, &a.EndpointID, &a.AccountID, &a.Email, &a.AccessTokenEnc, &a.RefreshTokenEnc, &idTokenEnc,
		&a.ExpiresAt, &a.LastRefreshAt, &a.Status,
		&a.CooldownUntil, &a.LastUsedAt,
		&balGranted, &balUsed, &balAvail,
		&balUpdatedAt, &balErr,
		&quotaHasCredits, &quotaUnlimited, &quotaBalance,
		&quotaPrimaryUsed, &quotaPrimaryResetAt,
		&quotaSecondaryUsed, &quotaSecondaryResetAt,
		&quotaUpdatedAt, &quotaErr,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CodexOAuthAccount{}, sql.ErrNoRows
		}
		return CodexOAuthAccount{}, fmt.Errorf("查询 codex_oauth_account 失败: %w", err)
	}
	a.IDTokenEnc = idTokenEnc
	if balGranted.Valid {
		v := balGranted.Decimal.Truncate(USDScale)
		a.BalanceTotalGrantedUSD = &v
	}
	if balUsed.Valid {
		v := balUsed.Decimal.Truncate(USDScale)
		a.BalanceTotalUsedUSD = &v
	}
	if balAvail.Valid {
		v := balAvail.Decimal.Truncate(USDScale)
		a.BalanceTotalAvailableUSD = &v
	}
	if balUpdatedAt.Valid {
		t := balUpdatedAt.Time
		a.BalanceUpdatedAt = &t
	}
	if balErr.Valid {
		msg := balErr.String
		if strings.TrimSpace(msg) != "" {
			a.BalanceError = &msg
		}
	}
	if quotaHasCredits.Valid {
		v := quotaHasCredits.Bool
		a.QuotaCreditsHasCredits = &v
	}
	if quotaUnlimited.Valid {
		v := quotaUnlimited.Bool
		a.QuotaCreditsUnlimited = &v
	}
	if quotaBalance.Valid {
		msg := strings.TrimSpace(quotaBalance.String)
		if msg != "" {
			a.QuotaCreditsBalance = &msg
		}
	}
	if quotaPrimaryUsed.Valid {
		v := int(quotaPrimaryUsed.Int64)
		a.QuotaPrimaryUsedPercent = &v
	}
	if quotaPrimaryResetAt.Valid {
		t := quotaPrimaryResetAt.Time
		a.QuotaPrimaryResetAt = &t
	}
	if quotaSecondaryUsed.Valid {
		v := int(quotaSecondaryUsed.Int64)
		a.QuotaSecondaryUsedPercent = &v
	}
	if quotaSecondaryResetAt.Valid {
		t := quotaSecondaryResetAt.Time
		a.QuotaSecondaryResetAt = &t
	}
	if quotaUpdatedAt.Valid {
		t := quotaUpdatedAt.Time
		a.QuotaUpdatedAt = &t
	}
	if quotaErr.Valid {
		msg := strings.TrimSpace(quotaErr.String)
		if msg != "" {
			a.QuotaError = &msg
		}
	}
	return a, nil
}

func (s *Store) DeleteCodexOAuthAccount(ctx context.Context, accountID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM codex_oauth_accounts WHERE id=?`, accountID)
	if err != nil {
		return fmt.Errorf("删除 codex_oauth_account 失败: %w", err)
	}
	return nil
}
