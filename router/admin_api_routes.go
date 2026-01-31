package router

import "github.com/gin-gonic/gin"

func setAdminAPIRoutes(r gin.IRoutes, opts Options) {
	admin := r.(*gin.RouterGroup).Group("/admin")
	admin.Use(requireRootSession(opts))

	setAdminHomeAPIRoutes(admin, opts)
	setAdminChannelGroupAPIRoutes(admin, opts)
	setAdminUserAPIRoutes(admin, opts)
	setAdminAnnouncementAPIRoutes(admin, opts)
	setAdminBillingAPIRoutes(admin, opts)
	setAdminUsageAPIRoutes(admin, opts)
	setAdminTicketAPIRoutes(admin, opts)
	setAdminOAuthAppAPIRoutes(admin, opts)
	setAdminSettingsAPIRoutes(admin, opts)
	setAdminPaymentChannelAPIRoutes(admin, opts)
}
