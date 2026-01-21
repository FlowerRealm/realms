package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	stripeWebhook "github.com/stripe/stripe-go/v81/webhook"

	"realms/internal/config"
	"realms/internal/middleware"
	"realms/internal/store"
)

func (a *App) paymentConfigEffective(ctx context.Context) config.PaymentConfig {
	cfg := a.cfg.Payment

	if v, ok, err := a.store.GetBoolAppSetting(ctx, store.SettingPaymentEPayEnable); err == nil && ok {
		cfg.EPay.Enable = v
	}
	if v, ok, err := a.store.GetStringAppSetting(ctx, store.SettingPaymentEPayGateway); err == nil && ok {
		cfg.EPay.Gateway = v
	}
	if v, ok, err := a.store.GetStringAppSetting(ctx, store.SettingPaymentEPayPartnerID); err == nil && ok {
		cfg.EPay.PartnerID = v
	}
	if v, ok, err := a.store.GetStringAppSetting(ctx, store.SettingPaymentEPayKey); err == nil && ok {
		cfg.EPay.Key = v
	}

	if v, ok, err := a.store.GetBoolAppSetting(ctx, store.SettingPaymentStripeEnable); err == nil && ok {
		cfg.Stripe.Enable = v
	}
	if v, ok, err := a.store.GetStringAppSetting(ctx, store.SettingPaymentStripeCurrency); err == nil && ok {
		cfg.Stripe.Currency = v
	}
	if v, ok, err := a.store.GetStringAppSetting(ctx, store.SettingPaymentStripeSecretKey); err == nil && ok {
		cfg.Stripe.SecretKey = v
	}
	if v, ok, err := a.store.GetStringAppSetting(ctx, store.SettingPaymentStripeWebhookSecret); err == nil && ok {
		cfg.Stripe.WebhookSecret = v
	}
	cfg.Stripe.Currency = strings.ToLower(strings.TrimSpace(cfg.Stripe.Currency))
	if cfg.Stripe.Currency == "" {
		cfg.Stripe.Currency = "cny"
	}
	return cfg
}

func parsePayOrderRef(ref string) (kind string, orderID int64, ok bool) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "sub_") {
		kind = "subscription"
		ref = strings.TrimPrefix(ref, "sub_")
	} else if strings.HasPrefix(ref, "topup_") {
		kind = "topup"
		ref = strings.TrimPrefix(ref, "topup_")
	} else {
		return "", 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(ref), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	return kind, id, true
}

func parseCNY(raw string) (decimal.Decimal, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "¥")
	if s == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() {
		return decimal.Zero, false
	}
	if d.Exponent() < -store.CNYScale {
		return decimal.Zero, false
	}
	return d.Truncate(store.CNYScale), true
}

func cnyToMinorUnits(cny decimal.Decimal) (int64, bool) {
	if cny.IsNegative() {
		return 0, false
	}
	if cny.Exponent() < -store.CNYScale {
		return 0, false
	}
	scaled := cny.Truncate(store.CNYScale).Shift(store.CNYScale)
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, false
	}
	n := scaled.IntPart()
	if !decimal.NewFromInt(n).Equal(scaled) {
		return 0, false
	}
	return n, true
}

