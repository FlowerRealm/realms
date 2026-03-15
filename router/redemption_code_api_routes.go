package router

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type redeemCodeResponse struct {
	RewardType                 string  `json:"reward_type"`
	BalanceUSD                 string  `json:"balance_usd,omitempty"`
	NewBalanceUSD              string  `json:"new_balance_usd,omitempty"`
	PlanName                   string  `json:"plan_name,omitempty"`
	SubscriptionStartAt        string  `json:"subscription_start_at,omitempty"`
	SubscriptionEndAt          string  `json:"subscription_end_at,omitempty"`
	SubscriptionActivationMode *string `json:"subscription_activation_mode,omitempty"`
}

type adminRedemptionCodeView struct {
	ID               int64  `json:"id"`
	BatchName        string `json:"batch_name"`
	Code             string `json:"code"`
	DistributionMode string `json:"distribution_mode"`
	RewardType       string `json:"reward_type"`
	PlanID           *int64 `json:"plan_id,omitempty"`
	PlanName         string `json:"plan_name,omitempty"`
	BalanceUSD       string `json:"balance_usd,omitempty"`
	MaxRedemptions   int    `json:"max_redemptions"`
	RedeemedCount    int    `json:"redeemed_count"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	Status           int    `json:"status"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func newGeneratedRedemptionCode() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "RC" + strings.ToUpper(hex.EncodeToString(buf[:])), nil
}

func setRedemptionCodeAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)
	r.POST("/billing/redeem", authn, redeemCodeHandler(opts))
}

func setAdminRedemptionCodeAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/redemption-codes", adminListRedemptionCodesHandler(opts))
	r.POST("/redemption-codes", adminCreateRedemptionCodesHandler(opts))
	r.GET("/redemption-codes/export", adminExportRedemptionCodesHandler(opts))
	r.PATCH("/redemption-codes/:code_id", adminUpdateRedemptionCodeHandler(opts))
	r.POST("/redemption-codes/:code_id/disable", adminDisableRedemptionCodeHandler(opts))
}

func redemptionCodeFeatureDisabled(c *gin.Context, opts Options) bool {
	return billingFeatureDisabled(c, opts)
}

func adminRedemptionCodesFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), store.SettingFeatureDisableBilling) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func toAdminRedemptionCodeView(item store.RedemptionCodeListItem) adminRedemptionCodeView {
	view := adminRedemptionCodeView{
		ID:               item.Code.ID,
		BatchName:        item.Code.BatchName,
		Code:             item.Code.Code,
		DistributionMode: string(item.Code.DistributionMode),
		RewardType:       string(item.Code.RewardType),
		MaxRedemptions:   item.Code.MaxRedemptions,
		RedeemedCount:    item.Code.RedeemedCount,
		Status:           int(item.Code.Status),
		CreatedAt:        item.Code.CreatedAt.Format("2006-01-02 15:04"),
		UpdatedAt:        item.Code.UpdatedAt.Format("2006-01-02 15:04"),
	}
	if item.Code.BalanceUSD.GreaterThan(decimal.Zero) {
		view.BalanceUSD = formatUSDPlain(item.Code.BalanceUSD)
	}
	if item.Code.SubscriptionPlanID != nil && *item.Code.SubscriptionPlanID > 0 {
		v := *item.Code.SubscriptionPlanID
		view.PlanID = &v
	}
	if item.Plan != nil {
		view.PlanName = item.Plan.Name
	}
	if item.Code.ExpiresAt != nil {
		view.ExpiresAt = item.Code.ExpiresAt.Format("2006-01-02 15:04")
	}
	return view
}

func parseRedemptionCodeStatus(raw string) (*store.RedemptionCodeStatus, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil, errors.New("status 不合法")
	}
	status := store.RedemptionCodeStatus(v)
	if status != store.RedemptionCodeStatusActive && status != store.RedemptionCodeStatusDisabled {
		return nil, errors.New("status 不合法")
	}
	return &status, nil
}

func parseRedemptionCodeExpiry(raw string) (*time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		return &endOfDay, nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, errors.New("expires_at 不合法")
}

func parseExpectedRedemptionRewardType(raw string) (store.RedemptionCodeRewardType, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return "", nil
	case string(store.RedemptionCodeRewardBalance):
		return store.RedemptionCodeRewardBalance, nil
	case string(store.RedemptionCodeRewardSubscription):
		return store.RedemptionCodeRewardSubscription, nil
	default:
		return "", errors.New("kind 不合法")
	}
}

func redeemCodeHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Kind                       string  `json:"kind"`
		Code                       string  `json:"code"`
		SubscriptionActivationMode *string `json:"subscription_activation_mode"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if redemptionCodeFeatureDisabled(c, opts) {
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
		expectedRewardType, err := parseExpectedRedemptionRewardType(req.Kind)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		res, err := opts.Store.RedeemCode(c.Request.Context(), store.RedeemCodeInput{
			UserID:                     userID,
			Code:                       req.Code,
			ExpectedRewardType:         expectedRewardType,
			SubscriptionActivationMode: req.SubscriptionActivationMode,
			Now:                        time.Now(),
		})
		if err != nil {
			switch {
			case errors.Is(err, store.ErrRedemptionCodeNotFound):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码不存在"})
			case errors.Is(err, store.ErrRedemptionCodeInactive):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码不可用"})
			case errors.Is(err, store.ErrRedemptionCodeExpired):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码已过期"})
			case errors.Is(err, store.ErrRedemptionCodeExhausted):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码已兑完"})
			case errors.Is(err, store.ErrRedemptionCodeAlreadyRedeemed):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "你已兑换过该兑换码"})
			case errors.Is(err, store.ErrRedemptionCodeRewardMismatch):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码类型不匹配"})
			case errors.Is(err, store.ErrSubscriptionActivationModeInvalid):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "subscription_activation_mode 不合法"})
			case errors.Is(err, store.ErrSubscriptionActivationModeRequired):
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "同套餐已有未过期订阅，请选择立即并行或顺延生效",
					"data": gin.H{
						"error_code": "subscription_activation_mode_required",
					},
				})
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换失败：" + err.Error()})
			}
			return
		}
		out := redeemCodeResponse{
			RewardType: string(res.RewardType),
		}
		msg := "兑换成功"
		switch res.RewardType {
		case store.RedemptionCodeRewardBalance:
			out.BalanceUSD = formatUSDPlain(res.BalanceUSD)
			out.NewBalanceUSD = formatUSDPlain(res.NewBalanceUSD)
			msg = "余额已入账"
		case store.RedemptionCodeRewardSubscription:
			if res.Plan != nil {
				out.PlanName = res.Plan.Name
			}
			if res.Subscription != nil {
				out.SubscriptionStartAt = res.Subscription.StartAt.Format("2006-01-02 15:04")
				out.SubscriptionEndAt = res.Subscription.EndAt.Format("2006-01-02 15:04")
			}
			out.SubscriptionActivationMode = res.SubscriptionActivationMode
			msg = "套餐已生效"
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": msg, "data": out})
	}
}

func adminListRedemptionCodesHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminRedemptionCodesFeatureDisabled(c, opts) {
			return
		}
		status, err := parseRedemptionCodeStatus(c.Query("status"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		var exhausted *bool
		if q := strings.TrimSpace(c.Query("exhausted")); q != "" {
			v := queryBool(q)
			exhausted = &v
		}
		items, err := opts.Store.ListRedemptionCodes(c.Request.Context(), store.RedemptionCodeListFilter{
			Keyword:          strings.TrimSpace(c.Query("keyword")),
			BatchName:        strings.TrimSpace(c.Query("batch_name")),
			DistributionMode: store.RedemptionCodeDistributionMode(strings.TrimSpace(c.Query("distribution_mode"))),
			RewardType:       store.RedemptionCodeRewardType(strings.TrimSpace(c.Query("reward_type"))),
			Status:           status,
			Exhausted:        exhausted,
			Limit:            500,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询兑换码失败"})
			return
		}
		out := make([]adminRedemptionCodeView, 0, len(items))
		for _, item := range items {
			out = append(out, toAdminRedemptionCodeView(item))
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminCreateRedemptionCodesHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		BatchName          string   `json:"batch_name"`
		Codes              []string `json:"codes"`
		Count              int      `json:"count"`
		DistributionMode   string   `json:"distribution_mode"`
		RewardType         string   `json:"reward_type"`
		SubscriptionPlanID *int64   `json:"subscription_plan_id"`
		BalanceUSD         string   `json:"balance_usd"`
		MaxRedemptions     int      `json:"max_redemptions"`
		ExpiresAt          string   `json:"expires_at"`
		Status             *int     `json:"status"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminRedemptionCodesFeatureDisabled(c, opts) {
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if strings.TrimSpace(req.BatchName) == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "batch_name 不能为空"})
			return
		}
		expiresAt, err := parseRedemptionCodeExpiry(req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		status := store.RedemptionCodeStatusActive
		if req.Status != nil {
			status = store.RedemptionCodeStatus(*req.Status)
			if status != store.RedemptionCodeStatusActive && status != store.RedemptionCodeStatusDisabled {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "status 不合法"})
				return
			}
		}
		var balanceUSD decimal.Decimal
		if strings.TrimSpace(req.BalanceUSD) != "" {
			balanceUSD, err = parseUSD(req.BalanceUSD)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
		}
		actorID, _ := adminActorIDFromContext(c)
		codes := make([]string, 0, len(req.Codes))
		seen := make(map[string]struct{}, len(req.Codes))
		for _, raw := range req.Codes {
			code := strings.ToUpper(strings.TrimSpace(raw))
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "codes 中包含重复兑换码"})
				return
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
		if len(codes) == 0 {
			count := req.Count
			if count <= 0 {
				count = 1
			}
			if count > 500 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "count 不能超过 500"})
				return
			}
			for i := 0; i < count; i++ {
				code, genErr := newGeneratedRedemptionCode()
				if genErr != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成兑换码失败"})
					return
				}
				codes = append(codes, code)
			}
		}
		items := make([]store.RedemptionCodeCreate, 0, len(codes))
		for _, code := range codes {
			items = append(items, store.RedemptionCodeCreate{
				BatchName:          strings.TrimSpace(req.BatchName),
				Code:               code,
				DistributionMode:   store.RedemptionCodeDistributionMode(strings.TrimSpace(req.DistributionMode)),
				RewardType:         store.RedemptionCodeRewardType(strings.TrimSpace(req.RewardType)),
				SubscriptionPlanID: req.SubscriptionPlanID,
				BalanceUSD:         balanceUSD,
				MaxRedemptions:     req.MaxRedemptions,
				ExpiresAt:          expiresAt,
				Status:             status,
				CreatedBy:          actorID,
			})
		}
		createdIDs, err := opts.Store.CreateRedemptionCodes(c.Request.Context(), items)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrRedemptionCodeDuplicate):
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码已存在"})
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"ids": createdIDs, "codes": codes}})
	}
}

func adminUpdateRedemptionCodeHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		MaxRedemptions *int    `json:"max_redemptions"`
		ExpiresAt      *string `json:"expires_at"`
		Status         *int    `json:"status"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminRedemptionCodesFeatureDisabled(c, opts) {
			return
		}
		codeID, err := strconv.ParseInt(strings.TrimSpace(c.Param("code_id")), 10, 64)
		if err != nil || codeID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "code_id 不合法"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		current, err := opts.Store.GetRedemptionCodeByID(c.Request.Context(), codeID)
		if err != nil {
			switch {
			case errors.Is(err, sql.ErrNoRows):
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询兑换码失败"})
			}
			return
		}

		maxRedemptions := current.Code.MaxRedemptions
		if req.MaxRedemptions != nil {
			maxRedemptions = *req.MaxRedemptions
		}

		expiresAt := current.Code.ExpiresAt
		if req.ExpiresAt != nil {
			expiresAt, err = parseRedemptionCodeExpiry(*req.ExpiresAt)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
		}

		status := current.Code.Status
		if req.Status != nil {
			status = store.RedemptionCodeStatus(*req.Status)
			if status != store.RedemptionCodeStatusActive && status != store.RedemptionCodeStatusDisabled {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "status 不合法"})
				return
			}
		}

		err = opts.Store.UpdateSharedRedemptionCode(c.Request.Context(), store.RedemptionCodeUpdate{
			ID:             codeID,
			MaxRedemptions: maxRedemptions,
			ExpiresAt:      expiresAt,
			Status:         status,
		})
		if err != nil {
			switch {
			case errors.Is(err, sql.ErrNoRows):
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}

func adminDisableRedemptionCodeHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminRedemptionCodesFeatureDisabled(c, opts) {
			return
		}
		codeID, err := strconv.ParseInt(strings.TrimSpace(c.Param("code_id")), 10, 64)
		if err != nil || codeID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "code_id 不合法"})
			return
		}
		if err := opts.Store.DisableRedemptionCode(c.Request.Context(), codeID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "停用失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已停用"})
	}
}

func adminExportRedemptionCodesHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminRedemptionCodesFeatureDisabled(c, opts) {
			return
		}
		status, err := parseRedemptionCodeStatus(c.Query("status"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		var exhausted *bool
		if q := strings.TrimSpace(c.Query("exhausted")); q != "" {
			v := queryBool(q)
			exhausted = &v
		}
		c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
		c.Writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "redemption-codes.csv"}))
		err = opts.Store.ExportRedemptionCodesCSV(c.Request.Context(), c.Writer, store.RedemptionCodeListFilter{
			Keyword:          strings.TrimSpace(c.Query("keyword")),
			BatchName:        strings.TrimSpace(c.Query("batch_name")),
			DistributionMode: store.RedemptionCodeDistributionMode(strings.TrimSpace(c.Query("distribution_mode"))),
			RewardType:       store.RedemptionCodeRewardType(strings.TrimSpace(c.Query("reward_type"))),
			Status:           status,
			Exhausted:        exhausted,
			Limit:            5000,
		})
		if err != nil {
			c.Status(http.StatusInternalServerError)
		}
	}
}
