package router

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetRouter(r *gin.Engine, opts Options) {
	setSystemRoutes(r, opts)
	setAuthAndPublicRoutes(r, opts)
	setOpenAIRoutes(r, opts)

	api := r.Group("/api")
	api.Use(gzip.Gzip(gzip.DefaultCompression))
	if opts.PersonalMode {
		// personal 模式最小可用：仅保留上游渠道 + 用量统计所需 API。
		// - 鉴权：/v1/* 与 /api/admin/* 走管理 Key
		// - 管理面：前端仍通过 /api/user/self 获取虚拟用户态
		setUserAPIRoutes(api, opts)
		setPersonalAPIKeyAPIRoutes(api, opts)
		setChannelAPIRoutes(api, opts)
		setModelAPIRoutes(api, opts)
		setAdminAPIRoutes(api, opts)
	} else {
		setOAuthAPIRoutes(api, opts)
		setUserAPIRoutes(api, opts)
		setTokenAPIRoutes(api, opts)
		setChannelAPIRoutes(api, opts)
		setModelAPIRoutes(api, opts)
		setUsageAPIRoutes(api, opts)
		setDashboardAPIRoutes(api, opts)
		setAnnouncementAPIRoutes(api, opts)
		setAccountAPIRoutes(api, opts)
		setBillingAPIRoutes(api, opts)
		setTicketAPIRoutes(api, opts)
		setAdminAPIRoutes(api, opts)
	}

	setWebSPARoutes(r, opts)
}
