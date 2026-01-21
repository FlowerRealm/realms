// payment_channels.go 提供支付渠道（按渠道配置 EPay/Stripe）的管理后台页面与操作入口。
package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"realms/internal/store"
)

type paymentChannelView struct {
	ID        int64
	Type      string
	TypeLabel string
	Name      string
	Status    int

	StripeCurrency         string
	StripeSecretKeySet     bool
	StripeWebhookSecretSet bool

	EPayGateway   string
	EPayPartnerID string
	EPayKeySet    bool

	WebhookURL string
	Usable     bool

	CreatedAt string
	UpdatedAt string
}

func paymentChannelTypeLabel(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case store.PaymentChannelTypeStripe:
		return "Stripe"
	case store.PaymentChannelTypeEPay:
		return "EPay"
	default:
		return "未知"
	}
}

func paymentChannelUsable(ch store.PaymentChannel) bool {
	if ch.Status != 1 {
		return false
	}
	switch ch.Type {
	case store.PaymentChannelTypeStripe:
		return ch.StripeSecretKey != nil && strings.TrimSpace(*ch.StripeSecretKey) != "" &&
			ch.StripeWebhookSecret != nil && strings.TrimSpace(*ch.StripeWebhookSecret) != ""
	case store.PaymentChannelTypeEPay:
		return ch.EPayGateway != nil && strings.TrimSpace(*ch.EPayGateway) != "" &&
			ch.EPayPartnerID != nil && strings.TrimSpace(*ch.EPayPartnerID) != "" &&
			ch.EPayKey != nil && strings.TrimSpace(*ch.EPayKey) != ""
	default:
		return false
	}
}

func toPaymentChannelView(ch store.PaymentChannel, baseURL string, loc *time.Location) paymentChannelView {
	v := paymentChannelView{
		ID:        ch.ID,
		Type:      ch.Type,
		TypeLabel: paymentChannelTypeLabel(ch.Type),
		Name:      ch.Name,
		Status:    ch.Status,
		Usable:    paymentChannelUsable(ch),
		CreatedAt: formatTimeIn(ch.CreatedAt, "2006-01-02 15:04:05", loc),
		UpdatedAt: formatTimeIn(ch.UpdatedAt, "2006-01-02 15:04:05", loc),
	}
	if ch.StripeCurrency != nil {
		v.StripeCurrency = strings.TrimSpace(*ch.StripeCurrency)
	}
	v.StripeSecretKeySet = ch.StripeSecretKey != nil && strings.TrimSpace(*ch.StripeSecretKey) != ""
	v.StripeWebhookSecretSet = ch.StripeWebhookSecret != nil && strings.TrimSpace(*ch.StripeWebhookSecret) != ""

	if ch.EPayGateway != nil {
		v.EPayGateway = strings.TrimSpace(*ch.EPayGateway)
	}
	if ch.EPayPartnerID != nil {
		v.EPayPartnerID = strings.TrimSpace(*ch.EPayPartnerID)
	}
	v.EPayKeySet = ch.EPayKey != nil && strings.TrimSpace(*ch.EPayKey) != ""

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch ch.Type {
	case store.PaymentChannelTypeStripe:
		v.WebhookURL = baseURL + "/api/pay/stripe/webhook/" + strconv.FormatInt(ch.ID, 10)
	case store.PaymentChannelTypeEPay:
		v.WebhookURL = baseURL + "/api/pay/epay/notify/" + strconv.FormatInt(ch.ID, 10)
	}
	return v
}

func (s *Server) PaymentChannels(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	loc, _ := s.adminTimeLocation(r.Context())
	rows, err := s.st.ListPaymentChannels(r.Context())
	if err != nil {
		http.Error(w, "查询支付渠道失败", http.StatusInternalServerError)
		return
	}
	baseURL := s.baseURLFromRequest(r)
	views := make([]paymentChannelView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toPaymentChannelView(row, baseURL, loc))
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_payment_channels", s.withFeatures(r.Context(), templateData{
		Title:           "支付渠道 - Realms",
		Error:           errMsg,
		Notice:          notice,
		User:            u,
		IsRoot:          isRoot,
		CSRFToken:       csrf,
		PaymentChannels: views,
	}))
}

func (s *Server) PaymentChannel(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	channelID, err := parseInt64(strings.TrimSpace(r.PathValue("payment_channel_id")))
	if err != nil || channelID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	ch, err := s.st.GetPaymentChannelByID(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "查询支付渠道失败", http.StatusInternalServerError)
		return
	}

	baseURL := s.baseURLFromRequest(r)
	loc, _ := s.adminTimeLocation(r.Context())
	view := toPaymentChannelView(ch, baseURL, loc)

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_payment_channel", s.withFeatures(r.Context(), templateData{
		Title:          "支付渠道配置 - Realms",
		Error:          errMsg,
		Notice:         notice,
		User:           u,
		IsRoot:         isRoot,
		CSRFToken:      csrf,
		PaymentChannel: &view,
	}))
}

