package router

import (
	"context"
	"io/fs"
	"net/http"

	openaiapi "realms/internal/api/openai"
	"realms/internal/channeltest"
	"realms/internal/config"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/tickets"
)

type Options struct {
	Store           *store.Store
	Sched           *scheduler.Scheduler
	AdminAPIKeyHash []byte

	EmailVerificationEnabledDefault bool
	AdminTimeZoneDefault            string

	BillingDefault config.BillingConfig
	SMTPDefault    config.SMTPConfig
	TicketStorage  *tickets.Storage

	FrontendIndexPage []byte // optional; when empty, SPA routes use fallback index.
	FrontendFS        fs.FS  // optional; when set, static assets are served from this FS (typically go:embed).

	// ChannelTestCLIRunnerURL 为 CLI Runner 服务地址；非空时启用 CLI 渠道测试。
	ChannelTestCLIRunnerURL   string
	ChannelTestCLIConcurrency int
	ChannelTestProbe          channeltest.Prober

	OpenAI *openaiapi.Handler

	// Optional.
	CodexOAuthHandler http.Handler

	// system
	Healthz       http.HandlerFunc
	RealmsIconSVG http.HandlerFunc
	FaviconICO    http.HandlerFunc

	// payments/webhooks
	SubscriptionOrderPaidWebhook  http.HandlerFunc
	StripeWebhookByPaymentChannel http.HandlerFunc
	EPayNotifyByPaymentChannel    http.HandlerFunc

	// codex/admin
	RefreshCodexQuotasByEndpoint http.HandlerFunc
	RefreshCodexQuota            http.HandlerFunc

	// codex/oauth (SPA APIs)
	StartCodexOAuth                func(ctx context.Context, endpointID int64, actorUserID int64) (string, error)
	CompleteCodexOAuth             func(ctx context.Context, endpointID int64, actorUserID int64, state string, code string) error
	RefreshCodexQuotasByEndpointID func(ctx context.Context, endpointID int64) error
	RefreshCodexQuotaByAccountID   func(ctx context.Context, accountID int64) error
}
