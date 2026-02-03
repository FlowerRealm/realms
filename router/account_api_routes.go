package router

import (
	"database/sql"
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/crypto"
	"realms/internal/store"
)

func setAccountAPIRoutes(r gin.IRoutes, opts Options) {
	authn := requireUserSession(opts)

	r.POST("/account/username", authn, accountUpdateUsernameHandler(opts))
	r.POST("/account/email", authn, accountUpdateEmailHandler(opts))
	r.POST("/account/password", authn, accountUpdatePasswordHandler(opts))
}

func accountUpdateUsernameHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "账号名不可修改"})
	}
}

func accountUpdateEmailHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		Email            string `json:"email"`
		VerificationCode string `json:"verification_code"`
	}
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
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
		if !emailVerifEnabled {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "当前环境未启用邮箱验证码，无法修改邮箱"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		email := strings.TrimSpace(strings.ToLower(req.Email))
		code := strings.TrimSpace(req.VerificationCode)
		if email == "" || code == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "新邮箱与验证码不能为空"})
			return
		}
		if _, err := mail.ParseAddress(email); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱地址不合法"})
			return
		}
		if email == strings.ToLower(u.Email) {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
			return
		}
		other, err := opts.Store.GetUserByEmail(c.Request.Context(), email)
		if err == nil && other.ID != u.ID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "邮箱地址已被占用"})
			return
		}
		if err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询邮箱失败"})
			return
		}

		ok, err = opts.Store.ConsumeEmailVerification(c.Request.Context(), email, crypto.TokenHash(code))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "验证码校验失败"})
			return
		}
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "验证码无效或已过期"})
			return
		}

		if err := opts.Store.UpdateUserEmail(c.Request.Context(), u.ID, email); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		sess := sessions.Default(c)
		sess.Clear()
		_ = sess.Save()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "邮箱已更新，请重新登录",
			"data":    gin.H{"force_logout": true},
		})
	}
}

func accountUpdatePasswordHandler(opts Options) gin.HandlerFunc {
	type reqBody struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	return func(c *gin.Context) {
		userID, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			return
		}
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户查询失败"})
			return
		}

		var req reqBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
			return
		}
		oldPassword := strings.TrimSpace(req.OldPassword)
		newPassword := strings.TrimSpace(req.NewPassword)
		if oldPassword == "" || newPassword == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "旧密码与新密码不能为空"})
			return
		}
		if !auth.CheckPassword(u.PasswordHash, oldPassword) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "旧密码不正确"})
			return
		}

		pwHash, err := auth.HashPassword(newPassword)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if err := opts.Store.UpdateUserPasswordHash(c.Request.Context(), u.ID, pwHash); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败"})
			return
		}

		sess := sessions.Default(c)
		sess.Clear()
		_ = sess.Save()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密码已更新，请重新登录",
			"data":    gin.H{"force_logout": true},
		})
	}
}
