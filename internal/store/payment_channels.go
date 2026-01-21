// Package store 封装支付渠道（payment_channels）的读写：以“渠道”为最小单位，每个渠道绑定类型并拥有独立配置。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	PaymentChannelTypeStripe = "stripe"
	PaymentChannelTypeEPay   = "epay"
)

func normalizePaymentChannelType(typ string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case PaymentChannelTypeStripe:
		return PaymentChannelTypeStripe, nil
	case PaymentChannelTypeEPay:
		return PaymentChannelTypeEPay, nil
	default:
		return "", errors.New("渠道类型不支持")
	}
}

func (s *Store) ListPaymentChannels(ctx context.Context) ([]PaymentChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, type, name, status,
  stripe_currency, stripe_secret_key, stripe_webhook_secret,
  epay_gateway, epay_partner_id, epay_key,
  created_at, updated_at
FROM payment_channels
ORDER BY id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("查询 payment_channels 失败: %w", err)
	}
	defer rows.Close()

	var out []PaymentChannel
	for rows.Next() {
		var ch PaymentChannel
		var stripeCurrency sql.NullString
		var stripeSecretKey sql.NullString
		var stripeWebhookSecret sql.NullString
		var epayGateway sql.NullString
		var epayPartnerID sql.NullString
		var epayKey sql.NullString
		if err := rows.Scan(
			&ch.ID, &ch.Type, &ch.Name, &ch.Status,
			&stripeCurrency, &stripeSecretKey, &stripeWebhookSecret,
			&epayGateway, &epayPartnerID, &epayKey,
			&ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 payment_channels 失败: %w", err)
		}
		if stripeCurrency.Valid {
			v := strings.TrimSpace(stripeCurrency.String)
			if v != "" {
				ch.StripeCurrency = &v
			}
		}
		if stripeSecretKey.Valid {
			v := strings.TrimSpace(stripeSecretKey.String)
			if v != "" {
				ch.StripeSecretKey = &v
			}
		}
		if stripeWebhookSecret.Valid {
			v := strings.TrimSpace(stripeWebhookSecret.String)
			if v != "" {
				ch.StripeWebhookSecret = &v
			}
		}
		if epayGateway.Valid {
			v := strings.TrimSpace(epayGateway.String)
			if v != "" {
				ch.EPayGateway = &v
			}
		}
		if epayPartnerID.Valid {
			v := strings.TrimSpace(epayPartnerID.String)
			if v != "" {
				ch.EPayPartnerID = &v
			}
		}
		if epayKey.Valid {
			v := strings.TrimSpace(epayKey.String)
			if v != "" {
				ch.EPayKey = &v
			}
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 payment_channels 失败: %w", err)
	}
	return out, nil
}