func (s *Server) CreatePaymentChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels?err="+url.QueryEscape("表单解析失败"), http.StatusFound)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	typ := strings.TrimSpace(r.FormValue("type"))
	enabled := strings.TrimSpace(r.FormValue("enabled")) != ""

	var stripeCurrency *string
	if v := strings.TrimSpace(r.FormValue("stripe_currency")); v != "" {
		stripeCurrency = &v
	}
	var stripeSecretKey *string
	if v := strings.TrimSpace(r.FormValue("stripe_secret_key")); v != "" {
		stripeSecretKey = &v
	}
	var stripeWebhookSecret *string
	if v := strings.TrimSpace(r.FormValue("stripe_webhook_secret")); v != "" {
		stripeWebhookSecret = &v
	}

	var epayGateway *string
	if v := strings.TrimSpace(r.FormValue("epay_gateway")); v != "" {
		epayGateway = &v
	}
	var epayPartnerID *string
	if v := strings.TrimSpace(r.FormValue("epay_partner_id")); v != "" {
		epayPartnerID = &v
	}
	var epayKey *string
	if v := strings.TrimSpace(r.FormValue("epay_key")); v != "" {
		epayKey = &v
	}

	id, err := s.st.CreatePaymentChannel(r.Context(), store.CreatePaymentChannelInput{
		Type:   typ,
		Name:   name,
		Status: boolToInt(enabled),

		StripeCurrency:      stripeCurrency,
		StripeSecretKey:     stripeSecretKey,
		StripeWebhookSecret: stripeWebhookSecret,

		EPayGateway:   epayGateway,
		EPayPartnerID: epayPartnerID,
		EPayKey:       epayKey,
	})
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, fmt.Sprintf("已创建渠道 #%d", id))
		return
	}
	http.Redirect(w, r, "/admin/settings/payment-channels?msg="+url.QueryEscape(fmt.Sprintf("已创建渠道 #%d", id)), http.StatusFound)
}

func (s *Server) UpdatePaymentChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	channelID, err := parseInt64(strings.TrimSpace(r.PathValue("payment_channel_id")))
	if err != nil || channelID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels/"+strconv.FormatInt(channelID, 10)+"?err="+url.QueryEscape("表单解析失败"), http.StatusFound)
		return
	}

	ctx := r.Context()
	existing, err := s.st.GetPaymentChannelByID(ctx, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if isAjax(r) {
				ajaxError(w, http.StatusNotFound, "支付渠道不存在")
				return
			}
			http.NotFound(w, r)
			return
		}
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "查询支付渠道失败")
			return
		}
		http.Error(w, "查询支付渠道失败", http.StatusInternalServerError)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	enabled := strings.TrimSpace(r.FormValue("enabled")) != ""

	var stripeCurrency *string = existing.StripeCurrency
	var epayGateway *string = existing.EPayGateway
	var epayPartnerID *string = existing.EPayPartnerID

	switch existing.Type {
	case store.PaymentChannelTypeStripe:
		if v := strings.TrimSpace(r.FormValue("stripe_currency")); v != "" {
			stripeCurrency = &v
		} else {
			stripeCurrency = nil
		}
	case store.PaymentChannelTypeEPay:
		if v := strings.TrimSpace(r.FormValue("epay_gateway")); v != "" {
			epayGateway = &v
		} else {
			epayGateway = nil
		}
		if v := strings.TrimSpace(r.FormValue("epay_partner_id")); v != "" {
			epayPartnerID = &v
		} else {
			epayPartnerID = nil
		}
	}

	var stripeSecretKey *string
	if v := strings.TrimSpace(r.FormValue("stripe_secret_key")); v != "" {
		stripeSecretKey = &v
	}
	var stripeWebhookSecret *string
	if v := strings.TrimSpace(r.FormValue("stripe_webhook_secret")); v != "" {
		stripeWebhookSecret = &v
	}
	var epayKey *string
	if v := strings.TrimSpace(r.FormValue("epay_key")); v != "" {
		epayKey = &v
	}

	if err := s.st.UpdatePaymentChannel(ctx, store.UpdatePaymentChannelInput{
		ID:     channelID,
		Name:   name,
		Status: boolToInt(enabled),

		StripeCurrency: stripeCurrency,
		EPayGateway:    epayGateway,
		EPayPartnerID:  epayPartnerID,

		StripeSecretKey:     stripeSecretKey,
		StripeWebhookSecret: stripeWebhookSecret,
		EPayKey:             epayKey,
	}); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels/"+strconv.FormatInt(channelID, 10)+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/settings/payment-channels/"+strconv.FormatInt(channelID, 10)+"?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) DeletePaymentChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	channelID, err := parseInt64(strings.TrimSpace(r.PathValue("payment_channel_id")))
	if err != nil || channelID <= 0 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "参数错误")
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels?err="+url.QueryEscape("参数错误"), http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels?err="+url.QueryEscape("表单解析失败"), http.StatusFound)
		return
	}
	if err := s.st.DeletePaymentChannel(r.Context(), channelID); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "删除失败")
			return
		}
		http.Redirect(w, r, "/admin/settings/payment-channels?err="+url.QueryEscape("删除失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已删除")
		return
	}
	http.Redirect(w, r, "/admin/settings/payment-channels?msg="+url.QueryEscape("已删除"), http.StatusFound)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
