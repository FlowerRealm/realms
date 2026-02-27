package router

import "github.com/gin-gonic/gin"

func setAdminAPIRoutes(r gin.IRoutes, opts Options) {
	admin := r.(*gin.RouterGroup).Group("/admin")
	admin.Use(requireRoot(opts))

	// personal 模式收敛：保留最小管理面 API（渠道管理走 /api/channel*；设置与用量统计走 /api/admin/*）。
	if opts.PersonalMode {
		setAdminUsageAPIRoutes(admin, opts)
		setAdminMCPAPIRoutes(admin, opts)
		setAdminSettingsAPIRoutes(admin, opts)
		setAdminPersonalConfigAPIRoutes(admin, opts)
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
