package router

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	rlmcrypto "realms/internal/crypto"
	"realms/internal/store"
)

const sessionUserUpdatedAtKey = "user_updated_at_unix"

const systemAdminUserID int64 = 0

func extractBearer(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func extractPresentedAPIKey(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	raw := extractBearer(c.GetHeader("Authorization"))
	if raw != "" {
		return raw, true
	}
	raw = strings.TrimSpace(c.GetHeader("x-api-key"))
	if raw != "" {
		return raw, true
	}
	return "", false
}

func applyPrincipalContext(c *gin.Context, p auth.Principal) {
	if c == nil {
		return
	}
	c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
	c.Set("rlm_user_id", p.UserID)
	c.Set("rlm_user_role", p.Role)
}

func requireRoot(opts Options) gin.HandlerFunc {
	sessionAuth := requireRootSession(opts)
	return func(c *gin.Context) {
		rawKey, hasKey := extractPresentedAPIKey(c)
		if hasKey {
			if len(opts.AdminAPIKeyHash) > 0 && subtle.ConstantTimeCompare(rlmcrypto.TokenHash(rawKey), opts.AdminAPIKeyHash) == 1 {
				p := auth.Principal{
					ActorType: auth.ActorTypeToken,
					UserID:    systemAdminUserID,
					Role:      store.UserRoleRoot,
				}
				applyPrincipalContext(c, p)
				c.Next()
				return
			}
			if _, ok := sessionUserID(c); !ok {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Key 无效"})
				c.Abort()
				return
			}
		}
		sessionAuth(c)
	}
}

func requireUserSession(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := sessionUserID(c)
		if !ok {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			c.Abort()
			return
		}

		// 防止 CSRF：要求 Realms-User header 匹配登录用户（跨站请求难以伪造该自定义 header）。
		realmsUser := strings.TrimSpace(c.GetHeader("Realms-User"))
		headerID, err := strconv.ParseInt(realmsUser, 10, 64)
		if err != nil || headerID <= 0 || headerID != userID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无权进行此操作，Realms-User 无效"})
			c.Abort()
			return
		}

		if opts.Store == nil {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			c.Abort()
			return
		}

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil || u.ID <= 0 || u.Status != 1 {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			c.Abort()
			return
		}

		// 当用户关键字段更新（例如邮箱/密码/角色/状态）后，强制旧会话失效，避免“已登出但 cookie 仍有效”。
		if staleSession(c, u) {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "会话已失效，请重新登录"})
			c.Abort()
			return
		}

		role := strings.TrimSpace(u.Role)
		p := auth.Principal{
			ActorType: auth.ActorTypeSession,
			UserID:    userID,
			Role:      role,
		}
		applyPrincipalContext(c, p)
		c.Next()
	}
}

func requireRootSession(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := sessionUserID(c)
		if !ok {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			c.Abort()
			return
		}

		realmsUser := strings.TrimSpace(c.GetHeader("Realms-User"))
		headerID, err := strconv.ParseInt(realmsUser, 10, 64)
		if err != nil || headerID <= 0 || headerID != userID {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无权进行此操作，Realms-User 无效"})
			c.Abort()
			return
		}

		if opts.Store == nil {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			c.Abort()
			return
		}

		u, err := opts.Store.GetUserByID(c.Request.Context(), userID)
		if err != nil || u.ID <= 0 || u.Status != 1 {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "未登录"})
			c.Abort()
			return
		}
		if strings.TrimSpace(u.Role) != store.UserRoleRoot {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "权限不足"})
			c.Abort()
			return
		}
		if staleSession(c, u) {
			clearSession(c)
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "会话已失效，请重新登录"})
			c.Abort()
			return
		}

		role := strings.TrimSpace(u.Role)
		p := auth.Principal{
			ActorType: auth.ActorTypeSession,
			UserID:    userID,
			Role:      role,
		}
		applyPrincipalContext(c, p)
		c.Next()
	}
}

func actorIDFromContext(c *gin.Context) (int64, bool) {
	if c == nil {
		return 0, false
	}
	v, ok := c.Get("rlm_user_id")
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	default:
		return 0, false
	}
}

func adminActorIDFromContext(c *gin.Context) (int64, bool) {
	return actorIDFromContext(c)
}

func isSystemAdminContext(c *gin.Context) bool {
	actorID, ok := actorIDFromContext(c)
	return ok && actorID == systemAdminUserID
}

func staleSession(c *gin.Context, u store.User) bool {
	if c == nil || u.ID <= 0 {
		return true
	}
	raw := sessions.Default(c).Get(sessionUserUpdatedAtKey)
	var unix int64
	switch x := raw.(type) {
	case int64:
		unix = x
	case int:
		unix = int64(x)
	case float64:
		unix = int64(x)
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
			unix = n
		}
	}
	if unix <= 0 {
		// 兼容旧 session：不强制失效。
		return false
	}
	return u.UpdatedAt.UTC().Unix() > unix
}

func clearSession(c *gin.Context) {
	if c == nil {
		return
	}
	sess := sessions.Default(c)
	sess.Clear()
	_ = sess.Save()
}

func userIDFromContext(c *gin.Context) (int64, bool) {
	userID, ok := actorIDFromContext(c)
	return userID, ok && userID > 0
}
