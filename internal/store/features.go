// Package store 提供功能禁用（Feature Bans）的统一读取与运行时判定。
package store

import (
	"context"
	"strconv"
	"strings"
)

// FeatureState 表示“对外可见”的功能开关最终状态。
// 说明：
// - *_Disabled=true 表示该功能应隐藏入口并拒绝访问。
// - 该状态会合并自用模式（self_mode）的硬禁用。
type FeatureState struct {
	WebAnnouncementsDisabled bool
	WebTokensDisabled        bool
	WebUsageDisabled         bool

	ModelsDisabled bool

	BillingDisabled bool
	TicketsDisabled bool

	AdminChannelsDisabled      bool
	AdminChannelGroupsDisabled bool
	AdminUsersDisabled         bool
	AdminUsageDisabled         bool
	AdminAnnouncementsDisabled bool
}

func (s *Store) FeatureStateEffective(ctx context.Context, selfMode bool) FeatureState {
	defaultB := func(key string) bool {
		if !s.hasAppSettingsDefaults {
			return false
		}
		d := s.appSettingsDefaults
		switch key {
		case SettingFeatureDisableWebAnnouncements:
			return d.FeatureDisableWebAnnouncements
		case SettingFeatureDisableWebTokens:
			return d.FeatureDisableWebTokens
		case SettingFeatureDisableWebUsage:
			return d.FeatureDisableWebUsage
		case SettingFeatureDisableModels:
			return d.FeatureDisableModels
		case SettingFeatureDisableBilling:
			return d.FeatureDisableBilling
		case SettingFeatureDisableTickets:
			return d.FeatureDisableTickets
		case SettingFeatureDisableAdminChannels:
			return d.FeatureDisableAdminChannels
		case SettingFeatureDisableAdminChannelGroups:
			return d.FeatureDisableAdminChannelGroups
		case SettingFeatureDisableAdminUsers:
			return d.FeatureDisableAdminUsers
		case SettingFeatureDisableAdminUsage:
			return d.FeatureDisableAdminUsage
		case SettingFeatureDisableAdminAnnouncements:
			return d.FeatureDisableAdminAnnouncements
		default:
			return false
		}
	}

	out := FeatureState{
		WebAnnouncementsDisabled: defaultB(SettingFeatureDisableWebAnnouncements),
		WebTokensDisabled:        defaultB(SettingFeatureDisableWebTokens),
		WebUsageDisabled:         defaultB(SettingFeatureDisableWebUsage),

		ModelsDisabled: defaultB(SettingFeatureDisableModels),

		BillingDisabled: selfMode || defaultB(SettingFeatureDisableBilling),
		TicketsDisabled: selfMode || defaultB(SettingFeatureDisableTickets),

		AdminChannelsDisabled:      defaultB(SettingFeatureDisableAdminChannels),
		AdminChannelGroupsDisabled: defaultB(SettingFeatureDisableAdminChannelGroups),
		AdminUsersDisabled:         defaultB(SettingFeatureDisableAdminUsers),
		AdminUsageDisabled:         defaultB(SettingFeatureDisableAdminUsage),
		AdminAnnouncementsDisabled: defaultB(SettingFeatureDisableAdminAnnouncements),
	}

	keys := []string{
		SettingFeatureDisableWebAnnouncements,
		SettingFeatureDisableWebTokens,
		SettingFeatureDisableWebUsage,
		SettingFeatureDisableModels,
		SettingFeatureDisableBilling,
		SettingFeatureDisableTickets,
		SettingFeatureDisableAdminChannels,
		SettingFeatureDisableAdminChannelGroups,
		SettingFeatureDisableAdminUsers,
		SettingFeatureDisableAdminUsage,
		SettingFeatureDisableAdminAnnouncements,
	}

	m, err := s.GetAppSettings(ctx, keys...)
	if err != nil {
		return out
	}

	parseBool := func(key string) (bool, bool) {
		raw, ok := m[key]
		if !ok {
			return false, false
		}
		v, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return false, false
		}
		return v, true
	}

	if v, ok := parseBool(SettingFeatureDisableWebAnnouncements); ok {
		out.WebAnnouncementsDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableWebTokens); ok {
		out.WebTokensDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableWebUsage); ok {
		out.WebUsageDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableModels); ok {
		out.ModelsDisabled = v
	}

	if v, ok := parseBool(SettingFeatureDisableBilling); ok {
		out.BillingDisabled = selfMode || v
	}
	if v, ok := parseBool(SettingFeatureDisableTickets); ok {
		out.TicketsDisabled = selfMode || v
	}

	if v, ok := parseBool(SettingFeatureDisableAdminChannels); ok {
		out.AdminChannelsDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableAdminChannelGroups); ok {
		out.AdminChannelGroupsDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableAdminUsers); ok {
		out.AdminUsersDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableAdminUsage); ok {
		out.AdminUsageDisabled = v
	}
	if v, ok := parseBool(SettingFeatureDisableAdminAnnouncements); ok {
		out.AdminAnnouncementsDisabled = v
	}

	return out
}
