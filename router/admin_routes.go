package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/middleware"
	"realms/internal/store"
	"realms/internal/web"
)

func setAdminRoutes(r *gin.Engine, opts Options) {
	if opts.Admin == nil {
		return
	}

	selfMode := opts.SelfMode
	sessionCookieName := web.SessionCookieNameForSelfMode(selfMode)

	adminChain := func(h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		))
	}
	adminFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		))
	}
	adminFeatureChain2 := func(featureKey1, featureKey2 string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey1),
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey2),
			middleware.SessionAuth(opts.Store, sessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		))
	}
	adminUploadFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.SessionAuth(opts.Store, web.SessionCookieName),
			middleware.RequireRoles(store.UserRoleRoot),
			middleware.CSRF(),
		))
	}

	r.GET("/admin", adminChain(http.HandlerFunc(opts.Admin.Home)))

	r.GET("/admin/channels", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.Channels)))
	r.GET("/admin/channels/:channel_id/detail", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.ChannelDetail)))
	r.POST("/admin/channels", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CreateChannel)))
	r.POST("/admin/channels/:channel_id", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannel)))
	r.POST("/admin/channels/:channel_id/request_policy", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelRequestPolicy)))
	r.POST("/admin/channels/:channel_id/param_override", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelParamOverride)))
	r.POST("/admin/channels/:channel_id/header_override", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelHeaderOverride)))
	r.POST("/admin/channels/:channel_id/status_code_mapping", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelStatusCodeMapping)))
	r.POST("/admin/channels/:channel_id/model_suffix_preserve", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelModelSuffixPreserve)))
	r.POST("/admin/channels/:channel_id/request_body_blacklist", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelRequestBodyBlacklist)))
	r.POST("/admin/channels/:channel_id/request_body_whitelist", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelRequestBodyWhitelist)))
	r.POST("/admin/channels/:channel_id/meta", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelNewAPIMeta)))
	r.POST("/admin/channels/:channel_id/setting", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.UpdateChannelNewAPISetting)))

	r.GET("/admin/channel-groups", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.ChannelGroups)))
	r.POST("/admin/channel-groups", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.CreateChannelGroup)))
	r.POST("/admin/channel-groups/:group_id", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.UpdateChannelGroup)))
	r.GET("/admin/channel-groups/:group_id", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.ChannelGroupDetail)))
	r.POST("/admin/channel-groups/:group_id/children/groups", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.CreateChildChannelGroup)))
	r.POST("/admin/channel-groups/:group_id/children/channels", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.AddChannelGroupChannelMember)))
	r.POST("/admin/channel-groups/:group_id/children/groups/:child_group_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.DeleteChannelGroupGroupMember)))
	r.POST("/admin/channel-groups/:group_id/children/channels/:channel_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.DeleteChannelGroupChannelMember)))
	r.POST("/admin/channel-groups/:group_id/children/reorder", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.ReorderChannelGroupMembers)))
	r.POST("/admin/channel-groups/:group_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannelGroups, http.HandlerFunc(opts.Admin.DeleteChannelGroup)))

	r.POST("/admin/channels/reorder", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.ReorderChannels)))
	// 兼容：部分前端/代理可能无法正确透传 path 参数，允许从表单 channel_id 读取。
	r.POST("/admin/channels/test", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.TestChannel)))
	r.POST("/admin/channels/:channel_id/test", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.TestChannel)))
	r.POST("/admin/channels/:channel_id/promote", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.PinChannel)))
	r.POST("/admin/channels/:channel_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.DeleteChannel)))

	r.GET("/admin/channels/:channel_id/models", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.ChannelModels)))
	r.POST("/admin/channels/:channel_id/models", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.CreateChannelModel)))
	r.GET("/admin/channels/:channel_id/models/:binding_id", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.ChannelModel)))
	r.POST("/admin/channels/:channel_id/models/:binding_id", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.UpdateChannelModel)))
	r.POST("/admin/channels/:channel_id/models/:binding_id/delete", adminFeatureChain2(store.SettingFeatureDisableAdminChannels, store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.DeleteChannelModel)))

	r.GET("/admin/channels/:channel_id/endpoints", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.Endpoints)))
	r.POST("/admin/channels/:channel_id/endpoints", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CreateEndpoint)))
	r.POST("/admin/endpoints/:endpoint_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.DeleteEndpoint)))
	r.POST("/admin/endpoints/:endpoint_id/openai-credentials", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CreateOpenAICredential)))
	r.POST("/admin/endpoints/:endpoint_id/anthropic-credentials", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CreateAnthropicCredential)))
	r.POST("/admin/openai-credentials/:credential_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.DeleteOpenAICredential)))
	r.POST("/admin/anthropic-credentials/:credential_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.DeleteAnthropicCredential)))
	r.GET("/admin/endpoints/:endpoint_id/codex-accounts", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CodexAccounts)))
	r.POST("/admin/endpoints/:endpoint_id/codex-accounts", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CreateCodexAccount)))
	r.POST("/admin/endpoints/:endpoint_id/codex-accounts/refresh", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.RefreshCodexQuotasByEndpoint)))
	r.POST("/admin/endpoints/:endpoint_id/codex-oauth/start", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.StartCodexOAuth)))
	r.POST("/admin/endpoints/:endpoint_id/codex-oauth/complete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CompleteCodexOAuth)))
	r.POST("/admin/codex-accounts/:account_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.DeleteCodexAccount)))
	r.POST("/admin/codex-accounts/:account_id/refresh", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.RefreshCodexQuota)))

	// 批量注册API
	r.POST("/admin/batch-register", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.StartBatchRegister)))
	r.GET("/admin/batch-register-progress/:task_id", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.StreamBatchRegisterProgress)))
	r.POST("/admin/batch-register/:task_id/cancel", adminFeatureChain(store.SettingFeatureDisableAdminChannels, http.HandlerFunc(opts.Admin.CancelBatchRegister)))

	r.GET("/admin/users", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.Users)))
	r.POST("/admin/users", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.CreateUser)))
	r.POST("/admin/users/:user_id", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.UpdateUser)))
	r.POST("/admin/users/:user_id/balance", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.AddUserBalance)))
	r.POST("/admin/users/:user_id/profile", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.UpdateUserProfile)))
	r.POST("/admin/users/:user_id/password", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.UpdateUserPassword)))
	r.POST("/admin/users/:user_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminUsers, http.HandlerFunc(opts.Admin.DeleteUser)))

	if !selfMode {
		r.GET("/admin/subscriptions", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.Subscriptions)))
		r.POST("/admin/subscriptions", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.CreateSubscriptionPlan)))
		r.GET("/admin/subscriptions/:plan_id", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.SubscriptionPlan)))
		r.POST("/admin/subscriptions/:plan_id", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.UpdateSubscriptionPlan)))
		r.POST("/admin/subscriptions/:plan_id/delete", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.DeleteSubscriptionPlan)))

		r.GET("/admin/orders", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.Orders)))
		r.POST("/admin/orders/:order_id/approve", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.ApproveSubscriptionOrder)))
		r.POST("/admin/orders/:order_id/reject", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.RejectSubscriptionOrder)))

		r.GET("/admin/settings/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.PaymentChannels)))
		r.POST("/admin/settings/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.CreatePaymentChannel)))
		r.GET("/admin/settings/payment-channels/:payment_channel_id", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.PaymentChannel)))
		r.POST("/admin/settings/payment-channels/:payment_channel_id", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.UpdatePaymentChannel)))
		r.POST("/admin/settings/payment-channels/:payment_channel_id/delete", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.DeletePaymentChannel)))

		r.GET("/admin/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "/admin/settings/payment-channels"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
		})))
		r.POST("/admin/payment-channels", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.CreatePaymentChannel)))
		r.GET("/admin/payment-channels/:payment_channel_id", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "/admin/settings/payment-channels?edit=" + strings.TrimSpace(r.PathValue("payment_channel_id"))
			if r.URL.RawQuery != "" {
				target += "&" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
		})))
		r.POST("/admin/payment-channels/:payment_channel_id", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.UpdatePaymentChannel)))
		r.POST("/admin/payment-channels/:payment_channel_id/delete", adminFeatureChain(store.SettingFeatureDisableBilling, http.HandlerFunc(opts.Admin.DeletePaymentChannel)))
	}

	r.GET("/admin/usage", adminFeatureChain(store.SettingFeatureDisableAdminUsage, http.HandlerFunc(opts.Admin.Usage)))
	r.GET("/admin/usage/events/:event_id/detail", adminFeatureChain(store.SettingFeatureDisableAdminUsage, http.HandlerFunc(opts.Admin.UsageEventDetailAPI)))

	r.GET("/admin/settings", adminChain(http.HandlerFunc(opts.Admin.Settings)))
	r.POST("/admin/settings", adminChain(http.HandlerFunc(opts.Admin.UpdateSettings)))
	r.GET("/admin/backup", adminChain(http.HandlerFunc(opts.Admin.Backup)))
	r.GET("/admin/export", adminChain(http.HandlerFunc(opts.Admin.Export)))
	r.POST("/admin/import", adminChain(http.HandlerFunc(opts.Admin.Import)))

	r.GET("/admin/oauth-apps", adminChain(http.HandlerFunc(opts.Admin.OAuthApps)))
	r.POST("/admin/oauth-apps", adminChain(http.HandlerFunc(opts.Admin.CreateOAuthApp)))
	r.GET("/admin/oauth-apps/:app_id", adminChain(http.HandlerFunc(opts.Admin.OAuthApp)))
	r.POST("/admin/oauth-apps/:app_id", adminChain(http.HandlerFunc(opts.Admin.UpdateOAuthApp)))
	r.POST("/admin/oauth-apps/:app_id/rotate-secret", adminChain(http.HandlerFunc(opts.Admin.RotateOAuthAppSecret)))

	r.GET("/admin/models", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.Models)))
	r.POST("/admin/models", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.CreateModel)))
	r.POST("/admin/models/library-lookup", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.ModelLibraryLookup)))
	r.POST("/admin/models/import-pricing", adminUploadFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.ImportModelPricing)))
	r.GET("/admin/models/:model_id", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.Model)))
	r.POST("/admin/models/:model_id", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.UpdateModel)))
	r.POST("/admin/models/:model_id/delete", adminFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.Admin.DeleteModel)))

	if !selfMode {
		r.GET("/admin/tickets", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Admin.Tickets)))
		r.GET("/admin/tickets/:ticket_id", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Admin.Ticket)))
		r.POST("/admin/tickets/:ticket_id/reply", adminUploadFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Admin.ReplyTicket)))
		r.POST("/admin/tickets/:ticket_id/close", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Admin.CloseTicket)))
		r.POST("/admin/tickets/:ticket_id/reopen", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Admin.ReopenTicket)))
		r.GET("/admin/tickets/:ticket_id/attachments/:attachment_id", adminFeatureChain(store.SettingFeatureDisableTickets, http.HandlerFunc(opts.Admin.TicketAttachmentDownload)))
	}

	r.GET("/admin/announcements", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(opts.Admin.Announcements)))
	r.POST("/admin/announcements", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(opts.Admin.CreateAnnouncement)))
	r.POST("/admin/announcements/:announcement_id", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(opts.Admin.UpdateAnnouncementStatus)))
	r.POST("/admin/announcements/:announcement_id/delete", adminFeatureChain(store.SettingFeatureDisableAdminAnnouncements, http.HandlerFunc(opts.Admin.DeleteAnnouncement)))
}

