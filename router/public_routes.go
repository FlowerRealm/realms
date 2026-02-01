package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"realms/internal/middleware"
	"realms/internal/store"
)

func setAuthAndPublicRoutes(r *gin.Engine, opts Options) {
	selfMode := opts.SelfMode

	publicChain := func(h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.BodyCache(0),
		))
	}

	publicFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.BodyCache(0),
		))
	}

	r.POST("/oauth/token", publicChain(http.HandlerFunc(oauthTokenHandler(opts))))
	r.POST("/api/email/verification/send", publicChain(http.HandlerFunc(emailVerificationSendHandler(opts))))

	if opts.CodexOAuthHandler != nil {
		r.GET("/auth/callback", publicChain(opts.CodexOAuthHandler))
	}

	// webhooks & notify
	if !selfMode {
		if opts.SubscriptionOrderPaidWebhook != nil {
			r.POST("/api/webhooks/subscription-orders/:order_id/paid", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.SubscriptionOrderPaidWebhook)))
		}
		if opts.StripeWebhook != nil {
			r.POST("/api/pay/stripe/webhook", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.StripeWebhook)))
		}
		if opts.StripeWebhookByPaymentChannel != nil {
			r.POST("/api/pay/stripe/webhook/:payment_channel_id", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.StripeWebhookByPaymentChannel)))
		}
		if opts.EPayNotify != nil {
			r.GET("/api/pay/epay/notify", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.EPayNotify)))
		}
		if opts.EPayNotifyByPaymentChannel != nil {
			r.GET("/api/pay/epay/notify/:payment_channel_id", publicFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.EPayNotifyByPaymentChannel)))
		}
	}
}