func (a *App) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := a.paymentConfigEffective(r.Context()).Stripe
	if !cfg.Enable || strings.TrimSpace(cfg.WebhookSecret) == "" {
		http.NotFound(w, r)
		return
	}

	payload := middleware.CachedBody(r.Context())
	if len(payload) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("Stripe-Signature")
	event, err := stripeWebhook.ConstructEventWithOptions(payload, signature, cfg.WebhookSecret, stripeWebhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		http.Error(w, "验签失败", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		ref := strings.TrimSpace(event.GetObjectValue("client_reference_id"))
		status := strings.TrimSpace(event.GetObjectValue("status"))
		if status != "complete" {
			break
		}
		kind, orderID, ok := parsePayOrderRef(ref)
		if !ok {
			break
		}
		amountTotalRaw := strings.TrimSpace(event.GetObjectValue("amount_total"))
		amountTotal, err := strconv.ParseInt(amountTotalRaw, 10, 64)
		if err != nil || amountTotal <= 0 || amountTotal > int64(^uint(0)>>1) {
			break
		}
		currency := strings.ToLower(strings.TrimSpace(event.GetObjectValue("currency")))
		if currency != "" && currency != cfg.Currency {
			break
		}

		paidMethod := "stripe"
		paidRef := strings.TrimSpace(event.GetObjectValue("id")) // Checkout Session ID
		var paidRefPtr *string
		if paidRef != "" {
			paidRefPtr = &paidRef
		}

		switch kind {
		case "subscription":
			o, err := a.store.GetSubscriptionOrderByID(r.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
			expected, ok := cnyToMinorUnits(o.AmountCNY)
			if !ok || expected != amountTotal {
				break
			}
			if _, _, err := a.store.MarkSubscriptionOrderPaidAndActivate(r.Context(), orderID, time.Now(), &paidMethod, paidRefPtr, nil); err != nil {
				if errors.Is(err, store.ErrOrderCanceled) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
		case "topup":
			o, err := a.store.GetTopupOrderByID(r.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
			expected, ok := cnyToMinorUnits(o.AmountCNY)
			if !ok || expected != amountTotal {
				break
			}
			if err := a.store.MarkTopupOrderPaid(r.Context(), orderID, &paidMethod, paidRefPtr, nil, time.Now()); err != nil {
				if errors.Is(err, store.ErrOrderCanceled) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleStripeWebhookByPaymentChannel(w http.ResponseWriter, r *http.Request) {
	channelIDRaw := strings.TrimSpace(r.PathValue("payment_channel_id"))
	channelID, err := strconv.ParseInt(channelIDRaw, 10, 64)
	if err != nil || channelID <= 0 {
		http.NotFound(w, r)
		return
	}

	ch, err := a.store.GetPaymentChannelByID(r.Context(), channelID)
	if err != nil || ch.Status != 1 || ch.Type != store.PaymentChannelTypeStripe {
		http.NotFound(w, r)
		return
	}
	if ch.StripeWebhookSecret == nil || strings.TrimSpace(*ch.StripeWebhookSecret) == "" {
		http.NotFound(w, r)
		return
	}

	currency := "cny"
	if ch.StripeCurrency != nil {
		currency = strings.ToLower(strings.TrimSpace(*ch.StripeCurrency))
	}
	if currency == "" {
		currency = "cny"
	}

	payload := middleware.CachedBody(r.Context())
	if len(payload) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("Stripe-Signature")
	event, err := stripeWebhook.ConstructEventWithOptions(payload, signature, strings.TrimSpace(*ch.StripeWebhookSecret), stripeWebhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		http.Error(w, "验签失败", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		ref := strings.TrimSpace(event.GetObjectValue("client_reference_id"))
		status := strings.TrimSpace(event.GetObjectValue("status"))
		if status != "complete" {
			break
		}
		kind, orderID, ok := parsePayOrderRef(ref)
		if !ok {
			break
		}
		amountTotalRaw := strings.TrimSpace(event.GetObjectValue("amount_total"))
		amountTotal, err := strconv.ParseInt(amountTotalRaw, 10, 64)
		if err != nil || amountTotal <= 0 || amountTotal > int64(^uint(0)>>1) {
			break
		}
		eventCurrency := strings.ToLower(strings.TrimSpace(event.GetObjectValue("currency")))
		if eventCurrency != "" && eventCurrency != currency {
			break
		}

		paidMethod := "stripe"
		paidRef := strings.TrimSpace(event.GetObjectValue("id")) // Checkout Session ID
		var paidRefPtr *string
		if paidRef != "" {
			paidRefPtr = &paidRef
		}
		paidChannelID := channelID

		switch kind {
		case "subscription":
			o, err := a.store.GetSubscriptionOrderByID(r.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
			expected, ok := cnyToMinorUnits(o.AmountCNY)
			if !ok || expected != amountTotal {
				break
			}
			if _, _, err := a.store.MarkSubscriptionOrderPaidAndActivate(r.Context(), orderID, time.Now(), &paidMethod, paidRefPtr, &paidChannelID); err != nil {
				if errors.Is(err, store.ErrOrderCanceled) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
		case "topup":
			o, err := a.store.GetTopupOrderByID(r.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
			expected, ok := cnyToMinorUnits(o.AmountCNY)
			if !ok || expected != amountTotal {
				break
			}
			if err := a.store.MarkTopupOrderPaid(r.Context(), orderID, &paidMethod, paidRefPtr, &paidChannelID, time.Now()); err != nil {
				if errors.Is(err, store.ErrOrderCanceled) {
					break
				}
				http.Error(w, "处理失败", http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleEPayNotify(w http.ResponseWriter, r *http.Request) {
	cfg := a.paymentConfigEffective(r.Context()).EPay
	if !cfg.Enable || strings.TrimSpace(cfg.Gateway) == "" || strings.TrimSpace(cfg.PartnerID) == "" || strings.TrimSpace(cfg.Key) == "" {
		http.NotFound(w, r)
		return
	}

	client, err := epay.NewClient(&epay.Config{
		PartnerID: cfg.PartnerID,
		Key:       cfg.Key,
	}, cfg.Gateway)
	if err != nil {
		http.Error(w, "配置错误", http.StatusInternalServerError)
		return
	}

	params := make(map[string]string)
	q := r.URL.Query()
	for k := range q {
		params[k] = q.Get(k)
	}

	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		_, _ = w.Write([]byte("fail"))
		return
	}
	_, _ = w.Write([]byte("success"))

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		return
	}

	kind, orderID, ok := parsePayOrderRef(verifyInfo.ServiceTradeNo)
	if !ok {
		return
	}

	paidMethod := "epay"
	paidRef := strings.TrimSpace(verifyInfo.TradeNo)
	var paidRefPtr *string
	if paidRef != "" {
		paidRefPtr = &paidRef
	}

	paidAt := time.Now()
	paidCNY, ok := parseCNY(verifyInfo.Money)
	if !ok || paidCNY.LessThanOrEqual(decimal.Zero) {
		return
	}

	switch kind {
	case "subscription":
		o, err := a.store.GetSubscriptionOrderByID(r.Context(), orderID)
		if err != nil {
			return
		}
		if !paidCNY.Equal(o.AmountCNY.Truncate(store.CNYScale)) {
			return
		}
		_, _, _ = a.store.MarkSubscriptionOrderPaidAndActivate(r.Context(), orderID, paidAt, &paidMethod, paidRefPtr, nil)
	case "topup":
		o, err := a.store.GetTopupOrderByID(r.Context(), orderID)
		if err != nil {
			return
		}
		if !paidCNY.Equal(o.AmountCNY.Truncate(store.CNYScale)) {
			return
		}
		_ = a.store.MarkTopupOrderPaid(r.Context(), orderID, &paidMethod, paidRefPtr, nil, paidAt)
	}
}

func (a *App) handleEPayNotifyByPaymentChannel(w http.ResponseWriter, r *http.Request) {
	channelIDRaw := strings.TrimSpace(r.PathValue("payment_channel_id"))
	channelID, err := strconv.ParseInt(channelIDRaw, 10, 64)
	if err != nil || channelID <= 0 {
		http.NotFound(w, r)
		return
	}

	ch, err := a.store.GetPaymentChannelByID(r.Context(), channelID)
	if err != nil || ch.Status != 1 || ch.Type != store.PaymentChannelTypeEPay {
		http.NotFound(w, r)
		return
	}
	if ch.EPayGateway == nil || strings.TrimSpace(*ch.EPayGateway) == "" || ch.EPayPartnerID == nil || strings.TrimSpace(*ch.EPayPartnerID) == "" || ch.EPayKey == nil || strings.TrimSpace(*ch.EPayKey) == "" {
		http.NotFound(w, r)
		return
	}

	client, err := epay.NewClient(&epay.Config{
		PartnerID: strings.TrimSpace(*ch.EPayPartnerID),
		Key:       strings.TrimSpace(*ch.EPayKey),
	}, strings.TrimSpace(*ch.EPayGateway))
	if err != nil {
		http.Error(w, "配置错误", http.StatusInternalServerError)
		return
	}

	params := make(map[string]string)
	q := r.URL.Query()
	for k := range q {
		params[k] = q.Get(k)
	}

	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		_, _ = w.Write([]byte("fail"))
		return
	}
	_, _ = w.Write([]byte("success"))

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		return
	}

	kind, orderID, ok := parsePayOrderRef(verifyInfo.ServiceTradeNo)
	if !ok {
		return
	}

	paidMethod := "epay"
	paidRef := strings.TrimSpace(verifyInfo.TradeNo)
	var paidRefPtr *string
	if paidRef != "" {
		paidRefPtr = &paidRef
	}
	paidChannelID := channelID

	paidAt := time.Now()
	paidCNY, ok := parseCNY(verifyInfo.Money)
	if !ok || paidCNY.LessThanOrEqual(decimal.Zero) {
		return
	}

	switch kind {
	case "subscription":
		o, err := a.store.GetSubscriptionOrderByID(r.Context(), orderID)
		if err != nil {
			return
		}
		if !paidCNY.Equal(o.AmountCNY.Truncate(store.CNYScale)) {
			return
		}
		_, _, _ = a.store.MarkSubscriptionOrderPaidAndActivate(r.Context(), orderID, paidAt, &paidMethod, paidRefPtr, &paidChannelID)
	case "topup":
		o, err := a.store.GetTopupOrderByID(r.Context(), orderID)
		if err != nil {
			return
		}
		if !paidCNY.Equal(o.AmountCNY.Truncate(store.CNYScale)) {
			return
		}
		_ = a.store.MarkTopupOrderPaid(r.Context(), orderID, &paidMethod, paidRefPtr, &paidChannelID, paidAt)
	}
}
