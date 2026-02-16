package router

import (
	"context"
	"io/fs"
	"net/http"

	openaiapi "realms/internal/api/openai"
	"realms/internal/config"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/tickets"
)

type Options struct {
	Store    *store.Store
	Sched    *scheduler.Scheduler
	SelfMode bool

	AllowOpenRegistration           bool
	EmailVerificationEnabledDefault bool
	PublicBaseURLDefault            string
	AdminTimeZoneDefault            string

	BillingDefault config.BillingConfig
	SMTPDefault    config.SMTPConfig
	TicketStorage  *tickets.Storage

	// frontend-backend-separation (new-api-aligned)
	FrontendBaseURL   string // optional; if set, non-API requests redirect to this base.
	FrontendDistDir   string // optional; e.g. "./web/dist" for serving static assets.
	FrontendIndexPage []byte // optional; when empty, SPA routes read dist/index.html at request time.
	FrontendFS        fs.FS  // optional; when set, static assets are served from this FS (typically go:embed).

	OpenAI *openaiapi.Handler

	// Optional.
	CodexOAuthHandler http.Handler

	// system
	Healthz       http.HandlerFunc
	RealmsIconSVG http.HandlerFunc
	FaviconICO    http.HandlerFunc

	// payments/webhooks (only mounted when !SelfMode in current routing)
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
