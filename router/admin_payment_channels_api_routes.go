package router

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

type adminPaymentChannelView struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	Name      string `json:"name"`
	Status    int    `json:"status"`
	Usable    bool   `json:"usable"`

	StripeCurrency         string `json:"stripe_currency,omitempty"`
	StripeSecretKeySet     bool   `json:"stripe_secret_key_set"`
	StripeWebhookSecretSet bool   `json:"stripe_webhook_secret_set"`

	EPayGateway   string `json:"epay_gateway,omitempty"`
	EPayPartnerID string `json:"epay_partner_id,omitempty"`
	EPayKeySet    bool   `json:"epay_key_set"`

	WebhookURL string `json:"webhook_url,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func setAdminPaymentChannelAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/payment-channels", adminListPaymentChannelsHandler(opts))
	r.POST("/payment-channels", adminCreatePaymentChannelHandler(opts))
	r.GET("/payment-channels/:payment_channel_id", adminGetPaymentChannelHandler(opts))
	r.PUT("/payment-channels/:payment_channel_id", adminUpdatePaymentChannelHandler(opts))
	r.DELETE("/payment-channels/:payment_channel_id", adminDeletePaymentChannelHandler(opts))
}

func adminPaymentChannelsFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableBilling) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
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

func toAdminPaymentChannelView(ch store.PaymentChannel, baseURL string) adminPaymentChannelView {
	v := adminPaymentChannelView{
		ID:        ch.ID,
		Type:      ch.Type,
		TypeLabel: paymentChannelTypeLabel(ch.Type),
		Name:      ch.Name,
		Status:    ch.Status,
		Usable:    paymentChannelUsable(ch),
		CreatedAt: ch.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: ch.UpdatedAt.Format("2006-01-02 15:04:05"),
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

func adminListPaymentChannelsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminPaymentChannelsFeatureDisabled(c, opts) {
			return
		}

		rows, err := opts.Store.ListPaymentChannels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询支付渠道失败"})
			return
		}
		baseURL := uiBaseURLFromRequest(c.Request.Context(), opts, c.Request)
		out := make([]adminPaymentChannelView, 0, len(rows))
		for _, row := range rows {
			out = append(out, toAdminPaymentChannelView(row, baseURL))
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminGetPaymentChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminPaymentChannelsFeatureDisabled(c, opts) {
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("payment_channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "payment_channel_id 不合法"})
			return
		}
		row, err := opts.Store.GetPaymentChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		baseURL := uiBaseURLFromRequest(c.Request.Context(), opts, c.Request)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": toAdminPaymentChannelView(row, baseURL)})
	}
}

func adminCreatePaymentChannelHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`

		StripeCurrency      *string `json:"stripe_currency,omitempty"`
		StripeSecretKey     *string `json:"stripe_secret_key,omitempty"`
		StripeWebhookSecret *string `json:"stripe_webhook_secret,omitempty"`

		EPayGateway   *string `json:"epay_gateway,omitempty"`
		EPayPartnerID *string `json:"epay_partner_id,omitempty"`
		EPayKey       *string `json:"epay_key,omitempty"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminPaymentChannelsFeatureDisabled(c, opts) {
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		id, err := opts.Store.CreatePaymentChannel(c.Request.Context(), store.CreatePaymentChannelInput{
			Type:   strings.TrimSpace(req.Type),
			Name:   strings.TrimSpace(req.Name),
			Status: boolToInt(req.Enabled),

			StripeCurrency:      req.StripeCurrency,
			StripeSecretKey:     req.StripeSecretKey,
			StripeWebhookSecret: req.StripeWebhookSecret,

			EPayGateway:   req.EPayGateway,
			EPayPartnerID: req.EPayPartnerID,
			EPayKey:       req.EPayKey,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": id}})
	}
}

func adminUpdatePaymentChannelHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name    *string `json:"name,omitempty"`
		Enabled *bool   `json:"enabled,omitempty"`

		StripeCurrency      *string `json:"stripe_currency,omitempty"`
		StripeSecretKey     *string `json:"stripe_secret_key,omitempty"`
		StripeWebhookSecret *string `json:"stripe_webhook_secret,omitempty"`

		EPayGateway   *string `json:"epay_gateway,omitempty"`
		EPayPartnerID *string `json:"epay_partner_id,omitempty"`
		EPayKey       *string `json:"epay_key,omitempty"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminPaymentChannelsFeatureDisabled(c, opts) {
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("payment_channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "payment_channel_id 不合法"})
			return
		}
		cur, err := opts.Store.GetPaymentChannelByID(c.Request.Context(), channelID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		name := cur.Name
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			name = strings.TrimSpace(*req.Name)
		}
		status := cur.Status
		if req.Enabled != nil {
			status = boolToInt(*req.Enabled)
		}

		if err := opts.Store.UpdatePaymentChannel(c.Request.Context(), store.UpdatePaymentChannelInput{
			ID:     channelID,
			Name:   name,
			Status: status,

			StripeCurrency: req.StripeCurrency,
			EPayGateway:    req.EPayGateway,
			EPayPartnerID:  req.EPayPartnerID,

			StripeSecretKey:     req.StripeSecretKey,
			StripeWebhookSecret: req.StripeWebhookSecret,
			EPayKey:             req.EPayKey,
		}); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminDeletePaymentChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminPaymentChannelsFeatureDisabled(c, opts) {
			return
		}
		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("payment_channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "payment_channel_id 不合法"})
			return
		}
		if err := opts.Store.DeletePaymentChannel(c.Request.Context(), channelID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
	}
}

