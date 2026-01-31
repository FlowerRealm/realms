package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"realms/internal/middleware"
	"realms/internal/store"
	"realms/internal/web"
)

func setWebRoutes(r *gin.Engine, opts Options) {
	if opts.Web == nil {
		return
	}

	selfMode := opts.SelfMode
	sessionCookieName := web.SessionCookieNameForSelfMode(selfMode)

	webChain := func(h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.StripWebQuery,
			middleware.FlashFromCookies,
		))
	}
	webFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.StripWebQuery,
			middleware.FlashFromCookies,
		))
	}
	webCSRFChain := func(h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.StripWebQuery,
			middleware.FlashFromCookies,
			middleware.CSRF(),
		))
	}
	webCSRFFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.StripWebQuery,
			middleware.FlashFromCookies,
			middleware.CSRF(),
		))
	}
	webUploadFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.StripWebQuery,
			middleware.FlashFromCookies,
			middleware.CSRF(),
		))
	}

	r.GET("/oauth/authorize", webChain(http.HandlerFunc(opts.Web.OAuthAuthorizePage)))
	r.POST("/oauth/authorize", webCSRFChain(http.HandlerFunc(opts.Web.OAuthAuthorize)))

	r.GET("/dashboard", webChain(http.HandlerFunc(opts.Web.Dashboard)))

	r.GET("/announcements", webFeatureChain(store.SettingFeatureDisableWebAnnouncements, http.HandlerFunc(opts.Web.AnnouncementsPage)))
	r.GET("/announcements/:announcement_id", webFeatureChain(store.SettingFeatureDisableWebAnnouncements, http.HandlerFunc(opts.Web.AnnouncementDetailPage)))
	r.POST("/announcements/:announcement_id/read", webCSRFFeatureChain(store.SettingFeatureDisableWebAnnouncements, http.HandlerFunc(opts.Web.AnnouncementMarkRead)))

	r.GET("/account", webChain(http.HandlerFunc(opts.Web.AccountPage)))
	r.POST("/account/username", webCSRFChain(http.HandlerFunc(opts.Web.AccountUpdateUsername)))
	r.POST("/account/email", webCSRFChain(http.HandlerFunc(opts.Web.AccountUpdateEmail)))
	r.POST("/account/password", webCSRFChain(http.HandlerFunc(opts.Web.AccountUpdatePassword)))

	r.GET("/tokens", webFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(opts.Web.TokensPage)))
	r.POST("/tokens/new", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(opts.Web.CreateToken)))
	r.POST("/tokens/rotate", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(opts.Web.RotateToken)))
	r.POST("/tokens/revoke", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(opts.Web.RevokeToken)))
	r.POST("/tokens/delete", webCSRFFeatureChain(store.SettingFeatureDisableWebTokens, http.HandlerFunc(opts.Web.DeleteToken)))

	r.GET("/models", webFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Web.ModelsPage)))

	if !selfMode {
		r.GET("/subscription", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.SubscriptionPage)))
		r.POST("/subscription/purchase", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.PurchaseSubscription)))

		r.GET("/topup", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.TopupPage)))
		r.POST("/topup/create", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.CreateTopupOrder)))

		r.GET("/pay/:kind/:order_id", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.PayPage)))
		r.GET("/pay/:kind/:order_id/success", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.PayReturnSuccess)))
		r.GET("/pay/:kind/:order_id/cancel", webFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.PayReturnCancel)))
		r.POST("/pay/:kind/:order_id/start", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.StartPayment)))
		r.POST("/pay/:kind/:order_id/cancel", webCSRFFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Web.CancelPayOrder)))
	}

	r.GET("/usage", webFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(opts.Web.UsagePage)))
	r.GET("/usage/before/:cursor_id", webFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(opts.Web.UsageBeforePage)))
	r.GET("/usage/after/:cursor_id", webFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(opts.Web.UsageAfterPage)))
	r.GET("/usage/events/:event_id/detail", webFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(opts.Web.UsageEventDetailAPI)))
	r.POST("/usage/filter", webCSRFFeatureChain(store.SettingFeatureDisableWebUsage, http.HandlerFunc(opts.Web.UsageFilter)))

	if !selfMode {
		r.GET("/tickets", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.TicketsPage)))
		r.GET("/tickets/open", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.TicketsOpenPage)))
		r.GET("/tickets/closed", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.TicketsClosedPage)))
		r.GET("/tickets/new", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.TicketNewPage)))
		r.POST("/tickets/new", webUploadFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.CreateTicket)))
		r.GET("/tickets/:ticket_id", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.TicketDetailPage)))
		r.POST("/tickets/:ticket_id/reply", webUploadFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.ReplyTicket)))
		r.GET("/tickets/:ticket_id/attachments/:attachment_id", webFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Web.TicketAttachmentDownload)))
	}

	r.POST("/logout", webCSRFChain(http.HandlerFunc(opts.Web.Logout)))
}

