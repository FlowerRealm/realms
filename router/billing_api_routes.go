package router

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	stripeCheckout "github.com/stripe/stripe-go/v81/checkout/session"

	"realms/internal/store"
)

type billingSubscriptionOrderView struct {
	ID         int64  `json:"id"`
	PlanName   string `json:"plan_name"`
	AmountCNY  string `json:"amount_cny"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	PaidAt     string `json:"paid_at,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
}

type billingSubscriptionView struct {
	Active       bool                          `json:"active"`
	PlanName     string                        `json:"plan_name"`
	PriceCNY     string                        `json:"price_cny"`
	GroupName    string                        `json:"group_name"`
	StartAt      string                        `json:"start_at"`
	EndAt        string                        `json:"end_at"`
	UsageWindows []dashboardSubscriptionWindow `json:"usage_windows,omitempty"`
}

type billingPlanView struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	PriceCNY     string `json:"price_cny"`
	GroupName    string `json:"group_name"`
	Limit5H      string `json:"limit_5h"`
	Limit1D      string `json:"limit_1d"`
	Limit7D      string `json:"limit_7d"`
	Limit30D     string `json:"limit_30d"`
	DurationDays int    `json:"duration_days"`
}

type billingSubscriptionPageResponse struct {
	Subscription       *billingSubscriptionView       `json:"subscription,omitempty"`
	Subscriptions      []billingSubscriptionView      `json:"subscriptions"`
	Plans              []billingPlanView              `json:"plans"`
	SubscriptionOrders []billingSubscriptionOrderView `json:"subscription_orders"`
}

type billingTopupOrderView struct {
	ID        int64  `json:"id"`
	AmountCNY string `json:"amount_cny"`
	CreditUSD string `json:"credit_usd"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	PaidAt    string `json:"paid_at,omitempty"`
}

type billingTopupPageResponse struct {
	BalanceUSD        string                      `json:"balance_usd"`
	PayAsYouGoEnabled bool                        `json:"pay_as_you_go_enabled"`
	TopupMinCNY       string                      `json:"topup_min_cny"`
	TopupOrders       []billingTopupOrderView     `json:"topup_orders"`
	PaymentChannels   []billingPaymentChannelView `json:"payment_channels"`
}

type billingPayOrderView struct {
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	AmountCNY string `json:"amount_cny"`
	CreditUSD string `json:"credit_usd,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type billingPaymentChannelView struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	Name      string `json:"name"`
}

type billingPayPageResponse struct {
	BaseURL         string                      `json:"base_url"`
	PayOrder        billingPayOrderView         `json:"pay_order"`
	PaymentChannels []billingPaymentChannelView `json:"payment_channels"`
}

func listUsableBillingPaymentChannels(c *gin.Context, opts Options) []billingPaymentChannelView {
	out := make([]billingPaymentChannelView, 0)
	if c == nil || opts.Store == nil {
		return out
	}

	rows, err := opts.Store.ListPaymentChannels(c.Request.Context())
	if err != nil {
		return out
	}
	for _, ch := range rows {
		if ch.Status != 1 {
			continue
		}
		switch ch.Type {
		case store.PaymentChannelTypeStripe:
			if ch.StripeSecretKey == nil || strings.TrimSpace(*ch.StripeSecretKey) == "" || ch.StripeWebhookSecret == nil || strings.TrimSpace(*ch.StripeWebhookSecret) == "" {
				continue
			}
			out = append(out, billingPaymentChannelView{ID: ch.ID, Type: ch.Type, TypeLabel: "Stripe", Name: ch.Name})
		case store.PaymentChannelTypeEPay:
			if ch.EPayGateway == nil || strings.TrimSpace(*ch.EPayGateway) == "" || ch.EPayPartnerID == nil || strings.TrimSpace(*ch.EPayPartnerID) == "" || ch.EPayKey == nil || strings.TrimSpace(*ch.EPayKey) == "" {
				continue
			}
			out = append(out, billingPaymentChannelView{ID: ch.ID, Type: ch.Type, TypeLabel: "EPay", Name: ch.Name})
		}
	}
	return out
}

func setBillingAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/billing/subscription", authn, billingSubscriptionPageHandler(opts))
	r.POST("/billing/subscription/purchase", authn, billingPurchaseSubscriptionHandler(opts))

	r.GET("/billing/topup", authn, billingTopupPageHandler(opts))
	r.POST("/billing/topup/create", authn, billingCreateTopupOrderHandler(opts))

	r.GET("/billing/pay/:kind/:order_id", authn, billingPayPageHandler(opts))
	r.POST("/billing/pay/:kind/:order_id/cancel", authn, billingCancelPayOrderHandler(opts))
	r.POST("/billing/pay/:kind/:order_id/start", authn, billingStartPaymentHandler(opts))
}

func billingFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableBilling) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func billingSubscriptionPageHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		now := time.Now()

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}

		subs, err := opts.Store.ListNonExpiredSubscriptionsWithPlans(c.Request.Context(), userID, now)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订阅查询失败"})
			return
		}

		var activeID int64
		var activeEnd time.Time
		for _, row := range subs {
			if row.Subscription.StartAt.After(now) {
				continue
			}
			if activeID == 0 || row.Subscription.EndAt.Before(activeEnd) {
				activeID = row.Subscription.ID
				activeEnd = row.Subscription.EndAt
			}
		}

		subViews := make([]billingSubscriptionView, 0, len(subs))
		activeIndex := -1
		for _, row := range subs {
			isActive := !row.Subscription.StartAt.After(now)
			g := strings.TrimSpace(row.Plan.GroupName)
			sv := billingSubscriptionView{
				Active:    isActive,
				PlanName:  row.Plan.Name,
				PriceCNY:  formatCNY(row.Plan.PriceCNY),
				GroupName: g,
				StartAt:   row.Subscription.StartAt.Format("2006-01-02 15:04"),
				EndAt:     row.Subscription.EndAt.Format("2006-01-02 15:04"),
			}

			if isActive {
				type winCfg struct {
					name  string
					dur   time.Duration
					limit decimal.Decimal
				}
				wins := []winCfg{
					{name: "5小时", dur: 5 * time.Hour, limit: row.Plan.Limit5HUSD},
					{name: "1天", dur: 24 * time.Hour, limit: row.Plan.Limit1DUSD},
					{name: "7天", dur: 7 * 24 * time.Hour, limit: row.Plan.Limit7DUSD},
					{name: "30天", dur: 30 * 24 * time.Hour, limit: row.Plan.Limit30DUSD},
				}
				for _, wcfg := range wins {
					if wcfg.limit.LessThanOrEqual(decimal.Zero) {
						continue
					}
					since := now.Add(-wcfg.dur)
					if row.Subscription.StartAt.After(since) {
						since = row.Subscription.StartAt
					}
					committed, reserved, err := opts.Store.SumCommittedAndReservedUSDBySubscription(c.Request.Context(), store.UsageSumWithReservedBySubscriptionInput{
						UserID:         userID,
						SubscriptionID: row.Subscription.ID,
						Since:          since,
						Now:            now,
					})
					if err != nil {
						continue
					}
					used := committed.Add(reserved)
					percent := int(used.Mul(decimal.NewFromInt(100)).Div(wcfg.limit).IntPart())
					if percent > 100 {
						percent = 100
					}
					sv.UsageWindows = append(sv.UsageWindows, dashboardSubscriptionWindow{
						Window:      wcfg.name,
						UsedUSD:     formatUSD(used),
						LimitUSD:    formatUSD(wcfg.limit),
						UsedPercent: percent,
					})
				}
			}

			subViews = append(subViews, sv)
			if row.Subscription.ID == activeID {
				activeIndex = len(subViews) - 1
			}
		}

		plans, err := opts.Store.ListSubscriptionPlans(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "套餐查询失败"})
			return
		}
		ags, err := allowedSubgroupsForMainGroup(c.Request.Context(), opts.Store, u.MainGroup)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询用户分组失败"})
			return
		}
		planViews := make([]billingPlanView, 0, len(plans))
		for _, p := range plans {
			g := strings.TrimSpace(p.GroupName)
			if g != "" {
				if _, ok := ags.Set[g]; !ok {
					continue
				}
			}
			planViews = append(planViews, billingPlanView{
				ID:           p.ID,
				Name:         p.Name,
				PriceCNY:     formatCNY(p.PriceCNY),
				GroupName:    g,
				Limit5H:      formatUSDOrUnlimited(p.Limit5HUSD),
				Limit1D:      formatUSDOrUnlimited(p.Limit1DUSD),
				Limit7D:      formatUSDOrUnlimited(p.Limit7DUSD),
				Limit30D:     formatUSDOrUnlimited(p.Limit30DUSD),
				DurationDays: p.DurationDays,
			})
		}

		var activeSub *billingSubscriptionView
		if activeIndex >= 0 && activeIndex < len(subViews) {
			activeSub = &subViews[activeIndex]
		}

		orders, err := opts.Store.ListSubscriptionOrdersByUser(c.Request.Context(), userID, 50)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单查询失败"})
			return
		}
		orderViews := make([]billingSubscriptionOrderView, 0, len(orders))
		for _, row := range orders {
			status := "未知"
			switch row.Order.Status {
			case store.SubscriptionOrderStatusPending:
				status = "待支付"
			case store.SubscriptionOrderStatusActive:
				status = "已生效"
			case store.SubscriptionOrderStatusCanceled:
				status = "已取消"
			}
			v := billingSubscriptionOrderView{
				ID:        row.Order.ID,
				PlanName:  row.Plan.Name,
				AmountCNY: formatCNY(row.Order.AmountCNY),
				Status:    status,
				CreatedAt: row.Order.CreatedAt.Format("2006-01-02 15:04"),
			}
			if row.Order.PaidAt != nil {
				v.PaidAt = row.Order.PaidAt.Format("2006-01-02 15:04")
			}
			if row.Order.ApprovedAt != nil {
				v.ApprovedAt = row.Order.ApprovedAt.Format("2006-01-02 15:04")
			}
			orderViews = append(orderViews, v)
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": billingSubscriptionPageResponse{
				Subscription:       activeSub,
				Subscriptions:      subViews,
				Plans:              planViews,
				SubscriptionOrders: orderViews,
			},
		})
	}
}

func billingPurchaseSubscriptionHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		PlanID int64 `json:"plan_id"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil || req.PlanID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		o, plan, err := opts.Store.CreateSubscriptionOrderByPlanID(c.Request.Context(), userID, req.PlanID, time.Now())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "下单失败：" + err.Error()})
			return
		}

		msg := fmt.Sprintf("订单 #%d 已创建（%s - %s），请选择支付方式。", o.ID, plan.Name, formatCNY(plan.PriceCNY))
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": msg,
			"data": gin.H{
				"order_id": o.ID,
			},
		})
	}
}

func billingTopupPageHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		billingCfg := billingConfigEffective(c.Request.Context(), opts)

		balanceUSD, err := opts.Store.GetUserBalanceUSD(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "余额查询失败"})
			return
		}

		orders, err := opts.Store.ListTopupOrdersByUser(c.Request.Context(), userID, 50)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单查询失败"})
			return
		}
		views := make([]billingTopupOrderView, 0, len(orders))
		for _, o := range orders {
			status := "未知"
			switch o.Status {
			case store.TopupOrderStatusPending:
				status = "待支付"
			case store.TopupOrderStatusPaid:
				status = "已入账"
			case store.TopupOrderStatusCanceled:
				status = "已取消"
			}
			v := billingTopupOrderView{
				ID:        o.ID,
				AmountCNY: formatCNY(o.AmountCNY),
				CreditUSD: formatUSD(o.CreditUSD),
				Status:    status,
				CreatedAt: o.CreatedAt.Format("2006-01-02 15:04"),
			}
			if o.PaidAt != nil {
				v.PaidAt = o.PaidAt.Format("2006-01-02 15:04")
			}
			views = append(views, v)
		}

		paymentChannels := listUsableBillingPaymentChannels(c, opts)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": billingTopupPageResponse{
				BalanceUSD:        formatUSD(balanceUSD),
				PayAsYouGoEnabled: billingCfg.EnablePayAsYouGo,
				TopupMinCNY:       formatCNY(billingCfg.MinTopupCNY),
				TopupOrders:       views,
				PaymentChannels:   paymentChannels,
			},
		})
	}
}

func billingCreateTopupOrderHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		AmountCNY string `json:"amount_cny"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		amountCNY, err := parseCNY(req.AmountCNY)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "金额不合法（示例：10.00）：" + err.Error()})
			return
		}
		if amountCNY.LessThanOrEqual(decimal.Zero) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "金额必须大于 0"})
			return
		}

		cfg := billingConfigEffective(c.Request.Context(), opts)
		minTopupCNY := cfg.MinTopupCNY
		if minTopupCNY.GreaterThan(decimal.Zero) && amountCNY.LessThan(minTopupCNY) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "金额不能小于最低充值：" + formatCNY(minTopupCNY)})
			return
		}
		if cfg.CreditUSDPerCNY.LessThanOrEqual(decimal.Zero) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未配置充值入账比例"})
			return
		}
		creditUSD := amountCNY.Mul(cfg.CreditUSDPerCNY).Truncate(store.USDScale)

		o, err := opts.Store.CreateTopupOrder(c.Request.Context(), userID, amountCNY, creditUSD, time.Now())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建订单失败：" + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "订单已创建，请选择支付方式",
			"data":    gin.H{"order_id": o.ID},
		})
	}
}

func billingPayPageHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		kind := strings.TrimSpace(c.Param("kind"))
		orderID, err := strconv.ParseInt(strings.TrimSpace(c.Param("order_id")), 10, 64)
		if err != nil || orderID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		baseURL := uiBaseURLFromRequest(c.Request.Context(), opts, c.Request)
		paymentChannels := listUsableBillingPaymentChannels(c, opts)

		view := billingPayOrderView{Kind: kind, ID: orderID}
		switch kind {
		case "subscription":
			o, err := opts.Store.GetSubscriptionOrderByID(c.Request.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单查询失败"})
				return
			}
			if o.UserID != userID {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			plan, err := opts.Store.GetSubscriptionPlanByID(c.Request.Context(), o.PlanID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "套餐查询失败"})
				return
			}

			view.Title = "订阅购买"
			if err == nil && strings.TrimSpace(plan.Name) != "" {
				view.Title = "订阅购买 - " + strings.TrimSpace(plan.Name)
			}
			view.AmountCNY = formatCNY(o.AmountCNY)
			view.CreatedAt = o.CreatedAt.Format("2006-01-02 15:04")
			switch o.Status {
			case store.SubscriptionOrderStatusPending:
				view.Status = "待支付"
			case store.SubscriptionOrderStatusActive:
				view.Status = "已生效"
			case store.SubscriptionOrderStatusCanceled:
				view.Status = "已取消"
			default:
				view.Status = "未知"
			}
		case "topup":
			o, err := opts.Store.GetTopupOrderByID(c.Request.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单查询失败"})
				return
			}
			if o.UserID != userID {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}

			view.Title = "余额充值"
			view.AmountCNY = formatCNY(o.AmountCNY)
			view.CreditUSD = formatUSD(o.CreditUSD)
			view.CreatedAt = o.CreatedAt.Format("2006-01-02 15:04")
			switch o.Status {
			case store.TopupOrderStatusPending:
				view.Status = "待支付"
			case store.TopupOrderStatusPaid:
				view.Status = "已入账"
			case store.TopupOrderStatusCanceled:
				view.Status = "已取消"
			default:
				view.Status = "未知"
			}
		default:
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": billingPayPageResponse{
				BaseURL:         baseURL,
				PayOrder:        view,
				PaymentChannels: paymentChannels,
			},
		})
	}
}

func billingCancelPayOrderHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		kind := strings.TrimSpace(c.Param("kind"))
		orderID, err := strconv.ParseInt(strings.TrimSpace(c.Param("order_id")), 10, 64)
		if err != nil || orderID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		switch kind {
		case "subscription":
			if err := opts.Store.CancelSubscriptionOrderByUser(c.Request.Context(), userID, orderID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
		case "topup":
			if err := opts.Store.CancelTopupOrderByUser(c.Request.Context(), userID, orderID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
		default:
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "订单已取消。若您已完成支付，请联系管理员处理退款。"})
	}
}

func billingStartPaymentHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		PaymentChannelID int64   `json:"payment_channel_id"`
		EPayType         *string `json:"epay_type,omitempty"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if billingFeatureDisabled(c, opts) {
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}

		kind := strings.TrimSpace(c.Param("kind"))
		orderID, err := strconv.ParseInt(strings.TrimSpace(c.Param("order_id")), 10, 64)
		if err != nil || orderID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		paymentChannelID := req.PaymentChannelID
		epayType := ""
		if req.EPayType != nil {
			epayType = strings.ToLower(strings.TrimSpace(*req.EPayType))
		}

		if paymentChannelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "请先选择支付渠道"})
			return
		}

		baseURL := uiBaseURLFromRequest(c.Request.Context(), opts, c.Request)
		successURL := baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10) + "/success"
		cancelURL := baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10) + "/cancel"

		ref := ""
		orderTitle := ""
		var amountCNY decimal.Decimal
		switch kind {
		case "subscription":
			o, err := opts.Store.GetSubscriptionOrderByID(c.Request.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单查询失败"})
				return
			}
			if o.UserID != userID {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			if o.Status != store.SubscriptionOrderStatusPending {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单状态不可支付"})
				return
			}

			plan, err := opts.Store.GetSubscriptionPlanByID(c.Request.Context(), o.PlanID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "套餐查询失败"})
				return
			}
			orderTitle = "订阅购买"
			if err == nil && strings.TrimSpace(plan.Name) != "" {
				orderTitle = "订阅购买 - " + strings.TrimSpace(plan.Name)
			}
			amountCNY = o.AmountCNY
			ref = "sub_" + strconv.FormatInt(orderID, 10)
		case "topup":
			o, err := opts.Store.GetTopupOrderByID(c.Request.Context(), orderID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单查询失败"})
				return
			}
			if o.UserID != userID {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			if o.Status != store.TopupOrderStatusPending {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单状态不可支付"})
				return
			}
			orderTitle = "余额充值"
			amountCNY = o.AmountCNY
			ref = "topup_" + strconv.FormatInt(orderID, 10)
		default:
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			return
		}

		if amountCNY.LessThanOrEqual(decimal.Zero) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单金额不合法"})
			return
		}
		unitAmount, err := cnyToMinorUnits(amountCNY)
		if err != nil || unitAmount <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订单金额不合法"})
			return
		}

		ch, err := opts.Store.GetPaymentChannelByID(c.Request.Context(), paymentChannelID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "支付渠道不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "支付渠道查询失败"})
			return
		}
		if ch.Status != 1 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "支付渠道未启用"})
			return
		}

		switch ch.Type {
		case store.PaymentChannelTypeStripe:
			if ch.StripeSecretKey == nil || strings.TrimSpace(*ch.StripeSecretKey) == "" || ch.StripeWebhookSecret == nil || strings.TrimSpace(*ch.StripeWebhookSecret) == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Stripe 渠道未配置或不可用"})
				return
			}
			currency := "cny"
			if ch.StripeCurrency != nil {
				currency = strings.ToLower(strings.TrimSpace(*ch.StripeCurrency))
			}
			if currency == "" {
				currency = "cny"
			}

			stripe.Key = strings.TrimSpace(*ch.StripeSecretKey)

			exp := time.Now().Add(2 * time.Hour).Unix()
			params := &stripe.CheckoutSessionParams{
				SuccessURL:        stripe.String(successURL),
				CancelURL:         stripe.String(cancelURL),
				Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
				ClientReferenceID: stripe.String(ref),
				CustomerEmail:     stripe.String(u.Email),
				ExpiresAt:         stripe.Int64(exp),
				LineItems: []*stripe.CheckoutSessionLineItemParams{
					{
						PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
							Currency:   stripe.String(currency),
							UnitAmount: stripe.Int64(unitAmount),
							ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
								Name: stripe.String(orderTitle),
							},
						},
						Quantity: stripe.Int64(1),
					},
				},
			}
			sess, err := stripeCheckout.New(params)
			if err != nil || strings.TrimSpace(sess.URL) == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 Stripe 支付失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"redirect_url": sess.URL}})
			return
		case store.PaymentChannelTypeEPay:
			if ch.EPayGateway == nil || strings.TrimSpace(*ch.EPayGateway) == "" || ch.EPayPartnerID == nil || strings.TrimSpace(*ch.EPayPartnerID) == "" || ch.EPayKey == nil || strings.TrimSpace(*ch.EPayKey) == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "EPay 渠道未配置或不可用"})
				return
			}

			if epayType == "" {
				epayType = "alipay"
			}
			switch epayType {
			case "alipay", "wxpay", "qqpay":
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "EPay 支付类型不支持"})
				return
			}

			client, err := epay.NewClient(&epay.Config{
				PartnerID: strings.TrimSpace(*ch.EPayPartnerID),
				Key:       strings.TrimSpace(*ch.EPayKey),
			}, strings.TrimSpace(*ch.EPayGateway))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "EPay 配置错误"})
				return
			}

			notifyURL, err := url.Parse(baseURL + "/api/pay/epay/notify/" + strconv.FormatInt(paymentChannelID, 10))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "回调 URL 配置错误"})
				return
			}
			returnURL, err := url.Parse(baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "回跳 URL 配置错误"})
				return
			}

			money := formatCNYFixed(amountCNY)
			purchaseURL, params, err := client.Purchase(&epay.PurchaseArgs{
				Type:           epayType,
				ServiceTradeNo: ref,
				Name:           orderTitle,
				Money:          money,
				Device:         epay.PC,
				NotifyUrl:      notifyURL,
				ReturnUrl:      returnURL,
			})
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 EPay 支付失败"})
				return
			}

			u2, err := url.Parse(purchaseURL)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建 EPay 支付失败"})
				return
			}
			q := u2.Query()
			for k, v := range params {
				q.Set(k, v)
			}
			u2.RawQuery = q.Encode()

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"redirect_url": u2.String()}})
			return
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "支付渠道类型不支持"})
			return
		}
	}
}
