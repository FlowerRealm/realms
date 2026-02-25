package router

import "github.com/gin-gonic/gin"

func setAdminAPIRoutes(r gin.IRoutes, opts Options) {
	admin := r.(*gin.RouterGroup).Group("/admin")
	admin.Use(requireRoot(opts))

	// 自用模式收敛：仅保留“用量统计”管理面 API（渠道管理走 /api/channel*）。
	if opts.SelfMode {
		setAdminUsageAPIRoutes(admin, opts)
		return
	}

	setAdminHomeAPIRoutes(admin, opts)
	setAdminChannelGroupAPIRoutes(admin, opts)
	setAdminMainGroupAPIRoutes(admin, opts)
	setAdminUserAPIRoutes(admin, opts)
	setAdminAnnouncementAPIRoutes(admin, opts)
	setAdminBillingAPIRoutes(admin, opts)
	setAdminUsageAPIRoutes(admin, opts)
	setAdminTicketAPIRoutes(admin, opts)
	setAdminOAuthAppAPIRoutes(admin, opts)
	setAdminSettingsAPIRoutes(admin, opts)
	setAdminPaymentChannelAPIRoutes(admin, opts)
}
