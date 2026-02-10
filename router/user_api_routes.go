package router

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/crypto"
	"realms/internal/store"
)

type userLoginRequest struct {
	Login    string `json:"login"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userRegisterRequest struct {
	Email            string `json:"email"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	VerificationCode string `json:"verification_code"`
}

func setUserAPIRoutes(r gin.IRoutes, opts Options) {
	r.POST("/user/register", userRegisterHandler(opts))
	r.POST("/user/login", userLoginHandler(opts))
	r.GET("/user/logout", userLogoutHandler())
	r.GET("/user/self", userSelfHandler(opts))
}

func userLoginHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req userLoginRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		login := strings.TrimSpace(req.Login)
		if login == "" {
			login = strings.TrimSpace(req.Username)
		}
		if login == "" {
			login = strings.TrimSpace(req.Email)
		}
		password := req.Password
		if login == "" || password == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}

		// email: 统一按小写匹配；username: 大小写敏感匹配。
		u, err := opts.Store.GetUserByEmail(c.Request.Context(), strings.ToLower(login))
		if err != nil && err == sql.ErrNoRows {
			u, err = opts.Store.GetUserByUsername(c.Request.Context(), login)
		}
		if err != nil || !auth.CheckPassword(u.PasswordHash, password) || u.Status != 1 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱/账号名或密码错误"})
			return
		}

		sess := sessions.Default(c)
		sess.Set("id", u.ID)
		sess.Set("username", u.Username)
		sess.Set("role", u.Role)
		sess.Set("status", u.Status)
		sess.Set(sessionUserUpdatedAtKey, u.UpdatedAt.UTC().Unix())
		if err := sess.Save(); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无法保存会话信息，请重试"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"id":       u.ID,
				"email":    u.Email,
				"username": u.Username,
				"role":     u.Role,
				"status":   u.Status,
			},
		})
	}
}

func userRegisterHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !opts.AllowOpenRegistration {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "当前环境未开放注册"})
			return
		}
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		var req userRegisterRequest
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
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

		// 账号名占用检查（保持与 SSR 注册逻辑一致）
		if _, err := opts.Store.GetUserByUsername(c.Request.Context(), username); err == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "账号名已被占用"})
			return
		} else if err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询账号名失败"})
			return
		}

		emailVerifEnabled := opts.EmailVerificationEnabledDefault
		if v, ok, err := opts.Store.GetBoolAppSetting(c.Request.Context(), store.SettingEmailVerificationEnable); err == nil && ok {
			emailVerifEnabled = v
		}
		if emailVerifEnabled {
			code := strings.TrimSpace(req.VerificationCode)
			if code == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "验证码不能为空"})
				return
			}
			ok, err := opts.Store.ConsumeEmailVerification(c.Request.Context(), email, crypto.TokenHash(code))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "验证码校验失败"})
				return
			}
			if !ok {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "验证码无效或已过期"})
				return
			}
		}

		pwHash, err := auth.HashPassword(password)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		role := store.UserRoleUser
		userCount, err := opts.Store.CountUsers(c.Request.Context())
		if err == nil && userCount == 0 {
			role = store.UserRoleRoot
		}
		userID, err := opts.Store.CreateUser(c.Request.Context(), email, username, pwHash, role)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "创建用户失败（可能邮箱或账号名已存在）"})
			return
		}

		sess := sessions.Default(c)
		sess.Set("id", userID)
		sess.Set("username", username)
		sess.Set("role", role)
		sess.Set("status", 1)
		if u, err := opts.Store.GetUserByID(c.Request.Context(), userID); err == nil && u.ID > 0 {
			sess.Set(sessionUserUpdatedAtKey, u.UpdatedAt.UTC().Unix())
		}
		if err := sess.Save(); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无法保存会话信息，请重试"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"id":       userID,
				"email":    email,
				"username": username,
				"role":     role,
				"status":   1,
			},
		})
	}
}

func userLogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessions.Default(c)
		sess.Clear()
		if err := sess.Save(); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无法清理会话，请重试"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	}
}

func userSelfHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}

		userID, ok := sessionUserID(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}

		emailVerifEnabled := opts.EmailVerificationEnabledDefault
		if v, ok, err := opts.Store.GetBoolAppSetting(c.Request.Context(), store.SettingEmailVerificationEnable); err == nil && ok {
			emailVerifEnabled = v
		}
		features := opts.Store.FeatureStateEffective(c.Request.Context(), opts.SelfMode)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"id":                         u.ID,
				"email":                      u.Email,
				"username":                   u.Username,
				"role":                       u.Role,
				"status":                     u.Status,
				"self_mode":                  opts.SelfMode,
				"email_verification_enabled": emailVerifEnabled,
				"features": gin.H{
					"web_announcements_disabled":    features.WebAnnouncementsDisabled,
					"web_tokens_disabled":           features.WebTokensDisabled,
					"web_usage_disabled":            features.WebUsageDisabled,
					"models_disabled":               features.ModelsDisabled,
					"billing_disabled":              features.BillingDisabled,
					"tickets_disabled":              features.TicketsDisabled,
					"admin_channels_disabled":       features.AdminChannelsDisabled,
					"admin_channel_groups_disabled": features.AdminChannelGroupsDisabled,
					"admin_users_disabled":          features.AdminUsersDisabled,
					"admin_usage_disabled":          features.AdminUsageDisabled,
					"admin_announcements_disabled":  features.AdminAnnouncementsDisabled,
				},
			},
		})
	}
}
