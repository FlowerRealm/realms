package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func defaultAPISettingsForChannelType(channelType string) (chatEnabled bool, responsesEnabled bool) {
	switch strings.TrimSpace(channelType) {
	case UpstreamTypeOpenAICompatible:
		return true, true
	case UpstreamTypeCodexOAuth:
		return false, true
	default:
		return false, false
	}
}

func normalizeUpstreamChannelSetting(channelType string, setting UpstreamChannelSetting) (UpstreamChannelSetting, error) {
	setting.Proxy = strings.TrimSpace(setting.Proxy)
	setting.SystemPrompt = strings.TrimSpace(setting.SystemPrompt)
	setting.CacheTTLPreference = strings.ToLower(strings.TrimSpace(setting.CacheTTLPreference))
	switch setting.CacheTTLPreference {
	case "", "inherit", "5m", "1h":
	default:
		setting.CacheTTLPreference = ""
	}
	if strings.TrimSpace(channelType) == UpstreamTypeCodexOAuth && setting.ChatCompletionsEnabled {
		return setting, errors.New("codex_oauth 渠道不支持 chat/completions")
	}
	if !setting.ChatCompletionsEnabled && !setting.ResponsesEnabled {
		return setting, errors.New("至少启用一个接口能力")
	}
	return setting, nil
}

func applyDefaultAPISettingsForChannelType(channelType string, setting UpstreamChannelSetting) UpstreamChannelSetting {
	if setting.ChatCompletionsEnabled || setting.ResponsesEnabled {
		return setting
	}
	chatDefault, responsesDefault := defaultAPISettingsForChannelType(channelType)
	setting.ChatCompletionsEnabled = chatDefault
	setting.ResponsesEnabled = responsesDefault
	return setting
}

func (s *Store) UpdateUpstreamChannelNewAPISetting(ctx context.Context, channelID int64, setting UpstreamChannelSetting) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	ch, err := s.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		return fmt.Errorf("查询 upstream_channel 失败: %w", err)
	}
	setting, err = normalizeUpstreamChannelSetting(ch.Type, setting)
	if err != nil {
		return err
	}

	b, err := json.Marshal(setting)
	if err != nil {
		return fmt.Errorf("setting 序列化失败: %w", err)
	}
	out := strings.TrimSpace(string(b))
	var v any
	if out != "" && out != "{}" {
		v = out
	}

	if _, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET setting=?, updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, v, channelID); err != nil {
		return fmt.Errorf("更新 upstream_channel setting 失败: %w", err)
	}
	return nil
}

func (s *Store) UpdateUpstreamChannelNewAPIMeta(ctx context.Context, channelID int64, openAIOrganization, testModel, tag, remark *string, weight int, autoBan bool) error {
	if channelID == 0 {
		return errors.New("channelID 不能为空")
	}

	openAIOrganization = trimNullableString(openAIOrganization)
	testModel = trimNullableString(testModel)
	tag = trimNullableString(tag)
	remark = trimNullableString(remark)

	if weight < 0 {
		return errors.New("weight 不能为负数")
	}
	autoBanInt := 0
	if autoBan {
		autoBanInt = 1
	}

	if _, err := s.db.ExecContext(ctx, `
UPDATE upstream_channels
SET openai_organization=?,
    test_model=?,
    tag=?,
    remark=?,
    weight=?,
    auto_ban=?,
    updated_at=CURRENT_TIMESTAMP
WHERE id=?
`, nullableString(openAIOrganization), nullableString(testModel), nullableString(tag), nullableString(remark), weight, autoBanInt, channelID); err != nil {
		return fmt.Errorf("更新 upstream_channel meta 失败: %w", err)
	}
	return nil
}

func trimNullableString(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}
