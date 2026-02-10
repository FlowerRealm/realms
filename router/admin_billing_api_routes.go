package router

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type adminSubscriptionPlanView struct {
	ID              int64  `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	GroupName       string `json:"group_name"`
	PriceMultiplier string `json:"price_multiplier"`
	PriceCNY        string `json:"price_cny"`
	DurationDays    int    `json:"duration_days"`
	Status          int    `json:"status"`

	Limit5H  string `json:"limit_5h,omitempty"`
	Limit1D  string `json:"limit_1d,omitempty"`
	Limit7D  string `json:"limit_7d,omitempty"`
	Limit30D string `json:"limit_30d,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type adminSubscriptionOrderView struct {
	ID         int64  `json:"id"`
	UserEmail  string `json:"user_email"`
	PlanName   string `json:"plan_name"`
	GroupName  string `json:"group_name,omitempty"`
	AmountCNY  string `json:"amount_cny"`
	Status     int    `json:"status"`
	StatusText string `json:"status_text"`
	CreatedAt  string `json:"created_at"`
	PaidAt     string `json:"paid_at,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
}

func setAdminBillingAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/subscriptions", adminListSubscriptionPlansHandler(opts))
	r.POST("/subscriptions", adminCreateSubscriptionPlanHandler(opts))
	r.GET("/subscriptions/:plan_id", adminGetSubscriptionPlanHandler(opts))
	r.PUT("/subscriptions/:plan_id", adminUpdateSubscriptionPlanHandler(opts))
	r.DELETE("/subscriptions/:plan_id", adminDeleteSubscriptionPlanHandler(opts))

	r.GET("/orders", adminListSubscriptionOrdersHandler(opts))
	r.POST("/orders/:order_id/approve", adminApproveSubscriptionOrderHandler(opts))
	r.POST("/orders/:order_id/reject", adminRejectSubscriptionOrderHandler(opts))
}

func adminBillingFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableBilling) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func adminListSubscriptionPlansHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		plans, err := opts.Store.ListAllSubscriptionPlans(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		out := make([]adminSubscriptionPlanView, 0, len(plans))
		for _, p := range plans {
			view := adminSubscriptionPlanView{
				ID:              p.ID,
				Code:            p.Code,
				Name:            p.Name,
				GroupName:       strings.TrimSpace(p.GroupName),
				PriceMultiplier: formatDecimalPlain(p.PriceMultiplier, store.PriceMultiplierScale),
				PriceCNY:        formatDecimalPlain(p.PriceCNY, store.CNYScale),
				DurationDays:    p.DurationDays,
				Status:          p.Status,
				CreatedAt:       p.CreatedAt.Format("2006-01-02 15:04"),
				UpdatedAt:       p.UpdatedAt.Format("2006-01-02 15:04"),
			}
			if p.Limit5HUSD.GreaterThan(decimal.Zero) {
				view.Limit5H = formatDecimalPlain(p.Limit5HUSD, store.USDScale)
			}
			if p.Limit1DUSD.GreaterThan(decimal.Zero) {
				view.Limit1D = formatDecimalPlain(p.Limit1DUSD, store.USDScale)
			}
			if p.Limit7DUSD.GreaterThan(decimal.Zero) {
				view.Limit7D = formatDecimalPlain(p.Limit7DUSD, store.USDScale)
			}
			if p.Limit30DUSD.GreaterThan(decimal.Zero) {
				view.Limit30D = formatDecimalPlain(p.Limit30DUSD, store.USDScale)
			}
			if view.DurationDays <= 0 {
				view.DurationDays = 30
			}
			if view.GroupName == "" {
				view.GroupName = store.DefaultGroupName
			}
			out = append(out, view)
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminGetSubscriptionPlanHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		planID, err := strconv.ParseInt(strings.TrimSpace(c.Param("plan_id")), 10, 64)
		if err != nil || planID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "plan_id 不合法"})
			return
		}
		p, err := opts.Store.GetSubscriptionPlanByID(c.Request.Context(), planID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		view := adminSubscriptionPlanView{
			ID:              p.ID,
			Code:            p.Code,
			Name:            p.Name,
			GroupName:       strings.TrimSpace(p.GroupName),
			PriceMultiplier: formatDecimalPlain(p.PriceMultiplier, store.PriceMultiplierScale),
			PriceCNY:        formatDecimalPlain(p.PriceCNY, store.CNYScale),
			DurationDays:    p.DurationDays,
			Status:          p.Status,
			CreatedAt:       p.CreatedAt.Format("2006-01-02 15:04"),
			UpdatedAt:       p.UpdatedAt.Format("2006-01-02 15:04"),
		}
		if p.Limit5HUSD.GreaterThan(decimal.Zero) {
			view.Limit5H = formatDecimalPlain(p.Limit5HUSD, store.USDScale)
		}
		if p.Limit1DUSD.GreaterThan(decimal.Zero) {
			view.Limit1D = formatDecimalPlain(p.Limit1DUSD, store.USDScale)
		}
		if p.Limit7DUSD.GreaterThan(decimal.Zero) {
			view.Limit7D = formatDecimalPlain(p.Limit7DUSD, store.USDScale)
		}
		if p.Limit30DUSD.GreaterThan(decimal.Zero) {
			view.Limit30D = formatDecimalPlain(p.Limit30DUSD, store.USDScale)
		}
		if view.DurationDays <= 0 {
			view.DurationDays = 30
		}
		if view.GroupName == "" {
			view.GroupName = store.DefaultGroupName
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
	}
}

func adminCreateSubscriptionPlanHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Code            string `json:"code"`
		Name            string `json:"name"`
		GroupName       string `json:"group_name"`
		PriceMultiplier string `json:"price_multiplier"`
		PriceCNY        string `json:"price_cny"`
		DurationDays    int    `json:"duration_days"`
		Status          int    `json:"status"`

		Limit5H  string `json:"limit_5h"`
		Limit1D  string `json:"limit_1d"`
		Limit7D  string `json:"limit_7d"`
		Limit30D string `json:"limit_30d"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "name 不能为空"})
			return
		}

		code := strings.TrimSpace(req.Code)
		if code == "" {
			gen, err := newSubscriptionPlanCode()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成套餐标识失败"})
				return
			}
			code = gen
		}

		group := strings.TrimSpace(req.GroupName)
		if group == "" {
			group = store.DefaultGroupName
		}
		if err := validateChannelGroupSelectable(c.Request.Context(), opts.Store, group); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		priceCNY, err := parseCNY(req.PriceCNY)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "价格不合法"})
			return
		}
		priceMultiplier, err := parseOptionalPriceMultiplier(req.PriceMultiplier)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "倍率不合法"})
			return
		}
		limit5H, err := parseOptionalUSD(req.Limit5H)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "5h 限额不合法"})
			return
		}
		limit1D, err := parseOptionalUSD(req.Limit1D)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "1d 限额不合法"})
			return
		}
		limit7D, err := parseOptionalUSD(req.Limit7D)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "7d 限额不合法"})
			return
		}
		limit30D, err := parseOptionalUSD(req.Limit30D)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "30d 限额不合法"})
			return
		}

		duration := req.DurationDays
		if duration <= 0 {
			duration = 30
		}
		status := req.Status
		if status != 0 && status != 1 {
			status = 1
		}

		id, err := opts.Store.CreateSubscriptionPlan(c.Request.Context(), store.SubscriptionPlanCreate{
			Code:            code,
			Name:            name,
			GroupName:       group,
			PriceMultiplier: priceMultiplier,
			PriceCNY:        priceCNY,
			Limit5HUSD:      limit5H,
			Limit1DUSD:      limit1D,
			Limit7DUSD:      limit7D,
			Limit30DUSD:     limit30D,
			DurationDays:    duration,
			Status:          status,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": id}})
	}
}

func adminUpdateSubscriptionPlanHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Code            string `json:"code"`
		Name            string `json:"name"`
		GroupName       string `json:"group_name"`
		PriceMultiplier string `json:"price_multiplier"`
		PriceCNY        string `json:"price_cny"`
		DurationDays    int    `json:"duration_days"`
		Status          int    `json:"status"`

		Limit5H  string `json:"limit_5h"`
		Limit1D  string `json:"limit_1d"`
		Limit7D  string `json:"limit_7d"`
		Limit30D string `json:"limit_30d"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		planID, err := strconv.ParseInt(strings.TrimSpace(c.Param("plan_id")), 10, 64)
		if err != nil || planID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "plan_id 不合法"})
			return
		}
		if _, err := opts.Store.GetSubscriptionPlanByID(c.Request.Context(), planID); err != nil {
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

		name := strings.TrimSpace(req.Name)
		if name == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "name 不能为空"})
			return
		}

		code := strings.TrimSpace(req.Code)
		if code == "" {
			gen, err := newSubscriptionPlanCode()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成套餐标识失败"})
				return
			}
			code = gen
		}

		group := strings.TrimSpace(req.GroupName)
		if group == "" {
			group = store.DefaultGroupName
		}
		if err := validateChannelGroupSelectable(c.Request.Context(), opts.Store, group); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		priceCNY, err := parseCNY(req.PriceCNY)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "价格不合法"})
			return
		}
		priceMultiplier, err := parseOptionalPriceMultiplier(req.PriceMultiplier)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "倍率不合法"})
			return
		}
		limit5H, err := parseOptionalUSD(req.Limit5H)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "5h 限额不合法"})
			return
		}
		limit1D, err := parseOptionalUSD(req.Limit1D)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "1d 限额不合法"})
			return
		}
		limit7D, err := parseOptionalUSD(req.Limit7D)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "7d 限额不合法"})
			return
		}
		limit30D, err := parseOptionalUSD(req.Limit30D)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "30d 限额不合法"})
			return
		}

		duration := req.DurationDays
		if duration <= 0 {
			duration = 30
		}
		status := req.Status
		if status != 0 && status != 1 {
			status = 1
		}

		if err := opts.Store.UpdateSubscriptionPlan(c.Request.Context(), store.SubscriptionPlanUpdate{
			ID:              planID,
			Code:            code,
			Name:            name,
			GroupName:       group,
			PriceMultiplier: priceMultiplier,
			PriceCNY:        priceCNY,
			Limit5HUSD:      limit5H,
			Limit1DUSD:      limit1D,
			Limit7DUSD:      limit7D,
			Limit30DUSD:     limit30D,
			DurationDays:    duration,
			Status:          status,
		}); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminDeleteSubscriptionPlanHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		planID, err := strconv.ParseInt(strings.TrimSpace(c.Param("plan_id")), 10, 64)
		if err != nil || planID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "plan_id 不合法"})
			return
		}
		sum, err := opts.Store.DeleteSubscriptionPlan(c.Request.Context(), planID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		msg := "已删除"
		if sum.SubscriptionsDeleted > 0 || sum.OrdersDeleted > 0 || sum.UsageEventsUnbound > 0 {
			msg += "（subscriptions=" + strconv.FormatInt(sum.SubscriptionsDeleted, 10) + ", orders=" + strconv.FormatInt(sum.OrdersDeleted, 10) + ", usage_events=" + strconv.FormatInt(sum.UsageEventsUnbound, 10) + "）"
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
	}
}

func adminListSubscriptionOrdersHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}

		rows, err := opts.Store.ListRecentSubscriptionOrders(c.Request.Context(), 200)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询订单失败"})
			return
		}

		out := make([]adminSubscriptionOrderView, 0, len(rows))
		for _, row := range rows {
			statusText := "未知"
			switch row.Order.Status {
			case store.SubscriptionOrderStatusPending:
				statusText = "待支付"
			case store.SubscriptionOrderStatusActive:
				statusText = "已生效"
			case store.SubscriptionOrderStatusCanceled:
				statusText = "已取消"
			}
			view := adminSubscriptionOrderView{
				ID:         row.Order.ID,
				UserEmail:  row.UserEmail,
				PlanName:   row.Plan.Name,
				GroupName:  strings.TrimSpace(row.Plan.GroupName),
				AmountCNY:  formatDecimalPlain(row.Order.AmountCNY, store.CNYScale),
				Status:     row.Order.Status,
				StatusText: statusText,
				CreatedAt:  row.Order.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			if row.Order.PaidAt != nil {
				view.PaidAt = row.Order.PaidAt.Format("2006-01-02 15:04:05")
			}
			if row.Order.ApprovedAt != nil {
				view.ApprovedAt = row.Order.ApprovedAt.Format("2006-01-02 15:04:05")
			}
			if view.GroupName == "" {
				view.GroupName = store.DefaultGroupName
			}
			out = append(out, view)
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminApproveSubscriptionOrderHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		orderID, err := strconv.ParseInt(strings.TrimSpace(c.Param("order_id")), 10, 64)
		if err != nil || orderID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "order_id 不合法"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		subID, err := opts.Store.ApproveSubscriptionOrderAndDelete(c.Request.Context(), orderID, userID, time.Now())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "批准失败：" + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "订单已批准并生效。", "data": gin.H{"subscription_id": subID}})
	}
}

func adminRejectSubscriptionOrderHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminBillingFeatureDisabled(c, opts) {
			return
		}
		orderID, err := strconv.ParseInt(strings.TrimSpace(c.Param("order_id")), 10, 64)
		if err != nil || orderID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "order_id 不合法"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		if err := opts.Store.RejectSubscriptionOrderAndDelete(c.Request.Context(), orderID, userID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "拒绝失败：" + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "订单已拒绝。"})
	}
}

func parseOptionalUSD(raw string) (decimal.Decimal, error) {
	// This helper keeps API consistent with SSR: empty means unlimited (=0).
	if strings.TrimSpace(raw) == "" {
		return decimal.Zero, nil
	}
	v, err := parseUSD(raw)
	if err != nil {
		return decimal.Zero, err
	}
	return v, nil
}

func parseOptionalPriceMultiplier(raw string) (decimal.Decimal, error) {
	// empty -> default (=1), for UI convenience.
	if strings.TrimSpace(raw) == "" {
		return store.DefaultGroupPriceMultiplier, nil
	}
	d, err := parseDecimalNonNeg(raw, store.PriceMultiplierScale)
	if err != nil {
		return decimal.Zero, err
	}
	if d.Sign() <= 0 {
		return decimal.Zero, errors.New("倍率必须大于 0")
	}
	return d, nil
}

func validateChannelGroupSelectable(ctx context.Context, st *store.Store, groupName string) error {
	if st == nil {
		return nil
	}
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		groupName = store.DefaultGroupName
	}
	g, err := st.GetChannelGroupByName(ctx, groupName)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("组不存在")
		}
		return errors.New("组查询失败")
	}
	if g.Status != 1 {
		return errors.New("组已禁用")
	}
	return nil
}

func newSubscriptionPlanCode() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "sp_" + hex.EncodeToString(buf[:]), nil
}
