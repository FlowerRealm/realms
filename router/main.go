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

	setWebSPARoutes(r, opts)
}
