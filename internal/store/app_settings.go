// Package store 提供应用级配置的读写封装，支持通过 UI 持久化少量运行期开关。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

const SettingEmailVerificationEnable = "email_verification_enable"

const (
	SettingFeatureDisableWebAnnouncements = "feature_disable_web_announcements"
	SettingFeatureDisableWebTokens        = "feature_disable_web_tokens"
	SettingFeatureDisableWebUsage         = "feature_disable_web_usage"

	SettingFeatureDisableModels = "feature_disable_models"

	SettingFeatureDisableBilling = "feature_disable_billing"
	SettingFeatureDisableTickets = "feature_disable_tickets"

	SettingFeatureDisableAdminChannels      = "feature_disable_admin_channels"
	SettingFeatureDisableAdminChannelGroups = "feature_disable_admin_channel_groups"
	SettingFeatureDisableAdminUsers         = "feature_disable_admin_users"
	SettingFeatureDisableAdminUsage         = "feature_disable_admin_usage"
	SettingFeatureDisableAdminAnnouncements = "feature_disable_admin_announcements"
)

// SettingSiteBaseURL 用于生成页面展示/回调/回跳链接等“对外可访问”的站点基础地址（形如 https://realms.example.com）。
// 为空表示不覆盖配置文件与请求推断逻辑。
const SettingSiteBaseURL = "site_base_url"

// SettingAdminTimeZone 用于管理后台展示时间的时区（IANA TZ database name，如 Asia/Shanghai）。
// 为空表示使用系统默认（当前默认为 Asia/Shanghai）。
const SettingAdminTimeZone = "admin_time_zone"

const (
	SettingSMTPServer     = "smtp_server"
	SettingSMTPPort       = "smtp_port"
	SettingSMTPSSLEnabled = "smtp_ssl_enabled"
	SettingSMTPAccount    = "smtp_account"
	SettingSMTPFrom       = "smtp_from"
	SettingSMTPToken      = "smtp_token"
)

const (
	SettingBillingEnablePayAsYouGo = "billing_enable_pay_as_you_go"
	SettingBillingMinTopupCNY      = "billing_min_topup_cny"
	SettingBillingCreditUSDPerCNY  = "billing_credit_usd_per_cny"
)

const (
	SettingPaymentEPayEnable    = "payment_epay_enable"
	SettingPaymentEPayGateway   = "payment_epay_gateway"
	SettingPaymentEPayPartnerID = "payment_epay_partner_id"
	SettingPaymentEPayKey       = "payment_epay_key"
)

const (
	SettingPaymentStripeEnable        = "payment_stripe_enable"
	SettingPaymentStripeSecretKey     = "payment_stripe_secret_key"
	SettingPaymentStripeWebhookSecret = "payment_stripe_webhook_secret"
	SettingPaymentStripeCurrency      = "payment_stripe_currency"
)

func (s *Store) GetAppSetting(ctx context.Context, key string) (string, bool, error) {
	if s.db == nil {
		return "", false, nil
	}
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE `key`=?", key).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("查询 app_settings 失败: %w", err)
	}
	return v, true, nil
}

func (s *Store) UpsertAppSetting(ctx context.Context, key string, value string) error {
	if s.db == nil {
		return errors.New("db 为空")
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO app_settings(`key`, value, created_at, updated_at)\n"+
			"VALUES(?, ?, NOW(), NOW())\n"+
			"ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=NOW()",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("写入 app_settings 失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteAppSetting(ctx context.Context, key string) error {
	if s.db == nil {
		return errors.New("db 为空")
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM app_settings WHERE `key`=?", key)
	if err != nil {
		return fmt.Errorf("删除 app_settings 失败: %w", err)
	}
	return nil
}

func (s *Store) DeleteAppSettings(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if s.db == nil {
		return errors.New("db 为空")
	}
	var b strings.Builder
	b.WriteString("DELETE FROM app_settings WHERE `key` IN (")
	for i := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
	}
	b.WriteString(")")
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		args = append(args, k)
	}
	if _, err := s.db.ExecContext(ctx, b.String(), args...); err != nil {
		return fmt.Errorf("批量删除 app_settings 失败: %w", err)
	}
	return nil
}

func (s *Store) GetAppSettings(ctx context.Context, keys ...string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	if s.db == nil {
		return out, nil
	}
	var b strings.Builder
	b.WriteString("SELECT `key`, value FROM app_settings WHERE `key` IN (")
	for i := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
	}
	b.WriteString(")")
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		args = append(args, k)
	}
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("查询 app_settings 失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("读取 app_settings 失败: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 app_settings 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetStringAppSetting(ctx context.Context, key string) (string, bool, error) {
	raw, ok, err := s.GetAppSetting(ctx, key)
	if err != nil || !ok {
		return "", ok, err
	}
	return strings.TrimSpace(raw), true, nil
}

func (s *Store) UpsertStringAppSetting(ctx context.Context, key string, value string) error {
	return s.UpsertAppSetting(ctx, key, strings.TrimSpace(value))
}

func (s *Store) GetBoolAppSetting(ctx context.Context, key string) (bool, bool, error) {
	raw, ok, err := s.GetAppSetting(ctx, key)
	if err != nil || !ok {
		return false, ok, err
	}
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, true, fmt.Errorf("解析 app_settings[%s] 失败: %w", key, err)
	}
	return b, true, nil
}

func (s *Store) UpsertBoolAppSetting(ctx context.Context, key string, value bool) error {
	return s.UpsertAppSetting(ctx, key, strconv.FormatBool(value))
}

func (s *Store) GetIntAppSetting(ctx context.Context, key string) (int, bool, error) {
	raw, ok, err := s.GetAppSetting(ctx, key)
	if err != nil || !ok {
		return 0, ok, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, true, fmt.Errorf("解析 app_settings[%s] 失败: %w", key, err)
	}
	return n, true, nil
}

func (s *Store) UpsertIntAppSetting(ctx context.Context, key string, value int) error {
	return s.UpsertAppSetting(ctx, key, strconv.Itoa(value))
}

func (s *Store) GetInt64AppSetting(ctx context.Context, key string) (int64, bool, error) {
	raw, ok, err := s.GetAppSetting(ctx, key)
	if err != nil || !ok {
		return 0, ok, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, true, fmt.Errorf("解析 app_settings[%s] 失败: %w", key, err)
	}
	return n, true, nil
}

func (s *Store) UpsertInt64AppSetting(ctx context.Context, key string, value int64) error {
	return s.UpsertAppSetting(ctx, key, strconv.FormatInt(value, 10))
}

func (s *Store) GetDecimalAppSetting(ctx context.Context, key string) (decimal.Decimal, bool, error) {
	raw, ok, err := s.GetAppSetting(ctx, key)
	if err != nil || !ok {
		return decimal.Zero, ok, err
	}
	d, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return decimal.Zero, true, fmt.Errorf("解析 app_settings[%s] 失败: %w", key, err)
	}
	return d, true, nil
}

func (s *Store) UpsertDecimalAppSetting(ctx context.Context, key string, value decimal.Decimal) error {
	return s.UpsertAppSetting(ctx, key, value.String())
}
