package router

import (
	"io/fs"
	"net/http"

	"realms/internal/admin"
	"realms/internal/config"
	openaiapi "realms/internal/api/openai"
	"realms/internal/store"
	"realms/internal/tickets"
	"realms/internal/web"
)

type Options struct {
	Store    *store.Store
	SelfMode bool

	AllowOpenRegistration         bool
	EmailVerificationEnabledDefault bool
	PublicBaseURLDefault            string
	AdminTimeZoneDefault            string

	BillingDefault config.BillingConfig
	PaymentDefault config.PaymentConfig
	SMTPDefault    config.SMTPConfig
	TicketStorage  *tickets.Storage

	// frontend-backend-separation (new-api-aligned)
	FrontendBaseURL   string // optional; if set, non-API requests redirect to this base.
	FrontendDistDir   string // optional; e.g. "./web/dist" for serving static assets.
	FrontendIndexPage []byte // optional; returned for SPA routes when FrontendBaseURL is empty.
	FrontendFS        fs.FS  // optional; when set, static assets are served from this FS (typically go:embed).

	Web    *web.Server
	Admin  *admin.Server
	OpenAI *openaiapi.Handler

	// Optional.
	CodexOAuthHandler http.Handler

	// system
	Healthz       http.HandlerFunc
	Version       http.HandlerFunc
	RealmsIconSVG http.HandlerFunc
	FaviconICO    http.HandlerFunc

	// payments/webhooks (only mounted when !SelfMode in current routing)
	SubscriptionOrderPaidWebhook  http.HandlerFunc
	StripeWebhook                 http.HandlerFunc
	StripeWebhookByPaymentChannel http.HandlerFunc
	EPayNotify                    http.HandlerFunc
	EPayNotifyByPaymentChannel    http.HandlerFunc

	// codex/admin
	RefreshCodexQuotasByEndpoint http.HandlerFunc
	RefreshCodexQuota            http.HandlerFunc
}
