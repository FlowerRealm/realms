package store

import (
	"context"
)

// FeatureDisabledEffective 返回某个 feature_disable_* 的最终禁用状态（包含 app_settings_defaults 与 self_mode 硬禁用）。
//
// 约定：
// - 运行期配置（app_settings）优先于配置文件默认（app_settings_defaults）
// - self_mode 会强制禁用计费与工单
// - 当读取数据库失败时，回退到配置文件默认值（不因 DB 故障放大影响）
func (s *Store) FeatureDisabledEffective(ctx context.Context, selfMode bool, key string) bool {
	if s == nil {
		return false
	}
	switch key {
	case SettingFeatureDisableBilling, SettingFeatureDisableTickets:
		if selfMode {
			return true
		}
	}

	disabled := false
	if s.hasAppSettingsDefaults {
		fs := s.appSettingsDefaults
		switch key {
		case SettingFeatureDisableWebAnnouncements:
			disabled = fs.FeatureDisableWebAnnouncements
		case SettingFeatureDisableWebTokens:
			disabled = fs.FeatureDisableWebTokens
		case SettingFeatureDisableWebUsage:
			disabled = fs.FeatureDisableWebUsage
		case SettingFeatureDisableModels:
			disabled = fs.FeatureDisableModels
		case SettingFeatureDisableBilling:
			disabled = fs.FeatureDisableBilling
		case SettingFeatureDisableTickets:
			disabled = fs.FeatureDisableTickets
		case SettingFeatureDisableAdminChannels:
			disabled = fs.FeatureDisableAdminChannels
		case SettingFeatureDisableAdminChannelGroups:
			disabled = fs.FeatureDisableAdminChannelGroups
		case SettingFeatureDisableAdminUsers:
			disabled = fs.FeatureDisableAdminUsers
		case SettingFeatureDisableAdminUsage:
			disabled = fs.FeatureDisableAdminUsage
		case SettingFeatureDisableAdminAnnouncements:
			disabled = fs.FeatureDisableAdminAnnouncements
		}
	}

	v, ok, err := s.GetBoolAppSetting(ctx, key)
	if err == nil && ok {
		disabled = v
	}
	return disabled
}