func (s *Store) GetPaymentChannelByID(ctx context.Context, channelID int64) (PaymentChannel, error) {
	if channelID <= 0 {
		return PaymentChannel{}, errors.New("payment_channel_id 不合法")
	}

	var ch PaymentChannel
	var stripeCurrency sql.NullString
	var stripeSecretKey sql.NullString
	var stripeWebhookSecret sql.NullString
	var epayGateway sql.NullString
	var epayPartnerID sql.NullString
	var epayKey sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, type, name, status,
  stripe_currency, stripe_secret_key, stripe_webhook_secret,
  epay_gateway, epay_partner_id, epay_key,
  created_at, updated_at
FROM payment_channels
WHERE id=?
`, channelID).Scan(
		&ch.ID, &ch.Type, &ch.Name, &ch.Status,
		&stripeCurrency, &stripeSecretKey, &stripeWebhookSecret,
		&epayGateway, &epayPartnerID, &epayKey,
		&ch.CreatedAt, &ch.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PaymentChannel{}, sql.ErrNoRows
		}
		return PaymentChannel{}, fmt.Errorf("查询 payment_channel 失败: %w", err)
	}
	if stripeCurrency.Valid {
		v := strings.TrimSpace(stripeCurrency.String)
		if v != "" {
			ch.StripeCurrency = &v
		}
	}
	if stripeSecretKey.Valid {
		v := strings.TrimSpace(stripeSecretKey.String)
		if v != "" {
			ch.StripeSecretKey = &v
		}
	}
	if stripeWebhookSecret.Valid {
		v := strings.TrimSpace(stripeWebhookSecret.String)
		if v != "" {
			ch.StripeWebhookSecret = &v
		}
	}
	if epayGateway.Valid {
		v := strings.TrimSpace(epayGateway.String)
		if v != "" {
			ch.EPayGateway = &v
		}
	}
	if epayPartnerID.Valid {
		v := strings.TrimSpace(epayPartnerID.String)
		if v != "" {
			ch.EPayPartnerID = &v
		}
	}
	if epayKey.Valid {
		v := strings.TrimSpace(epayKey.String)
		if v != "" {
			ch.EPayKey = &v
		}
	}
	return ch, nil
}

type CreatePaymentChannelInput struct {
	Type   string
	Name   string
	Status int

	StripeCurrency      *string
	StripeSecretKey     *string
	StripeWebhookSecret *string

	EPayGateway   *string
	EPayPartnerID *string
	EPayKey       *string
}

func (s *Store) CreatePaymentChannel(ctx context.Context, in CreatePaymentChannelInput) (int64, error) {
	typ, err := normalizePaymentChannelType(in.Type)
	if err != nil {
		return 0, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return 0, errors.New("渠道名称不能为空")
	}
	status := 0
	if in.Status != 0 {
		status = 1
	}

	normOrNil := func(v *string) *string {
		if v == nil {
			return nil
		}
		s := strings.TrimSpace(*v)
		if s == "" {
			return nil
		}
		return &s
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO payment_channels(
  type, name, status,
  stripe_currency, stripe_secret_key, stripe_webhook_secret,
  epay_gateway, epay_partner_id, epay_key,
  created_at, updated_at
) VALUES(?, ?, ?,
  ?, ?, ?,
  ?, ?, ?,
  NOW(), NOW()
)
`, typ, name, status,
		normOrNil(in.StripeCurrency), normOrNil(in.StripeSecretKey), normOrNil(in.StripeWebhookSecret),
		normOrNil(in.EPayGateway), normOrNil(in.EPayPartnerID), normOrNil(in.EPayKey),
	)
	if err != nil {
		return 0, fmt.Errorf("创建 payment_channel 失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 payment_channel id 失败: %w", err)
	}
	return id, nil
}

type UpdatePaymentChannelInput struct {
	ID     int64
	Name   string
	Status int

	StripeCurrency *string
	EPayGateway    *string
	EPayPartnerID  *string

	StripeSecretKey     *string
	StripeWebhookSecret *string
	EPayKey             *string
}

func (s *Store) UpdatePaymentChannel(ctx context.Context, in UpdatePaymentChannelInput) error {
	if in.ID <= 0 {
		return errors.New("payment_channel_id 不合法")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errors.New("渠道名称不能为空")
	}
	status := 0
	if in.Status != 0 {
		status = 1
	}

	normOrNil := func(v *string) *string {
		if v == nil {
			return nil
		}
		s := strings.TrimSpace(*v)
		if s == "" {
			return nil
		}
		return &s
	}

	if _, err := s.db.ExecContext(ctx, `
UPDATE payment_channels
SET name=?, status=?,
  stripe_currency=?,
  stripe_secret_key=COALESCE(?, stripe_secret_key),
  stripe_webhook_secret=COALESCE(?, stripe_webhook_secret),
  epay_gateway=?,
  epay_partner_id=?,
  epay_key=COALESCE(?, epay_key),
  updated_at=NOW()
WHERE id=?
`, name, status,
		normOrNil(in.StripeCurrency),
		normOrNil(in.StripeSecretKey),
		normOrNil(in.StripeWebhookSecret),
		normOrNil(in.EPayGateway),
		normOrNil(in.EPayPartnerID),
		normOrNil(in.EPayKey),
		in.ID,
	); err != nil {
		return fmt.Errorf("更新 payment_channel 失败: %w", err)
	}
	return nil
}

func (s *Store) DeletePaymentChannel(ctx context.Context, channelID int64) error {
	if channelID <= 0 {
		return errors.New("payment_channel_id 不合法")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM payment_channels WHERE id=?`, channelID); err != nil {
		return fmt.Errorf("删除 payment_channel 失败: %w", err)
	}
	return nil
}
