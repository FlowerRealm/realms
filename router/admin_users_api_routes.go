package router

import (
	"database/sql"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

type adminUserView struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	UserGroup  string `json:"user_group"`
	Role       string `json:"role"`
	Status     int    `json:"status"`
	BalanceUSD string `json:"balance_usd"`
	CreatedAt  string `json:"created_at"`
}

func setAdminUserAPIRoutes(r gin.IRoutes, opts Options) {
	r.GET("/users", adminListUsersHandler(opts))
	r.POST("/users", adminCreateUserHandler(opts))
	r.PUT("/users/:user_id", adminUpdateUserHandler(opts))
	r.POST("/users/:user_id/password", adminResetUserPasswordHandler(opts))
	r.POST("/users/:user_id/balance", adminAddUserBalanceHandler(opts))
	r.DELETE("/users/:user_id", adminDeleteUserHandler(opts))
}

func adminUsersFeatureDisabled(c *gin.Context, opts Options) bool {
	if c == nil || opts.Store == nil {
		return false
	}
	if opts.Store.FeatureDisabledEffective(c.Request.Context(), opts.SelfMode, store.SettingFeatureDisableAdminUsers) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
		return true
	}
	return false
}

func adminListUsersHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}

		users, err := opts.Store.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}
		userIDs := make([]int64, 0, len(users))
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
		balances, err := opts.Store.GetUserBalancesUSD(c.Request.Context(), userIDs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "余额查询失败"})
			return
		}

		out := make([]adminUserView, 0, len(users))
		for _, u := range users {
			mainGroup := strings.TrimSpace(u.MainGroup)
			if mainGroup == "" {
				mainGroup = store.DefaultGroupName
			}
			out = append(out, adminUserView{
				ID:         u.ID,
				Email:      u.Email,
				Username:   u.Username,
				UserGroup:  mainGroup,
				Role:       u.Role,
				Status:     u.Status,
				BalanceUSD: formatUSDPlain(balances[u.ID]),
				CreatedAt:  u.CreatedAt.Format("2006-01-02 15:04"),
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func adminCreateUserHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Email     string `json:"email"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		Role      string `json:"role"`
		UserGroup string `json:"user_group"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		email := strings.TrimSpace(strings.ToLower(req.Email))
		username, err := store.NormalizeUsername(req.Username)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		password := req.Password
		if email == "" || password == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱或密码不能为空"})
			return
		}
		if _, err := mail.ParseAddress(email); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱不合法"})
			return
		}

		role := strings.TrimSpace(req.Role)
		if role == "" {
			role = store.UserRoleUser
		}
		if role != store.UserRoleUser && role != store.UserRoleRoot {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "role 不合法"})
			return
		}

		// 账号名占用检查（保持与 SSR 一致）。
		if _, err := opts.Store.GetUserByUsername(c.Request.Context(), username); err == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "账号名已被占用"})
			return
		} else if err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询账号名失败"})
			return
		}

		pwHash, err := auth.HashPassword(password)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "密码不合法"})
			return
		}
		userID, err := opts.Store.CreateUser(c.Request.Context(), email, username, pwHash, role)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建失败（可能邮箱或账号名已存在）"})
			return
		}

		if err := opts.Store.SetUserMainGroup(c.Request.Context(), userID, req.UserGroup); err != nil {
			_ = opts.Store.DeleteUser(c.Request.Context(), userID)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已创建", "data": gin.H{"id": userID}})
	}
}

func adminUpdateUserHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Email     *string `json:"email,omitempty"`
		Username  *string `json:"username,omitempty"`
		Status    *int    `json:"status,omitempty"`
		Role      *string `json:"role,omitempty"`
		UserGroup *string `json:"user_group,omitempty"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}

		userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "user_id 不合法"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		target, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		actorID, _ := userIDFromContext(c)
		if actorID > 0 && actorID == target.ID {
			// 与 SSR 一致：不能禁用/降级当前用户。
			if req.Status != nil && *req.Status == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "不能禁用当前登录用户"})
				return
			}
			if req.Role != nil && strings.TrimSpace(*req.Role) != store.UserRoleRoot {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "不能修改当前登录用户的 root 角色"})
				return
			}
		}

		changed := false

		if req.Email != nil {
			email := strings.TrimSpace(strings.ToLower(*req.Email))
			if email == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱不能为空"})
				return
			}
			if _, err := mail.ParseAddress(email); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱不合法"})
				return
			}
			if email != target.Email {
				if err := opts.Store.UpdateUserEmail(c.Request.Context(), target.ID, email); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
				changed = true
			}
		}

		if req.Username != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "账号名不可修改"})
			return
		}

		if req.Status != nil {
			status := *req.Status
			if status != 0 && status != 1 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "status 不合法"})
				return
			}
			if status != target.Status {
				if err := opts.Store.SetUserStatus(c.Request.Context(), target.ID, status); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
				changed = true
			}
		}

		if req.Role != nil {
			role := strings.TrimSpace(*req.Role)
			if role == "" {
				role = target.Role
			}
			if role != store.UserRoleUser && role != store.UserRoleRoot {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "role 不合法"})
				return
			}
			if role != target.Role {
				if err := opts.Store.SetUserRole(c.Request.Context(), target.ID, role); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
					return
				}
				changed = true
			}
		}

		if req.UserGroup != nil {
			mainGroup := strings.TrimSpace(*req.UserGroup)
			if mainGroup == "" {
				mainGroup = store.DefaultGroupName
			}
			if mainGroup != strings.TrimSpace(target.MainGroup) {
				if err := opts.Store.SetUserMainGroup(c.Request.Context(), target.ID, mainGroup); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
					return
				}
				changed = true
			}
		}

		msg := "已保存"
		if changed {
			// best-effort: bump updated_at based invalidation for future API auth.
			_, _ = opts.Store.GetUserByID(c.Request.Context(), target.ID)
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
	}
}

func adminResetUserPasswordHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Password string `json:"password"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}
		userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "user_id 不合法"})
			return
		}
		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		if strings.TrimSpace(req.Password) == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "新密码不能为空"})
			return
		}
		pwHash, err := auth.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if err := opts.Store.UpdateUserPasswordHash(c.Request.Context(), userID, pwHash); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}
		_ = opts.Store.DeleteSessionsByUserID(c.Request.Context(), userID)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "密码已重置，并已强制登出该用户"})
	}
}

func adminAddUserBalanceHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		AmountUSD string `json:"amount_usd"`
		Note      string `json:"note"`
	}

	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}

		userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "user_id 不合法"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		amountUSD, err := parseUSD(req.AmountUSD)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if amountUSD.LessThanOrEqual(decimal.Zero) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "金额必须大于 0"})
			return
		}

		start := time.Now()
		newBal, err := opts.Store.AddUserBalanceUSD(c.Request.Context(), userID, amountUSD)
		_ = start // reserved for audit latency in the future.
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "入账失败：" + err.Error()})
			return
		}
		_ = strings.TrimSpace(req.Note) // note currently best-effort: reserved for audit.

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "已入账",
			"data": gin.H{
				"balance_usd": formatUSDPlain(newBal),
			},
		})
	}
}

func adminDeleteUserHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if adminUsersFeatureDisabled(c, opts) {
			return
		}

		userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "user_id 不合法"})
			return
		}

		actorID, _ := userIDFromContext(c)
		if actorID > 0 && actorID == userID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "不能删除当前登录用户"})
			return
		}

		if err := opts.Store.DeleteUser(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
	}
}
