package router

import (
	"context"
	"net/http"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/config"
	"realms/internal/security"
	"realms/internal/store"
)

func billingConfigEffective(ctx context.Context, opts Options) config.BillingConfig {
	cfg := opts.BillingDefault
	if opts.Store == nil {
		return cfg
	}

	if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo); err == nil && ok {
		cfg.EnablePayAsYouGo = v
	}
	if v, ok, err := opts.Store.GetDecimalAppSetting(ctx, store.SettingBillingMinTopupCNY); err == nil && ok {
		cfg.MinTopupCNY = v
	}
	if v, ok, err := opts.Store.GetDecimalAppSetting(ctx, store.SettingBillingCreditUSDPerCNY); err == nil && ok {
		cfg.CreditUSDPerCNY = v
	}

	if cfg.MinTopupCNY.IsNegative() {
		cfg.MinTopupCNY = decimal.Zero
	}
	if cfg.CreditUSDPerCNY.IsNegative() {
		cfg.CreditUSDPerCNY = decimal.Zero
	}

	cfg.MinTopupCNY = cfg.MinTopupCNY.Truncate(store.CNYScale)
	cfg.CreditUSDPerCNY = cfg.CreditUSDPerCNY.Truncate(store.USDScale)
	return cfg
}

func paymentConfigEffective(ctx context.Context, opts Options) config.PaymentConfig {
	cfg := opts.PaymentDefault
	if opts.Store == nil {
		return cfg
	}

	if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingPaymentStripeEnable); err == nil && ok {
		cfg.Stripe.Enable = v
	}
	if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingPaymentStripeSecretKey); err == nil && ok {
		cfg.Stripe.SecretKey = v
	}
	if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingPaymentStripeWebhookSecret); err == nil && ok {
		cfg.Stripe.WebhookSecret = v
	}
	if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingPaymentStripeCurrency); err == nil && ok {
		cfg.Stripe.Currency = v
	}

	if v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingPaymentEPayEnable); err == nil && ok {
		cfg.EPay.Enable = v
	}
	if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingPaymentEPayGateway); err == nil && ok {
		cfg.EPay.Gateway = v
	}
	if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingPaymentEPayPartnerID); err == nil && ok {
		cfg.EPay.PartnerID = v
	}
	if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingPaymentEPayKey); err == nil && ok {
		cfg.EPay.Key = v
	}

	cfg.Stripe.Currency = strings.ToLower(strings.TrimSpace(cfg.Stripe.Currency))
	if cfg.Stripe.Currency == "" {
		cfg.Stripe.Currency = "cny"
	}
	cfg.EPay.Gateway = strings.TrimSpace(cfg.EPay.Gateway)
	cfg.EPay.PartnerID = strings.TrimSpace(cfg.EPay.PartnerID)
	cfg.EPay.Key = strings.TrimSpace(cfg.EPay.Key)
	cfg.Stripe.SecretKey = strings.TrimSpace(cfg.Stripe.SecretKey)
	cfg.Stripe.WebhookSecret = strings.TrimSpace(cfg.Stripe.WebhookSecret)
	return cfg
}

func uiBaseURLFromRequest(ctx context.Context, opts Options, r *http.Request) string {
	if strings.TrimSpace(opts.FrontendBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(opts.FrontendBaseURL), "/")
	}
	if opts.Store != nil {
		if v, ok, err := opts.Store.GetStringAppSetting(ctx, store.SettingSiteBaseURL); err == nil && ok {
			if normalized, err := config.NormalizeHTTPBaseURL(v, "site_base_url"); err == nil && normalized != "" {
				return normalized
			}
		}
	}
	return security.DeriveBaseURLFromRequest(r, false, nil)
}
