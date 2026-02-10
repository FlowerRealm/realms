package router

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

type userTokenView struct {
	ID         int64      `json:"id"`
	Name       *string    `json:"name,omitempty"`
	TokenHint  *string    `json:"token_hint,omitempty"`
	Status     int        `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func setTokenAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.GET("/token", authn, listUserTokensHandler(opts))
	r.GET("/token/", authn, listUserTokensHandler(opts))

	r.POST("/token", authn, createUserTokenHandler(opts))
	r.POST("/token/", authn, createUserTokenHandler(opts))

	r.GET("/token/:token_id/reveal", authn, revealUserTokenHandler(opts))
	r.POST("/token/:token_id/rotate", authn, rotateUserTokenHandler(opts))
	r.POST("/token/:token_id/revoke", authn, revokeUserTokenHandler(opts))
	r.GET("/token/:token_id/groups", authn, getUserTokenGroupsHandler(opts))
	r.PUT("/token/:token_id/groups", authn, replaceUserTokenGroupsHandler(opts))
	r.DELETE("/token/:token_id", authn, deleteUserTokenHandler(opts))
}

func listUserTokensHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokens, err := opts.Store.ListUserTokens(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Token 列表失败"})
			return
		}
		out := make([]userTokenView, 0, len(tokens))
		for _, t := range tokens {
			out = append(out, userTokenView{
				ID:         t.ID,
				Name:       t.Name,
				TokenHint:  t.TokenHint,
				Status:     t.Status,
				CreatedAt:  t.CreatedAt,
				RevokedAt:  t.RevokedAt,
				LastUsedAt: t.LastUsedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func createUserTokenHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Name *string `json:"name,omitempty"`
	}
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		var req reqBody
		_ = c.ShouldBindJSON(&req)
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				req.Name = nil
			} else {
				req.Name = &name
			}
		}

		raw, err := auth.NewRandomToken("sk_", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成令牌失败"})
			return
		}
		tokenID, hint, err := opts.Store.CreateUserToken(c.Request.Context(), userID, req.Name, raw)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建令牌失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"token_id":   tokenID,
				"token":      raw,
				"token_hint": hint,
			},
		})
	}
}

func rotateUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}

		raw, err := auth.NewRandomToken("sk_", 32)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成令牌失败"})
			return
		}
		if err := opts.Store.RotateUserToken(c.Request.Context(), userID, tokenID, raw); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "令牌不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "重新生成失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"token_id": tokenID,
				"token":    raw,
			},
		})
	}
}

func revealUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}

		tok, err := opts.Store.RevealUserToken(c.Request.Context(), userID, tokenID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "令牌不存在"})
				return
			}
			if errors.Is(err, store.ErrUserTokenRevoked) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "令牌已撤销，无法查看"})
				return
			}
			if errors.Is(err, store.ErrUserTokenNotRevealable) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "旧令牌不支持查看，请重新生成"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查看失败"})
			return
		}

		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"token_id": tokenID,
				"token":    tok,
			},
		})
	}
}

func revokeUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}
		if err := opts.Store.RevokeUserToken(c.Request.Context(), userID, tokenID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "撤销失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func deleteUserTokenHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}
		if err := opts.Store.DeleteUserToken(c.Request.Context(), userID, tokenID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "令牌不存在"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

type tokenGroupOptionView struct {
	Name              string          `json:"name"`
	Description       *string         `json:"description,omitempty"`
	Status            int             `json:"status"`
	PriceMultiplier   decimal.Decimal `json:"price_multiplier"`
	UserGroupPriority int             `json:"user_group_priority"`
}

type tokenGroupBindingView struct {
	GroupName string `json:"group_name"`
	Priority  int    `json:"priority"`
}

type tokenGroupsView struct {
	TokenID           int64                   `json:"token_id"`
	UserGroup         string                  `json:"user_group"`
	AllowedGroups     []tokenGroupOptionView  `json:"allowed_groups"`
	Bindings          []tokenGroupBindingView `json:"bindings"`
	EffectiveBindings []tokenGroupBindingView `json:"effective_bindings"`
}

func getUserTokenGroupsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}

		tokens, err := opts.Store.ListUserTokens(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Token 失败"})
			return
		}
		owned := false
		for _, t := range tokens {
			if t.ID == tokenID {
				owned = true
				break
			}
		}
		if !owned {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "not found"})
			return
		}

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}
		mainGroup := strings.TrimSpace(u.MainGroup)
		if mainGroup == "" {
			mainGroup = store.DefaultGroupName
		}

		allowedRows, err := opts.Store.ListMainGroupSubgroups(c.Request.Context(), mainGroup)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if len(allowedRows) == 0 {
			allowedRows = append(allowedRows, store.MainGroupSubgroup{MainGroup: mainGroup, Subgroup: store.DefaultGroupName, Priority: 0})
		}

		cgs, err := opts.Store.ListChannelGroups(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询分组失败"})
			return
		}
		cgByName := make(map[string]store.ChannelGroup, len(cgs))
		for _, g := range cgs {
			cgByName[strings.TrimSpace(g.Name)] = g
		}

		allowedViews := make([]tokenGroupOptionView, 0, len(allowedRows)+1)
		seenAllowed := make(map[string]struct{}, len(allowedRows)+1)
		for _, row := range allowedRows {
			name := strings.TrimSpace(row.Subgroup)
			if name == "" {
				continue
			}
			if _, ok := seenAllowed[name]; ok {
				continue
			}
			seenAllowed[name] = struct{}{}
			cg, ok := cgByName[name]
			priceMult := store.DefaultGroupPriceMultiplier
			status := 0
			var desc *string
			if ok {
				priceMult = cg.PriceMultiplier
				status = cg.Status
				desc = cg.Description
			}
			allowedViews = append(allowedViews, tokenGroupOptionView{
				Name:              name,
				Description:       desc,
				Status:            status,
				PriceMultiplier:   priceMult,
				UserGroupPriority: row.Priority,
			})
		}
		if _, ok := seenAllowed[store.DefaultGroupName]; !ok {
			priceMult := store.DefaultGroupPriceMultiplier
			status := 0
			var desc *string
			if cg, ok := cgByName[store.DefaultGroupName]; ok {
				priceMult = cg.PriceMultiplier
				status = cg.Status
				desc = cg.Description
			}
			allowedViews = append(allowedViews, tokenGroupOptionView{
				Name:              store.DefaultGroupName,
				Description:       desc,
				Status:            status,
				PriceMultiplier:   priceMult,
				UserGroupPriority: 0,
			})
		}

		bindings, err := opts.Store.ListTokenGroupBindings(c.Request.Context(), tokenID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Token 分组失败"})
			return
		}
		effective, err := opts.Store.ListEffectiveTokenGroupBindings(c.Request.Context(), tokenID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Token 生效分组失败"})
			return
		}

		toViews := func(rows []store.TokenGroupBinding) []tokenGroupBindingView {
			out := make([]tokenGroupBindingView, 0, len(rows))
			for _, row := range rows {
				name := strings.TrimSpace(row.GroupName)
				if name == "" {
					continue
				}
				out = append(out, tokenGroupBindingView{GroupName: name, Priority: row.Priority})
			}
			if len(out) == 0 {
				out = append(out, tokenGroupBindingView{GroupName: store.DefaultGroupName, Priority: 0})
			}
			return out
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": tokenGroupsView{
			TokenID:           tokenID,
			UserGroup:         mainGroup,
			AllowedGroups:     allowedViews,
			Bindings:          toViews(bindings),
			EffectiveBindings: toViews(effective),
		}})
	}
}

func replaceUserTokenGroupsHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Groups []string `json:"groups"`
	}
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
		if err != nil || tokenID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "token_id 不合法"})
			return
		}

		tokens, err := opts.Store.ListUserTokens(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询 Token 失败"})
			return
		}
		owned := false
		for _, t := range tokens {
			if t.ID == tokenID {
				owned = true
				break
			}
		}
		if !owned {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "not found"})
			return
		}

		var req reqBody
		_ = c.ShouldBindJSON(&req)
		if err := opts.Store.ReplaceTokenGroups(c.Request.Context(), tokenID, req.Groups); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
	}
}
