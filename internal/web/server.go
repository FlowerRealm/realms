// Package web 实现服务端渲染的最小 Web 控制台：注册/登录/会话/令牌管理。
package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	stripeCheckout "github.com/stripe/stripe-go/v81/checkout/session"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/crypto"
	"realms/internal/icons"
	"realms/internal/scheduler"
	"realms/internal/security"
	"realms/internal/store"
	ticketspkg "realms/internal/tickets"
)

const SessionCookieName = "realms_session"

type Server struct {
	store               *store.Store
	sched               *scheduler.Scheduler
	exec                UpstreamDoer
	selfMode            bool
	allowRegister       bool
	disableSecureCookie bool
	publicBaseURL       string
	trustProxyHeaders   bool
	trustedProxies      []netip.Prefix
	billingDefault      config.BillingConfig
	paymentDefault      config.PaymentConfig
	smtpDefault         config.SMTPConfig
	emailVerifDefault   bool
	ticketsCfg          config.TicketsConfig
	ticketStorage       *ticketspkg.Storage
	tmpl                *template.Template
}

type UpstreamDoer interface {
	Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)
}

func NewServer(st *store.Store, sched *scheduler.Scheduler, exec UpstreamDoer, selfMode bool, allowRegister bool, disableSecureCookie bool, billingDefault config.BillingConfig, paymentDefault config.PaymentConfig, smtpDefault config.SMTPConfig, emailVerifDefault bool, publicBaseURL string, trustProxyHeaders bool, trustedProxyCIDRs []string, ticketsCfg config.TicketsConfig, ticketStorage *ticketspkg.Storage) (*Server, error) {
	if ticketStorage == nil {
		ticketStorage = ticketspkg.NewStorage(ticketsCfg.AttachmentsDir)
	}
	t, err := template.New("web").Funcs(template.FuncMap{
		"modelIconURL": icons.ModelIconURL,
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	var trustedProxies []netip.Prefix
	for _, raw := range trustedProxyCIDRs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		pfx, err := netip.ParsePrefix(s)
		if err != nil {
			// 兼容单个 IP（不带 /32 或 /128）。
			addr, err2 := netip.ParseAddr(s)
			if err2 != nil {
				return nil, fmt.Errorf("解析 trusted_proxy_cidrs[%q] 失败: %w", s, err)
			}
			pfx = netip.PrefixFrom(addr, addr.BitLen())
		}
		trustedProxies = append(trustedProxies, pfx)
	}

	return &Server{
		store:               st,
		sched:               sched,
		exec:                exec,
		selfMode:            selfMode,
		allowRegister:       allowRegister,
		disableSecureCookie: disableSecureCookie,
		publicBaseURL:       strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		trustProxyHeaders:   trustProxyHeaders,
		trustedProxies:      trustedProxies,
		billingDefault:      billingDefault,
		paymentDefault:      paymentDefault,
		smtpDefault:         smtpDefault,
		emailVerifDefault:   emailVerifDefault,
		ticketsCfg:          ticketsCfg,
		ticketStorage:       ticketStorage,
		tmpl:                t,
	}, nil
}

func (s *Server) emailVerificationEnabled(ctx context.Context) bool {
	v, ok, err := s.store.GetBoolAppSetting(ctx, store.SettingEmailVerificationEnable)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingEmailVerificationEnable, "err", err)
		return s.emailVerifDefault
	}
	if ok {
		return v
	}
	return s.emailVerifDefault
}

func (s *Server) smtpConfigEffective(ctx context.Context) config.SMTPConfig {
	cfg := s.smtpDefault
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}

	server, ok, err := s.store.GetStringAppSetting(ctx, store.SettingSMTPServer)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingSMTPServer, "err", err)
	} else if ok {
		cfg.SMTPServer = server
	}
	port, ok, err := s.store.GetIntAppSetting(ctx, store.SettingSMTPPort)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingSMTPPort, "err", err)
	} else if ok {
		cfg.SMTPPort = port
	}
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}
	ssl, ok, err := s.store.GetBoolAppSetting(ctx, store.SettingSMTPSSLEnabled)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingSMTPSSLEnabled, "err", err)
	} else if ok {
		cfg.SMTPSSLEnabled = ssl
	}
	account, ok, err := s.store.GetStringAppSetting(ctx, store.SettingSMTPAccount)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingSMTPAccount, "err", err)
	} else if ok {
		cfg.SMTPAccount = account
	}
	from, ok, err := s.store.GetStringAppSetting(ctx, store.SettingSMTPFrom)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingSMTPFrom, "err", err)
	} else if ok {
		cfg.SMTPFrom = from
	}
	token, ok, err := s.store.GetStringAppSetting(ctx, store.SettingSMTPToken)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingSMTPToken, "err", err)
	} else if ok {
		cfg.SMTPToken = token
	}
	return cfg
}

func (s *Server) billingConfigEffective(ctx context.Context) config.BillingConfig {
	cfg := s.billingDefault

	enable, ok, err := s.store.GetBoolAppSetting(ctx, store.SettingBillingEnablePayAsYouGo)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingBillingEnablePayAsYouGo, "err", err)
	} else if ok {
		cfg.EnablePayAsYouGo = enable
	}

	minTopup, ok, err := s.store.GetDecimalAppSetting(ctx, store.SettingBillingMinTopupCNY)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingBillingMinTopupCNY, "err", err)
	} else if ok {
		cfg.MinTopupCNY = minTopup
	}

	creditRatio, ok, err := s.store.GetDecimalAppSetting(ctx, store.SettingBillingCreditUSDPerCNY)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingBillingCreditUSDPerCNY, "err", err)
	} else if ok {
		cfg.CreditUSDPerCNY = creditRatio
	}

	return cfg
}

func (s *Server) paymentConfigEffective(ctx context.Context) config.PaymentConfig {
	cfg := s.paymentDefault

	enableEPay, ok, err := s.store.GetBoolAppSetting(ctx, store.SettingPaymentEPayEnable)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentEPayEnable, "err", err)
	} else if ok {
		cfg.EPay.Enable = enableEPay
	}
	gateway, ok, err := s.store.GetStringAppSetting(ctx, store.SettingPaymentEPayGateway)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentEPayGateway, "err", err)
	} else if ok {
		cfg.EPay.Gateway = gateway
	}
	partnerID, ok, err := s.store.GetStringAppSetting(ctx, store.SettingPaymentEPayPartnerID)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentEPayPartnerID, "err", err)
	} else if ok {
		cfg.EPay.PartnerID = partnerID
	}
	key, ok, err := s.store.GetStringAppSetting(ctx, store.SettingPaymentEPayKey)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentEPayKey, "err", err)
	} else if ok {
		cfg.EPay.Key = key
	}

	enableStripe, ok, err := s.store.GetBoolAppSetting(ctx, store.SettingPaymentStripeEnable)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentStripeEnable, "err", err)
	} else if ok {
		cfg.Stripe.Enable = enableStripe
	}
	currency, ok, err := s.store.GetStringAppSetting(ctx, store.SettingPaymentStripeCurrency)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentStripeCurrency, "err", err)
	} else if ok {
		cfg.Stripe.Currency = currency
	}
	secret, ok, err := s.store.GetStringAppSetting(ctx, store.SettingPaymentStripeSecretKey)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentStripeSecretKey, "err", err)
	} else if ok {
		cfg.Stripe.SecretKey = secret
	}
	webhookSecret, ok, err := s.store.GetStringAppSetting(ctx, store.SettingPaymentStripeWebhookSecret)
	if err != nil {
		slog.Error("读取 app_settings 失败", "key", store.SettingPaymentStripeWebhookSecret, "err", err)
	} else if ok {
		cfg.Stripe.WebhookSecret = webhookSecret
	}

	cfg.Stripe.Currency = strings.ToLower(strings.TrimSpace(cfg.Stripe.Currency))
	if cfg.Stripe.Currency == "" {
		cfg.Stripe.Currency = "cny"
	}

	return cfg
}

type UserView struct {
	ID       int64
	Email    string
	Username string
	Role     string
}

type TokenView struct {
	ID        int64
	Name      string
	TokenHint string
	Status    int
}

type UsageWindowView struct {
	Window       string
	Since        string
	Until        string
	UsedUSD      string
	CommittedUSD string
	ReservedUSD  string
	LimitUSD     string
	RemainingUSD string
	UsedPercent  int // 0-100

	RequestCount int64
	TotalTokens  int64
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
	CacheHitRate string // e.g. "15.5%"
	RPM          string // Requests Per Minute
	TPM          string // Tokens Per Minute
}

type UsageEventView struct {
	ID                int64
	Time              string
	Endpoint          string
	Method            string
	Model             string
	StatusCode        string
	LatencyMS         string
	InputTokens       string
	OutputTokens      string
	CachedTokens      string
	RequestBytes      string
	ResponseBytes     string
	CostUSD           string
	State             string
	StateLabel        string
	StateBadgeClass   string
	RequestID         string
	UpstreamChannelID string
	Error             string
	ErrorClass        string
	ErrorMessage      string
	IsStream          bool
}

type ModelView struct {
	ID                  string
	OwnedBy             string
	InputUSDPer1M       string
	OutputUSDPer1M      string
	CacheInputUSDPer1M  string
	CacheOutputUSDPer1M string
}

type SubscriptionView struct {
	Active       bool
	PlanName     string
	PriceCNY     string
	GroupName    string
	StartAt      string
	EndAt        string
	UsageWindows []UsageWindowView
}

type SubscriptionOrderView struct {
	ID         int64
	PlanName   string
	AmountCNY  string
	Status     string
	CreatedAt  string
	PaidAt     string
	ApprovedAt string
}

type TopupOrderView struct {
	ID        int64
	AmountCNY string
	CreditUSD string
	Status    string
	CreatedAt string
	PaidAt    string
}

type PayOrderView struct {
	Kind      string
	ID        int64
	Title     string
	AmountCNY string
	CreditUSD string
	Status    string
	CreatedAt string
}

type PaymentChannelView struct {
	ID        int64
	Type      string
	TypeLabel string
	Name      string
}

type PlanView struct {
	ID           int64
	Name         string
	PriceCNY     string
	GroupName    string
	Limit5H      string
	Limit1D      string
	Limit7D      string
	Limit30D     string
	DurationDays int
}

type TemplateData struct {
	Title                    string
	ContentHTML              template.HTML
	Error                    string
	Notice                   string
	Next                     string
	AllowRegister            bool
	SelfMode                 bool
	Features                 store.FeatureState
	EmailVerificationEnabled bool
	User                     *UserView
	CSRFToken                string
	Tokens                   []TokenView
	Token                    string
	BaseURL                  string
	UsageWindows             []UsageWindowView
	UsageStart               string
	UsageEnd                 string
	UsageEvents              []UsageEventView
	UsageNextBeforeID        string
	UsagePrevAfterID         string
	UsageCursorActive        bool
	UsageLimit               int
	Models                   []ModelView
	Subscription             *SubscriptionView
	Subscriptions            []SubscriptionView
	Plans                    []PlanView
	SubscriptionOrders       []SubscriptionOrderView
	BalanceUSD               string
	PayAsYouGoEnabled        bool
	TodayUsageUSD            string
	TodayRequests            int64
	TodayTokens              int64
	TodayRPM                 int
	TodayTPM                 int
	DashboardCharts          DashboardChartsView
	TopupMinCNY              string
	TopupOrders              []TopupOrderView
	PayOrder                 *PayOrderView
	PaymentChannels          []PaymentChannelView
	PaymentStripeEnabled     bool
	PaymentEPayEnabled       bool

	Tickets        []TicketListItemView
	Ticket         *TicketDetailView
	TicketMessages []TicketMessageView

	Announcements            []AnnouncementListItemView
	Announcement             *AnnouncementDetailView
	UnreadAnnouncementsCount int
	UnreadAnnouncement       *AnnouncementDetailView

	OAuthAppName             string
	OAuthClientID            string
	OAuthRedirectURI         string
	OAuthScope               string
	OAuthState               string
	OAuthCodeChallenge       string
	OAuthCodeChallengeMethod string
}

type DashboardChartsView struct {
	ModelStats      []ModelUsageView      `json:"model_stats"`
	TimeSeriesStats []TimeSeriesUsageView `json:"time_series_stats"`
}

type ModelUsageView struct {
	Model        string `json:"model"`
	IconURL      string `json:"icon_url"`
	Color        string `json:"color"`
	Requests     int64  `json:"requests"`
	Tokens       int64  `json:"tokens"`
	CommittedUSD string `json:"committed_usd"`
}

type TimeSeriesUsageView struct {
	Label        string  `json:"label"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	CommittedUSD float64 `json:"committed_usd"`
}

func userViewFromUser(u store.User) *UserView {
	return &UserView{
		ID:       u.ID,
		Email:    u.Email,
		Username: u.Username,
		Role:     u.Role,
	}
}

func (s *Server) withFeatures(ctx context.Context, data TemplateData) TemplateData {
	data.Features = s.store.FeatureStateEffective(ctx, s.selfMode)
	return data
}

func (s *Server) Render(w http.ResponseWriter, name string, data TemplateData) {
	data.SelfMode = s.selfMode

	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("渲染模板失败", "err", err)
		http.Error(w, "页面渲染失败", http.StatusInternalServerError)
		return
	}
	data.ContentHTML = template.HTML(buf.String())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "base", data); err != nil {
		slog.Error("渲染模板失败", "err", err)
		http.Error(w, "页面渲染失败", http.StatusInternalServerError)
		return
	}
}

func (s *Server) Index(w http.ResponseWriter, r *http.Request) {
	if r.URL != nil && r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (s *Server) LoginPage(w http.ResponseWriter, r *http.Request) {
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	next := sanitizeNextPath(r.URL.Query().Get("next"))
	s.Render(w, "page_login", TemplateData{
		Title:         "登录 - Realms",
		Notice:        notice,
		Next:          next,
		AllowRegister: s.allowRegister,
	})
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	next := sanitizeNextPath(r.FormValue("next"))
	login := strings.TrimSpace(strings.ToLower(r.FormValue("login")))
	if login == "" {
		login = strings.TrimSpace(strings.ToLower(r.FormValue("email"))) // 兼容旧字段
	}
	password := r.FormValue("password")
	if login == "" || password == "" {
		s.Render(w, "page_login", TemplateData{Title: "登录 - Realms", Error: "邮箱或密码不能为空", Next: next, AllowRegister: s.allowRegister})
		return
	}
	u, err := s.store.GetUserByEmail(r.Context(), login)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		u, err = s.store.GetUserByUsername(r.Context(), login)
	}
	if err != nil || !auth.CheckPassword(u.PasswordHash, password) || u.Status != 1 {
		s.Render(w, "page_login", TemplateData{Title: "登录 - Realms", Error: "邮箱/账号名或密码错误", Next: next, AllowRegister: s.allowRegister})
		return
	}
	if err := s.issueSession(w, r, u.ID); err != nil {
		http.Error(w, "创建会话失败", http.StatusInternalServerError)
		return
	}
	target := "/dashboard"
	if next != "" {
		target = next
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) RegisterPage(w http.ResponseWriter, r *http.Request) {
	if !s.allowRegister {
		http.Error(w, "当前环境未开放注册", http.StatusForbidden)
		return
	}
	s.Render(w, "page_register", TemplateData{
		Title:                    "注册 - Realms",
		EmailVerificationEnabled: s.emailVerificationEnabled(r.Context()),
	})
}

func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	if !s.allowRegister {
		http.Error(w, "当前环境未开放注册", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	emailVerifEnabled := s.emailVerificationEnabled(r.Context())
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	username, err := store.NormalizeUsername(r.FormValue("username"))
	if err != nil {
		s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: err.Error(), EmailVerificationEnabled: emailVerifEnabled})
		return
	}
	password := r.FormValue("password")
	if email == "" || password == "" {
		s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: "邮箱或密码不能为空", EmailVerificationEnabled: emailVerifEnabled})
		return
	}
	if _, err := s.store.GetUserByUsername(r.Context(), username); err == nil {
		s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: "账号名已被占用", EmailVerificationEnabled: emailVerifEnabled})
		return
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "查询账号名失败", http.StatusInternalServerError)
		return
	}
	if emailVerifEnabled {
		code := strings.TrimSpace(r.FormValue("verification_code"))
		if code == "" {
			s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: "验证码不能为空", EmailVerificationEnabled: emailVerifEnabled})
			return
		}
		ok, err := s.store.ConsumeEmailVerification(r.Context(), email, crypto.TokenHash(code))
		if err != nil {
			http.Error(w, "验证码校验失败", http.StatusInternalServerError)
			return
		}
		if !ok {
			s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: "验证码无效或已过期", EmailVerificationEnabled: emailVerifEnabled})
			return
		}
	}
	pwHash, err := auth.HashPassword(password)
	if err != nil {
		s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: err.Error(), EmailVerificationEnabled: emailVerifEnabled})
		return
	}

	role := store.UserRoleUser
	userCount, err := s.store.CountUsers(r.Context())
	if err == nil && userCount == 0 {
		role = store.UserRoleRoot
	}
	userID, err := s.store.CreateUser(r.Context(), email, username, pwHash, role)
	if err != nil {
		s.Render(w, "page_register", TemplateData{Title: "注册 - Realms", Error: "创建用户失败（可能邮箱或账号名已存在）", EmailVerificationEnabled: emailVerifEnabled})
		return
	}
	if err := s.issueSession(w, r, userID); err != nil {
		http.Error(w, "创建会话失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (s *Server) Dashboard(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	unreadCount, err := s.store.CountUnreadAnnouncements(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "查询公告失败", http.StatusInternalServerError)
		return
	}

	var unreadAnn *AnnouncementDetailView
	if unreadCount > 0 {
		a, err := s.store.GetLatestUnreadAnnouncement(r.Context(), p.UserID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "查询公告失败", http.StatusInternalServerError)
			return
		}
		if err == nil {
			unreadAnn = &AnnouncementDetailView{
				ID:        a.ID,
				Title:     a.Title,
				Body:      a.Body,
				CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
			}
		}
	}

	billingCfg := s.billingConfigEffective(r.Context())
	balanceUSD, err := s.store.GetUserBalanceUSD(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "余额查询失败", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	subs, err := s.store.ListNonExpiredSubscriptionsWithPlans(r.Context(), u.ID, now)
	var subView *SubscriptionView
	if err == nil {
		var activeID int64
		var activeEnd time.Time
		for _, row := range subs {
			if row.Subscription.StartAt.After(now) {
				continue
			}
			if activeID == 0 || row.Subscription.EndAt.Before(activeEnd) {
				activeID = row.Subscription.ID
				activeEnd = row.Subscription.EndAt
			}
		}
		for _, row := range subs {
			if row.Subscription.ID == activeID {
				subView = &SubscriptionView{
					Active:   true,
					PlanName: row.Plan.Name,
					EndAt:    row.Subscription.EndAt.Format("2006-01-02 15:04"),
				}
				// Get 5h window usage for simple display
				since := now.Add(-5 * time.Hour)
				if row.Subscription.StartAt.After(since) {
					since = row.Subscription.StartAt
				}
				committed, reserved, err := s.store.SumCommittedAndReservedUSDBySubscription(r.Context(), store.UsageSumWithReservedBySubscriptionInput{
					UserID:         u.ID,
					SubscriptionID: row.Subscription.ID,
					Since:          since,
					Now:            now,
				})
				if err == nil && !row.Plan.Limit5HUSD.IsZero() {
					used := committed.Add(reserved)
					percent := int(used.Mul(decimal.NewFromInt(100)).Div(row.Plan.Limit5HUSD).IntPart())
					if percent > 100 {
						percent = 100
					}
					subView.UsageWindows = append(subView.UsageWindows, UsageWindowView{
						Window:      "5小时",
						UsedUSD:     formatUSD(used),
						LimitUSD:    formatUSD(row.Plan.Limit5HUSD),
						UsedPercent: percent,
					})
				}
				break
			}
		}
	}

	// Calculate today's usage (since midnight Asia/Shanghai)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	todayStart := time.Now().In(loc)
	todayStart = time.Date(todayStart.Year(), todayStart.Month(), todayStart.Day(), 0, 0, 0, 0, loc)

	todayCommitted, todayReserved, err := s.store.SumCommittedAndReservedUSD(r.Context(), store.UsageSumWithReservedInput{
		UserID: u.ID,
		Since:  todayStart,
		Now:    now,
	})
	todayUsageUSD := "0"
	if err == nil {
		todayUsageUSD = formatUSD(todayCommitted.Add(todayReserved))
	}

	todayStats, _ := s.store.GetUsageTokenStatsByUserRange(r.Context(), u.ID, todayStart, now)

	// RPM/TPM (last 5 minutes)
	fiveMinAgo := now.Add(-5 * time.Minute)
	recentStats, _ := s.store.GetUsageTokenStatsByUserRange(r.Context(), u.ID, fiveMinAgo, now)
	rpm := int(recentStats.Requests / 5)
	tpm := int(recentStats.Tokens / 5)

	// Charts data (Today)
	modelStats, _ := s.store.GetUsageStatsByModelRange(r.Context(), u.ID, todayStart, now)
	timeStats, _ := s.store.GetUsageTimeSeriesRange(r.Context(), u.ID, todayStart, now)

	// Fill missing hours to ensure 24 hours display
	timeMap := make(map[string]store.TimeSeriesUsageStats)
	for _, ts := range timeStats {
		timeMap[ts.Time.In(loc).Format("15:00")] = ts
	}

	palette := []string{"#6366f1", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#ec4899", "#06b6d4", "#84cc16", "#14b8a6", "#64748b"}
	chartView := DashboardChartsView{
		// Ensure JSON uses [] instead of null, so the template JS can safely call .map().
		ModelStats:      make([]ModelUsageView, 0),
		TimeSeriesStats: make([]TimeSeriesUsageView, 0, 24),
	}
	for i, m := range modelStats {
		color := palette[i%len(palette)]
		chartView.ModelStats = append(chartView.ModelStats, ModelUsageView{
			Model:        m.Model,
			IconURL:      icons.ModelIconURL(m.Model, ""),
			Color:        color,
			Requests:     m.Requests,
			Tokens:       m.Tokens,
			CommittedUSD: formatUSDPlain(m.CommittedUSD),
		})
	}

	for i := 0; i < 24; i++ {
		hr := time.Date(todayStart.Year(), todayStart.Month(), todayStart.Day(), i, 0, 0, 0, loc)
		label := hr.Format("15:00")
		if ts, ok := timeMap[label]; ok {
			f, _ := ts.CommittedUSD.Float64()
			chartView.TimeSeriesStats = append(chartView.TimeSeriesStats, TimeSeriesUsageView{
				Label:        label,
				Requests:     ts.Requests,
				Tokens:       ts.Tokens,
				CommittedUSD: f,
			})
		} else {
			chartView.TimeSeriesStats = append(chartView.TimeSeriesStats, TimeSeriesUsageView{
				Label:        label,
				Requests:     0,
				Tokens:       0,
				CommittedUSD: 0,
			})
		}
	}

	s.Render(w, "page_dashboard", s.withFeatures(r.Context(), TemplateData{
		Title:                    "控制台 - Realms",
		User:                     userViewFromUser(u),
		CSRFToken:                csrfToken(p),
		BaseURL:                  s.baseURLFromRequest(r),
		UnreadAnnouncementsCount: int(unreadCount),
		UnreadAnnouncement:       unreadAnn,
		BalanceUSD:               formatUSD(balanceUSD),
		PayAsYouGoEnabled:        billingCfg.EnablePayAsYouGo,
		Subscription:             subView,
		TodayUsageUSD:            todayUsageUSD,
		TodayRequests:            todayStats.Requests,
		TodayTokens:              todayStats.Tokens,
		TodayRPM:                 rpm,
		TodayTPM:                 tpm,
		DashboardCharts:          chartView,
	}))
}

func (s *Server) AccountPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	s.Render(w, "page_account", s.withFeatures(r.Context(), TemplateData{
		Title:                    "账号设置 - Realms",
		User:                     userViewFromUser(u),
		CSRFToken:                csrfToken(p),
		EmailVerificationEnabled: s.emailVerificationEnabled(r.Context()),
	}))
}

func (s *Server) AccountUpdateUsername(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	username, err := store.NormalizeUsername(r.FormValue("username"))
	if err != nil {
		s.renderAccountPage(w, r, u, err.Error())
		return
	}
	if u.Username == username {
		http.Redirect(w, r, "/account", http.StatusFound)
		return
	}
	other, err := s.store.GetUserByUsername(r.Context(), username)
	if err == nil && other.ID != u.ID {
		s.renderAccountPage(w, r, u, "账号名已被占用")
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "查询账号名失败", http.StatusInternalServerError)
		return
	}

	if err := s.store.UpdateUserUsername(r.Context(), u.ID, username); err != nil {
		s.renderAccountPage(w, r, u, "保存失败")
		return
	}
	s.forceLogoutUser(w, r, u.ID, "账号名已更新，请重新登录")
}

func (s *Server) AccountUpdateEmail(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	if !s.emailVerificationEnabled(r.Context()) {
		s.renderAccountPage(w, r, u, "当前环境未启用邮箱验证码，无法修改邮箱")
		return
	}

	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	code := strings.TrimSpace(r.FormValue("verification_code"))
	if email == "" || code == "" {
		s.renderAccountPage(w, r, u, "新邮箱与验证码不能为空")
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		s.renderAccountPage(w, r, u, "邮箱地址不合法")
		return
	}
	if email == strings.ToLower(u.Email) {
		http.Redirect(w, r, "/account", http.StatusFound)
		return
	}
	if other, err := s.store.GetUserByEmail(r.Context(), email); err == nil && other.ID != u.ID {
		s.renderAccountPage(w, r, u, "邮箱地址已被占用")
		return
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "查询邮箱失败", http.StatusInternalServerError)
		return
	}

	ok, err := s.store.ConsumeEmailVerification(r.Context(), email, crypto.TokenHash(code))
	if err != nil {
		http.Error(w, "验证码校验失败", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.renderAccountPage(w, r, u, "验证码无效或已过期")
		return
	}

	if err := s.store.UpdateUserEmail(r.Context(), u.ID, email); err != nil {
		s.renderAccountPage(w, r, u, "保存失败")
		return
	}
	s.forceLogoutUser(w, r, u.ID, "邮箱已更新，请重新登录")
}

func (s *Server) AccountUpdatePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	oldPassword := r.FormValue("old_password")
	newPassword := r.FormValue("new_password")
	if strings.TrimSpace(oldPassword) == "" || strings.TrimSpace(newPassword) == "" {
		s.renderAccountPage(w, r, u, "旧密码与新密码不能为空")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, oldPassword) {
		s.renderAccountPage(w, r, u, "旧密码不正确")
		return
	}
	pwHash, err := auth.HashPassword(newPassword)
	if err != nil {
		s.renderAccountPage(w, r, u, err.Error())
		return
	}
	if err := s.store.UpdateUserPasswordHash(r.Context(), u.ID, pwHash); err != nil {
		s.renderAccountPage(w, r, u, "保存失败")
		return
	}
	s.forceLogoutUser(w, r, u.ID, "密码已更新，请重新登录")
}

func (s *Server) TokensPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	tokens, err := s.store.ListUserTokens(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "令牌查询失败", http.StatusInternalServerError)
		return
	}

	var tokenViews []TokenView
	for _, t := range tokens {
		tv := TokenView{ID: t.ID, Status: t.Status}
		if t.Name != nil {
			tv.Name = *t.Name
		}
		if t.TokenHint != nil {
			tv.TokenHint = *t.TokenHint
		}
		tokenViews = append(tokenViews, tv)
	}

	s.Render(w, "page_tokens", s.withFeatures(r.Context(), TemplateData{
		Title:     "API 令牌 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrfToken(p),
		Tokens:    tokenViews,
		BaseURL:   s.baseURLFromRequest(r),
	}))
}

func (s *Server) SubscriptionPage(w http.ResponseWriter, r *http.Request) {
	s.subscriptionPage(w, r, "")
}

func (s *Server) TopupPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	billingCfg := s.billingConfigEffective(r.Context())
	payCfg := s.paymentConfigEffective(r.Context())

	balanceUSD, err := s.store.GetUserBalanceUSD(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "余额查询失败", http.StatusInternalServerError)
		return
	}

	orders, err := s.store.ListTopupOrdersByUser(r.Context(), u.ID, 50)
	if err != nil {
		http.Error(w, "订单查询失败", http.StatusInternalServerError)
		return
	}
	var views []TopupOrderView
	for _, o := range orders {
		status := "未知"
		switch o.Status {
		case store.TopupOrderStatusPending:
			status = "待支付"
		case store.TopupOrderStatusPaid:
			status = "已入账"
		case store.TopupOrderStatusCanceled:
			status = "已取消"
		}
		v := TopupOrderView{
			ID:        o.ID,
			AmountCNY: formatCNY(o.AmountCNY),
			CreditUSD: formatUSD(o.CreditUSD),
			Status:    status,
			CreatedAt: o.CreatedAt.Format("2006-01-02 15:04"),
		}
		if o.PaidAt != nil {
			v.PaidAt = o.PaidAt.Format("2006-01-02 15:04")
		}
		views = append(views, v)
	}

	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}

	hasStripeChannel := false
	hasEPayChannel := false
	if rows, err := s.store.ListPaymentChannels(r.Context()); err == nil {
		for _, ch := range rows {
			if ch.Status != 1 {
				continue
			}
			switch ch.Type {
			case store.PaymentChannelTypeStripe:
				if ch.StripeSecretKey == nil || strings.TrimSpace(*ch.StripeSecretKey) == "" || ch.StripeWebhookSecret == nil || strings.TrimSpace(*ch.StripeWebhookSecret) == "" {
					continue
				}
				hasStripeChannel = true
			case store.PaymentChannelTypeEPay:
				if ch.EPayGateway == nil || strings.TrimSpace(*ch.EPayGateway) == "" || ch.EPayPartnerID == nil || strings.TrimSpace(*ch.EPayPartnerID) == "" || ch.EPayKey == nil || strings.TrimSpace(*ch.EPayKey) == "" {
					continue
				}
				hasEPayChannel = true
			}
			if hasStripeChannel && hasEPayChannel {
				break
			}
		}
	} else {
		slog.Error("读取 payment_channels 失败", "err", err)
	}

	stripeEnabled := hasStripeChannel || (payCfg.Stripe.Enable && strings.TrimSpace(payCfg.Stripe.SecretKey) != "" && strings.TrimSpace(payCfg.Stripe.WebhookSecret) != "")
	epayEnabled := hasEPayChannel || (payCfg.EPay.Enable && strings.TrimSpace(payCfg.EPay.Gateway) != "" && strings.TrimSpace(payCfg.EPay.PartnerID) != "" && strings.TrimSpace(payCfg.EPay.Key) != "")

	minTopupCNY := billingCfg.MinTopupCNY

	s.Render(w, "page_topup", s.withFeatures(r.Context(), TemplateData{
		Title:                "余额充值 - Realms",
		User:                 userViewFromUser(u),
		Notice:               notice,
		Error:                errMsg,
		CSRFToken:            csrfToken(p),
		BaseURL:              s.baseURLFromRequest(r),
		BalanceUSD:           formatUSD(balanceUSD),
		PayAsYouGoEnabled:    billingCfg.EnablePayAsYouGo,
		TopupMinCNY:          formatCNY(minTopupCNY),
		TopupOrders:          views,
		PaymentStripeEnabled: stripeEnabled,
		PaymentEPayEnabled:   epayEnabled,
	}))
}

func (s *Server) CreateTopupOrder(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	amountRaw := strings.TrimSpace(r.FormValue("amount_cny"))
	amountCNY, err := parseCNY(amountRaw)
	if err != nil {
		http.Redirect(w, r, "/topup?err="+url.QueryEscape("金额不合法（示例：10.00）："+err.Error()), http.StatusFound)
		return
	}
	if amountCNY.LessThanOrEqual(decimal.Zero) {
		http.Redirect(w, r, "/topup?err="+url.QueryEscape("金额必须大于 0"), http.StatusFound)
		return
	}

	cfg := s.billingConfigEffective(r.Context())
	minTopupCNY := cfg.MinTopupCNY
	if minTopupCNY.GreaterThan(decimal.Zero) && amountCNY.LessThan(minTopupCNY) {
		http.Redirect(w, r, "/topup?err="+url.QueryEscape("金额不能小于最低充值："+formatCNY(minTopupCNY)), http.StatusFound)
		return
	}
	if cfg.CreditUSDPerCNY.LessThanOrEqual(decimal.Zero) {
		http.Redirect(w, r, "/topup?err="+url.QueryEscape("未配置充值入账比例"), http.StatusFound)
		return
	}
	creditUSD := amountCNY.Mul(cfg.CreditUSDPerCNY).Truncate(store.USDScale)

	o, err := s.store.CreateTopupOrder(r.Context(), u.ID, amountCNY, creditUSD, time.Now())
	if err != nil {
		http.Redirect(w, r, "/topup?err="+url.QueryEscape("创建订单失败："+err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/pay/topup/"+strconv.FormatInt(o.ID, 10)+"?msg="+url.QueryEscape("订单已创建，请选择支付方式"), http.StatusFound)
}

func (s *Server) PayPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	kind := strings.TrimSpace(r.PathValue("kind"))
	orderID, err := parseInt64(strings.TrimSpace(r.PathValue("order_id")))
	if err != nil || orderID <= 0 {
		http.NotFound(w, r)
		return
	}

	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}

	baseURL := s.baseURLFromRequest(r)

	var paymentChannels []PaymentChannelView
	hasStripeChannel := false
	hasEPayChannel := false
	if rows, err := s.store.ListPaymentChannels(r.Context()); err == nil {
		for _, ch := range rows {
			if ch.Status != 1 {
				continue
			}
			switch ch.Type {
			case store.PaymentChannelTypeStripe:
				if ch.StripeSecretKey == nil || strings.TrimSpace(*ch.StripeSecretKey) == "" || ch.StripeWebhookSecret == nil || strings.TrimSpace(*ch.StripeWebhookSecret) == "" {
					continue
				}
				paymentChannels = append(paymentChannels, PaymentChannelView{
					ID:        ch.ID,
					Type:      ch.Type,
					TypeLabel: "Stripe",
					Name:      ch.Name,
				})
				hasStripeChannel = true
			case store.PaymentChannelTypeEPay:
				if ch.EPayGateway == nil || strings.TrimSpace(*ch.EPayGateway) == "" || ch.EPayPartnerID == nil || strings.TrimSpace(*ch.EPayPartnerID) == "" || ch.EPayKey == nil || strings.TrimSpace(*ch.EPayKey) == "" {
					continue
				}
				paymentChannels = append(paymentChannels, PaymentChannelView{
					ID:        ch.ID,
					Type:      ch.Type,
					TypeLabel: "EPay",
					Name:      ch.Name,
				})
				hasEPayChannel = true
			}
		}
	} else {
		slog.Error("读取 payment_channels 失败", "err", err)
	}

	payCfg := s.paymentConfigEffective(r.Context())
	stripeEnabled := hasStripeChannel || (payCfg.Stripe.Enable && strings.TrimSpace(payCfg.Stripe.SecretKey) != "" && strings.TrimSpace(payCfg.Stripe.WebhookSecret) != "")
	epayEnabled := hasEPayChannel || (payCfg.EPay.Enable && strings.TrimSpace(payCfg.EPay.Gateway) != "" && strings.TrimSpace(payCfg.EPay.PartnerID) != "" && strings.TrimSpace(payCfg.EPay.Key) != "")

	view := PayOrderView{Kind: kind, ID: orderID}

	switch kind {
	case "subscription":
		o, err := s.store.GetSubscriptionOrderByID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "订单查询失败", http.StatusInternalServerError)
			return
		}
		if o.UserID != u.ID {
			http.NotFound(w, r)
			return
		}
		plan, err := s.store.GetSubscriptionPlanByID(r.Context(), o.PlanID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "套餐查询失败", http.StatusInternalServerError)
			return
		}

		view.Title = "订阅购买"
		if err == nil && strings.TrimSpace(plan.Name) != "" {
			view.Title = "订阅购买 - " + strings.TrimSpace(plan.Name)
		}
		view.AmountCNY = formatCNY(o.AmountCNY)
		view.CreatedAt = o.CreatedAt.Format("2006-01-02 15:04")
		switch o.Status {
		case store.SubscriptionOrderStatusPending:
			view.Status = "待支付"
		case store.SubscriptionOrderStatusActive:
			view.Status = "已生效"
		case store.SubscriptionOrderStatusCanceled:
			view.Status = "已取消"
		default:
			view.Status = "未知"
		}
	case "topup":
		o, err := s.store.GetTopupOrderByID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "订单查询失败", http.StatusInternalServerError)
			return
		}
		if o.UserID != u.ID {
			http.NotFound(w, r)
			return
		}

		view.Title = "余额充值"
		view.AmountCNY = formatCNY(o.AmountCNY)
		view.CreditUSD = formatUSD(o.CreditUSD)
		view.CreatedAt = o.CreatedAt.Format("2006-01-02 15:04")
		switch o.Status {
		case store.TopupOrderStatusPending:
			view.Status = "待支付"
		case store.TopupOrderStatusPaid:
			view.Status = "已入账"
		case store.TopupOrderStatusCanceled:
			view.Status = "已取消"
		default:
			view.Status = "未知"
		}
	default:
		http.NotFound(w, r)
		return
	}

	s.Render(w, "page_pay", s.withFeatures(r.Context(), TemplateData{
		Title:                "支付 - Realms",
		User:                 userViewFromUser(u),
		Notice:               notice,
		Error:                errMsg,
		CSRFToken:            csrfToken(p),
		BaseURL:              baseURL,
		PayOrder:             &view,
		PaymentChannels:      paymentChannels,
		PaymentStripeEnabled: stripeEnabled,
		PaymentEPayEnabled:   epayEnabled,
	}))
}

func (s *Server) CancelPayOrder(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	kind := strings.TrimSpace(r.PathValue("kind"))
	orderID, err := parseInt64(strings.TrimSpace(r.PathValue("order_id")))
	if err != nil || orderID <= 0 {
		http.NotFound(w, r)
		return
	}

	switch kind {
	case "subscription":
		if err := s.store.CancelSubscriptionOrderByUser(r.Context(), u.ID, orderID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
			return
		}
	case "topup":
		if err := s.store.CancelTopupOrderByUser(r.Context(), u.ID, orderID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape(err.Error()), http.StatusFound)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?msg="+url.QueryEscape("订单已取消。若您已完成支付，请联系管理员处理退款。"), http.StatusFound)
}

func (s *Server) StartPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	kind := strings.TrimSpace(r.PathValue("kind"))
	orderID, err := parseInt64(strings.TrimSpace(r.PathValue("order_id")))
	if err != nil || orderID <= 0 {
		http.NotFound(w, r)
		return
	}

	paymentChannelIDRaw := strings.TrimSpace(r.FormValue("payment_channel_id"))
	paymentChannelID := int64(0)
	if paymentChannelIDRaw != "" {
		id, err := parseInt64(paymentChannelIDRaw)
		if err != nil || id <= 0 {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("支付渠道不合法"), http.StatusFound)
			return
		}
		paymentChannelID = id
	}

	method := strings.ToLower(strings.TrimSpace(r.FormValue("method")))
	if paymentChannelID == 0 && method == "" {
		http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("请选择支付方式"), http.StatusFound)
		return
	}

	baseURL := s.baseURLFromRequest(r)

	ref := ""
	orderTitle := ""
	amountCNY := decimal.Zero
	switch kind {
	case "subscription":
		o, err := s.store.GetSubscriptionOrderByID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "订单查询失败", http.StatusInternalServerError)
			return
		}
		if o.UserID != u.ID {
			http.NotFound(w, r)
			return
		}
		if o.Status != store.SubscriptionOrderStatusPending {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("订单状态不可支付"), http.StatusFound)
			return
		}

		plan, err := s.store.GetSubscriptionPlanByID(r.Context(), o.PlanID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "套餐查询失败", http.StatusInternalServerError)
			return
		}
		orderTitle = "订阅购买"
		if err == nil && strings.TrimSpace(plan.Name) != "" {
			orderTitle = "订阅购买 - " + strings.TrimSpace(plan.Name)
		}
		amountCNY = o.AmountCNY
		ref = "sub_" + strconv.FormatInt(orderID, 10)
	case "topup":
		o, err := s.store.GetTopupOrderByID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "订单查询失败", http.StatusInternalServerError)
			return
		}
		if o.UserID != u.ID {
			http.NotFound(w, r)
			return
		}
		if o.Status != store.TopupOrderStatusPending {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("订单状态不可支付"), http.StatusFound)
			return
		}

		orderTitle = "余额充值"
		amountCNY = o.AmountCNY
		ref = "topup_" + strconv.FormatInt(orderID, 10)
	default:
		http.NotFound(w, r)
		return
	}

	if amountCNY.LessThanOrEqual(decimal.Zero) {
		http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("订单金额不合法"), http.StatusFound)
		return
	}

	unitAmount, err := cnyToMinorUnits(amountCNY)
	if err != nil || unitAmount <= 0 {
		http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("订单金额不合法"), http.StatusFound)
		return
	}

	successURL := baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10) + "?msg=" + url.QueryEscape("支付已完成，正在入账/生效，请稍后刷新查看状态。")
	cancelURL := baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10) + "?msg=" + url.QueryEscape("已取消支付。")

	if paymentChannelID > 0 {
		ch, err := s.store.GetPaymentChannelByID(r.Context(), paymentChannelID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("支付渠道不存在"), http.StatusFound)
				return
			}
			http.Error(w, "支付渠道查询失败", http.StatusInternalServerError)
			return
		}
		if ch.Status != 1 {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("支付渠道未启用"), http.StatusFound)
			return
		}

		switch ch.Type {
		case store.PaymentChannelTypeStripe:
			if ch.StripeSecretKey == nil || strings.TrimSpace(*ch.StripeSecretKey) == "" || ch.StripeWebhookSecret == nil || strings.TrimSpace(*ch.StripeWebhookSecret) == "" {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("Stripe 渠道未配置或不可用"), http.StatusFound)
				return
			}
			currency := "cny"
			if ch.StripeCurrency != nil {
				currency = strings.ToLower(strings.TrimSpace(*ch.StripeCurrency))
			}
			if currency == "" {
				currency = "cny"
			}

			stripe.Key = strings.TrimSpace(*ch.StripeSecretKey)

			exp := time.Now().Add(2 * time.Hour).Unix()
			params := &stripe.CheckoutSessionParams{
				SuccessURL:        stripe.String(successURL),
				CancelURL:         stripe.String(cancelURL),
				Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
				ClientReferenceID: stripe.String(ref),
				CustomerEmail:     stripe.String(u.Email),
				ExpiresAt:         stripe.Int64(exp),
				LineItems: []*stripe.CheckoutSessionLineItemParams{
					{
						PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
							Currency:   stripe.String(currency),
							UnitAmount: stripe.Int64(unitAmount),
							ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
								Name: stripe.String(orderTitle),
							},
						},
						Quantity: stripe.Int64(1),
					},
				},
			}
			sess, err := stripeCheckout.New(params)
			if err != nil || strings.TrimSpace(sess.URL) == "" {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("创建 Stripe 支付失败"), http.StatusFound)
				return
			}
			http.Redirect(w, r, sess.URL, http.StatusFound)
			return
		case store.PaymentChannelTypeEPay:
			if ch.EPayGateway == nil || strings.TrimSpace(*ch.EPayGateway) == "" || ch.EPayPartnerID == nil || strings.TrimSpace(*ch.EPayPartnerID) == "" || ch.EPayKey == nil || strings.TrimSpace(*ch.EPayKey) == "" {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("EPay 渠道未配置或不可用"), http.StatusFound)
				return
			}

			epayType := strings.ToLower(strings.TrimSpace(r.FormValue("epay_type")))
			if epayType == "" {
				epayType = "alipay"
			}
			switch epayType {
			case "alipay", "wxpay", "qqpay":
			default:
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("EPay 支付类型不支持"), http.StatusFound)
				return
			}

			client, err := epay.NewClient(&epay.Config{
				PartnerID: strings.TrimSpace(*ch.EPayPartnerID),
				Key:       strings.TrimSpace(*ch.EPayKey),
			}, strings.TrimSpace(*ch.EPayGateway))
			if err != nil {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("EPay 配置错误"), http.StatusFound)
				return
			}

			notifyURL, err := url.Parse(baseURL + "/api/pay/epay/notify/" + strconv.FormatInt(paymentChannelID, 10))
			if err != nil {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("回调 URL 配置错误"), http.StatusFound)
				return
			}
			returnURL, err := url.Parse(baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10))
			if err != nil {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("回跳 URL 配置错误"), http.StatusFound)
				return
			}

			money := formatCNYFixed(amountCNY)
			purchaseURL, params, err := client.Purchase(&epay.PurchaseArgs{
				Type:           epayType,
				ServiceTradeNo: ref,
				Name:           orderTitle,
				Money:          money,
				Device:         epay.PC,
				NotifyUrl:      notifyURL,
				ReturnUrl:      returnURL,
			})
			if err != nil {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("创建 EPay 支付失败"), http.StatusFound)
				return
			}

			u2, err := url.Parse(purchaseURL)
			if err != nil {
				http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("创建 EPay 支付失败"), http.StatusFound)
				return
			}
			q := u2.Query()
			for k, v := range params {
				q.Set(k, v)
			}
			u2.RawQuery = q.Encode()

			http.Redirect(w, r, u2.String(), http.StatusFound)
			return
		default:
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("支付渠道类型不支持"), http.StatusFound)
			return
		}
	}

	payCfg := s.paymentConfigEffective(r.Context())
	switch method {
	case "stripe":
		cfg := payCfg.Stripe
		if !cfg.Enable || strings.TrimSpace(cfg.SecretKey) == "" || strings.TrimSpace(cfg.WebhookSecret) == "" {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("Stripe 未配置或未启用"), http.StatusFound)
			return
		}
		cfg.Currency = strings.ToLower(strings.TrimSpace(cfg.Currency))
		if cfg.Currency == "" {
			cfg.Currency = "cny"
		}

		stripe.Key = cfg.SecretKey

		exp := time.Now().Add(2 * time.Hour).Unix()
		params := &stripe.CheckoutSessionParams{
			SuccessURL:        stripe.String(successURL),
			CancelURL:         stripe.String(cancelURL),
			Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
			ClientReferenceID: stripe.String(ref),
			CustomerEmail:     stripe.String(u.Email),
			ExpiresAt:         stripe.Int64(exp),
			LineItems: []*stripe.CheckoutSessionLineItemParams{
				{
					PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
						Currency:   stripe.String(cfg.Currency),
						UnitAmount: stripe.Int64(unitAmount),
						ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
							Name: stripe.String(orderTitle),
						},
					},
					Quantity: stripe.Int64(1),
				},
			},
		}

		sess, err := stripeCheckout.New(params)
		if err != nil || strings.TrimSpace(sess.URL) == "" {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("创建 Stripe 支付失败"), http.StatusFound)
			return
		}
		http.Redirect(w, r, sess.URL, http.StatusFound)
		return
	case "epay":
		cfg := payCfg.EPay
		if !cfg.Enable || strings.TrimSpace(cfg.Gateway) == "" || strings.TrimSpace(cfg.PartnerID) == "" || strings.TrimSpace(cfg.Key) == "" {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("EPay 未配置或未启用"), http.StatusFound)
			return
		}

		epayType := strings.ToLower(strings.TrimSpace(r.FormValue("epay_type")))
		if epayType == "" {
			epayType = "alipay"
		}
		switch epayType {
		case "alipay", "wxpay", "qqpay":
		default:
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("EPay 支付类型不支持"), http.StatusFound)
			return
		}

		client, err := epay.NewClient(&epay.Config{
			PartnerID: cfg.PartnerID,
			Key:       cfg.Key,
		}, cfg.Gateway)
		if err != nil {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("EPay 配置错误"), http.StatusFound)
			return
		}

		notifyURL, err := url.Parse(baseURL + "/api/pay/epay/notify")
		if err != nil {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("回调 URL 配置错误"), http.StatusFound)
			return
		}
		returnURL, err := url.Parse(baseURL + "/pay/" + kind + "/" + strconv.FormatInt(orderID, 10))
		if err != nil {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("回跳 URL 配置错误"), http.StatusFound)
			return
		}

		money := formatCNYFixed(amountCNY)
		purchaseURL, params, err := client.Purchase(&epay.PurchaseArgs{
			Type:           epayType,
			ServiceTradeNo: ref,
			Name:           orderTitle,
			Money:          money,
			Device:         epay.PC,
			NotifyUrl:      notifyURL,
			ReturnUrl:      returnURL,
		})
		if err != nil {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("创建 EPay 支付失败"), http.StatusFound)
			return
		}

		u2, err := url.Parse(purchaseURL)
		if err != nil {
			http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("创建 EPay 支付失败"), http.StatusFound)
			return
		}
		q := u2.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u2.RawQuery = q.Encode()

		http.Redirect(w, r, u2.String(), http.StatusFound)
		return
	default:
		http.Redirect(w, r, "/pay/"+kind+"/"+strconv.FormatInt(orderID, 10)+"?err="+url.QueryEscape("支付方式不支持"), http.StatusFound)
		return
	}
}

func (s *Server) UsagePage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayStr := todayStart.Format("2006-01-02")

	q := r.URL.Query()
	startStr := strings.TrimSpace(q.Get("start"))
	endStr := strings.TrimSpace(q.Get("end"))
	limit := 50
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "limit 不合法", http.StatusBadRequest)
			return
		}
		limit = n
	}
	if limit < 10 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	if startStr == "" {
		startStr = todayStr
	}
	if endStr == "" {
		endStr = startStr
	}
	since, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		http.Error(w, "start 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	endDate, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		http.Error(w, "end 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	if since.After(endDate) {
		http.Error(w, "start 不能晚于 end", http.StatusBadRequest)
		return
	}
	if endDate.After(todayStart) {
		endDate = todayStart
		endStr = todayStr
	}
	until := endDate.Add(24 * time.Hour)
	if endStr == todayStr {
		until = now
	}

	committed, reserved, err := s.store.SumCommittedAndReservedUSDRange(r.Context(), store.UsageSumWithReservedRangeInput{
		UserID: u.ID,
		Since:  since,
		Until:  until,
		Now:    now,
	})
	if err != nil {
		http.Error(w, "用量汇总失败", http.StatusInternalServerError)
		return
	}

	tokenStats, err := s.store.GetUsageTokenStatsByUserRange(r.Context(), u.ID, since, until)
	if err != nil {
		http.Error(w, "Token 统计失败", http.StatusInternalServerError)
		return
	}

	used := committed.Add(reserved)

	// Calculate RPM / TPM (based on time since earliest start or 1 min min)
	dur := until.Sub(since)
	minutes := dur.Minutes()
	if minutes < 1 {
		minutes = 1
	}
	rpm := float64(tokenStats.Requests) / minutes
	tpm := float64(tokenStats.Tokens) / minutes

	view := UsageWindowView{
		Window:       "统计区间（UTC）",
		Since:        since.Format("2006-01-02 15:04"),
		Until:        until.Format("2006-01-02 15:04"),
		UsedUSD:      formatUSD(used),
		CommittedUSD: formatUSD(committed),
		ReservedUSD:  formatUSD(reserved),
		RequestCount: tokenStats.Requests,
		TotalTokens:  tokenStats.Tokens,
		InputTokens:  tokenStats.InputTokens,
		OutputTokens: tokenStats.OutputTokens,
		CachedTokens: tokenStats.CachedInputTokens + tokenStats.CachedOutputTokens,
		CacheHitRate: fmt.Sprintf("%.1f%%", tokenStats.CacheRatio*100),
		RPM:          fmt.Sprintf("%.1f", rpm),
		TPM:          fmt.Sprintf("%.1f", tpm),
	}
	view.LimitUSD = "-"

	var beforeID *int64
	if v := strings.TrimSpace(q.Get("before_id")); v != "" {
		id, err := parseInt64(v)
		if err != nil || id <= 0 {
			http.Error(w, "before_id 不合法", http.StatusBadRequest)
			return
		}
		beforeID = &id
	}
	var afterID *int64
	if v := strings.TrimSpace(q.Get("after_id")); v != "" {
		id, err := parseInt64(v)
		if err != nil || id <= 0 {
			http.Error(w, "after_id 不合法", http.StatusBadRequest)
			return
		}
		afterID = &id
	}
	if beforeID != nil && afterID != nil {
		http.Error(w, "before_id 与 after_id 不能同时使用", http.StatusBadRequest)
		return
	}

	events, err := s.store.ListUsageEventsByUserRange(r.Context(), u.ID, since, until, limit, beforeID, afterID)
	if err != nil {
		http.Error(w, "查询请求明细失败", http.StatusInternalServerError)
		return
	}
	var eventViews []UsageEventView
	for _, e := range events {
		endpoint := "-"
		if e.Endpoint != nil && strings.TrimSpace(*e.Endpoint) != "" {
			endpoint = *e.Endpoint
		}
		method := "-"
		if e.Method != nil && strings.TrimSpace(*e.Method) != "" {
			method = *e.Method
		}
		model := "-"
		if e.Model != nil && strings.TrimSpace(*e.Model) != "" {
			model = *e.Model
		}
		statusCode := "-"
		if e.StatusCode > 0 {
			statusCode = strconv.Itoa(e.StatusCode)
		}
		latencyMS := "-"
		if e.LatencyMS > 0 {
			latencyMS = strconv.Itoa(e.LatencyMS)
		}
		inTok := "-"
		if e.InputTokens != nil {
			inTok = strconv.FormatInt(*e.InputTokens, 10)
		}
		outTok := "-"
		if e.OutputTokens != nil {
			outTok = strconv.FormatInt(*e.OutputTokens, 10)
		}
		var cached int64
		if e.CachedInputTokens != nil {
			cached += *e.CachedInputTokens
		}
		if e.CachedOutputTokens != nil {
			cached += *e.CachedOutputTokens
		}
		cachedTok := "-"
		if cached > 0 {
			cachedTok = strconv.FormatInt(cached, 10)
		}
		reqBytes := strconv.FormatInt(e.RequestBytes, 10)
		respBytes := strconv.FormatInt(e.ResponseBytes, 10)
		costUSD := decimal.Zero
		switch e.State {
		case store.UsageStateCommitted:
			costUSD = e.CommittedUSD
		case store.UsageStateReserved:
			costUSD = e.ReservedUSD
		}
		cost := formatUSD(costUSD)
		if e.State == store.UsageStateReserved {
			cost += " (预留)"
		}
		stateLabel := e.State
		stateBadge := "bg-secondary-subtle text-secondary border border-secondary-subtle"
		switch e.State {
		case store.UsageStateCommitted:
			stateLabel = "已结算"
			stateBadge = "bg-success-subtle text-success border border-success-subtle"
		case store.UsageStateReserved:
			stateLabel = "预留中"
			stateBadge = "bg-warning-subtle text-warning border border-warning-subtle"
		case store.UsageStateVoid:
			stateLabel = "已作废"
			stateBadge = "bg-secondary-subtle text-secondary border border-secondary-subtle"
		case store.UsageStateExpired:
			stateLabel = "已过期"
			stateBadge = "bg-secondary-subtle text-secondary border border-secondary-subtle"
		}
		upstreamChannelID := "-"
		if e.UpstreamChannelID != nil && *e.UpstreamChannelID > 0 {
			upstreamChannelID = strconv.FormatInt(*e.UpstreamChannelID, 10)
		}
		errClass := ""
		if e.ErrorClass != nil && strings.TrimSpace(*e.ErrorClass) != "" {
			errClass = strings.TrimSpace(*e.ErrorClass)
		}
		errMsg := ""
		if e.ErrorMessage != nil && strings.TrimSpace(*e.ErrorMessage) != "" {
			errMsg = strings.TrimSpace(*e.ErrorMessage)
		}
		if errClass == "client_disconnect" {
			errClass = ""
			errMsg = ""
		}
		errText := ""
		if errClass != "" {
			errText = errClass
		}
		if errMsg != "" {
			if errText == "" {
				errText = errMsg
			} else {
				errText = errText + " (" + errMsg + ")"
			}
		}

		eventViews = append(eventViews, UsageEventView{
			ID:                e.ID,
			Time:              e.Time.UTC().Format("2006-01-02 15:04:05"),
			Endpoint:          endpoint,
			Method:            method,
			Model:             model,
			StatusCode:        statusCode,
			LatencyMS:         latencyMS,
			InputTokens:       inTok,
			OutputTokens:      outTok,
			CachedTokens:      cachedTok,
			RequestBytes:      reqBytes,
			ResponseBytes:     respBytes,
			CostUSD:           cost,
			State:             e.State,
			StateLabel:        stateLabel,
			StateBadgeClass:   stateBadge,
			RequestID:         e.RequestID,
			UpstreamChannelID: upstreamChannelID,
			Error:             errText,
			ErrorClass:        errClass,
			ErrorMessage:      errMsg,
			IsStream:          e.IsStream,
		})
	}
	nextBeforeID := ""
	if len(events) == limit {
		next := events[len(events)-1].ID
		nextBeforeID = strconv.FormatInt(next, 10)
	}
	prevAfterID := ""
	if len(events) > 0 {
		canPrev := beforeID != nil || (afterID != nil && len(events) == limit)
		if canPrev {
			prev := events[0].ID
			prevAfterID = strconv.FormatInt(prev, 10)
		}
	}
	cursorActive := beforeID != nil || afterID != nil

	s.Render(w, "page_usage", s.withFeatures(r.Context(), TemplateData{
		Title:             "用量统计 - Realms",
		User:              userViewFromUser(u),
		CSRFToken:         csrfToken(p),
		BaseURL:           s.baseURLFromRequest(r),
		UsageWindows:      []UsageWindowView{view},
		UsageStart:        startStr,
		UsageEnd:          endStr,
		UsageEvents:       eventViews,
		UsageNextBeforeID: nextBeforeID,
		UsagePrevAfterID:  prevAfterID,
		UsageCursorActive: cursorActive,
		UsageLimit:        limit,
	}))
}

func (s *Server) PurchaseSubscription(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	planID, err := parseInt64(strings.TrimSpace(r.FormValue("plan_id")))
	if err != nil || planID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	o, plan, err := s.store.CreateSubscriptionOrderByPlanID(r.Context(), u.ID, planID, time.Now())
	if err != nil {
		s.subscriptionPage(w, r, "下单失败："+err.Error())
		return
	}
	msg := fmt.Sprintf("订单 #%d 已创建（%s - %s），请选择支付方式。", o.ID, plan.Name, formatCNY(plan.PriceCNY))
	http.Redirect(w, r, "/pay/subscription/"+strconv.FormatInt(o.ID, 10)+"?msg="+url.QueryEscape(msg), http.StatusFound)
}

func (s *Server) subscriptionPage(w http.ResponseWriter, r *http.Request, errMsg string) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	subs, err := s.store.ListNonExpiredSubscriptionsWithPlans(r.Context(), u.ID, now)
	if err != nil {
		http.Error(w, "订阅查询失败", http.StatusInternalServerError)
		return
	}

	var activeID int64
	var activeEnd time.Time
	for _, row := range subs {
		if row.Subscription.StartAt.After(now) {
			continue
		}
		if activeID == 0 || row.Subscription.EndAt.Before(activeEnd) {
			activeID = row.Subscription.ID
			activeEnd = row.Subscription.EndAt
		}
	}

	var subViews []SubscriptionView
	activeIndex := -1
	for _, row := range subs {
		isActive := !row.Subscription.StartAt.After(now)
		sv := SubscriptionView{
			Active:    isActive,
			PlanName:  row.Plan.Name,
			PriceCNY:  formatCNY(row.Plan.PriceCNY),
			GroupName: strings.TrimSpace(row.Plan.GroupName),
			StartAt:   row.Subscription.StartAt.Format("2006-01-02 15:04"),
			EndAt:     row.Subscription.EndAt.Format("2006-01-02 15:04"),
		}

		// Calculate 3 windows for this specific subscription's plan
		type winCfg struct {
			name  string
			dur   time.Duration
			limit decimal.Decimal
		}
		wins := []winCfg{
			{name: "5小时", dur: 5 * time.Hour, limit: row.Plan.Limit5HUSD},
			{name: "1天", dur: 24 * time.Hour, limit: row.Plan.Limit1DUSD},
			{name: "7天", dur: 7 * 24 * time.Hour, limit: row.Plan.Limit7DUSD},
			{name: "30天", dur: 30 * 24 * time.Hour, limit: row.Plan.Limit30DUSD},
		}

		if isActive {
			for _, wcfg := range wins {
				if wcfg.limit.LessThanOrEqual(decimal.Zero) {
					continue
				}
				since := now.Add(-wcfg.dur)
				if row.Subscription.StartAt.After(since) {
					since = row.Subscription.StartAt
				}
				committed, reserved, err := s.store.SumCommittedAndReservedUSDBySubscription(r.Context(), store.UsageSumWithReservedBySubscriptionInput{
					UserID:         u.ID,
					SubscriptionID: row.Subscription.ID,
					Since:          since,
					Now:            now,
				})
				if err != nil {
					continue
				}
				used := committed.Add(reserved)
				percent := int(used.Mul(decimal.NewFromInt(100)).Div(wcfg.limit).IntPart())
				if percent > 100 {
					percent = 100
				}
				sv.UsageWindows = append(sv.UsageWindows, UsageWindowView{
					Window:      wcfg.name,
					UsedUSD:     formatUSD(used),
					LimitUSD:    formatUSD(wcfg.limit),
					UsedPercent: percent,
				})
			}
		}
		subViews = append(subViews, sv)
		if row.Subscription.ID == activeID {
			activeIndex = len(subViews) - 1
		}
	}

	plans, err := s.store.ListSubscriptionPlans(r.Context())
	if err != nil {
		http.Error(w, "套餐查询失败", http.StatusInternalServerError)
		return
	}
	allowedGroups := make(map[string]struct{}, len(u.Groups))
	for _, g := range u.Groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		allowedGroups[g] = struct{}{}
	}
	var planViews []PlanView
	for _, p := range plans {
		g := strings.TrimSpace(p.GroupName)
		if g == "" {
			g = store.DefaultGroupName
		}
		if _, ok := allowedGroups[g]; !ok {
			continue
		}
		planViews = append(planViews, PlanView{
			ID:           p.ID,
			Name:         p.Name,
			PriceCNY:     formatCNY(p.PriceCNY),
			GroupName:    g,
			Limit5H:      formatUSDOrUnlimited(p.Limit5HUSD),
			Limit1D:      formatUSDOrUnlimited(p.Limit1DUSD),
			Limit7D:      formatUSDOrUnlimited(p.Limit7DUSD),
			Limit30D:     formatUSDOrUnlimited(p.Limit30DUSD),
			DurationDays: p.DurationDays,
		})
	}

	var subView *SubscriptionView
	if activeIndex >= 0 && activeIndex < len(subViews) {
		subView = &subViews[activeIndex]
	}

	orders, err := s.store.ListSubscriptionOrdersByUser(r.Context(), u.ID, 50)
	if err != nil {
		http.Error(w, "订单查询失败", http.StatusInternalServerError)
		return
	}
	var orderViews []SubscriptionOrderView
	for _, row := range orders {
		status := "未知"
		switch row.Order.Status {
		case store.SubscriptionOrderStatusPending:
			status = "待支付"
		case store.SubscriptionOrderStatusActive:
			status = "已生效"
		case store.SubscriptionOrderStatusCanceled:
			status = "已取消"
		}
		v := SubscriptionOrderView{
			ID:        row.Order.ID,
			PlanName:  row.Plan.Name,
			AmountCNY: formatCNY(row.Order.AmountCNY),
			Status:    status,
			CreatedAt: row.Order.CreatedAt.Format("2006-01-02 15:04"),
		}
		if row.Order.PaidAt != nil {
			v.PaidAt = row.Order.PaidAt.Format("2006-01-02 15:04")
		}
		if row.Order.ApprovedAt != nil {
			v.ApprovedAt = row.Order.ApprovedAt.Format("2006-01-02 15:04")
		}
		orderViews = append(orderViews, v)
	}

	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.Render(w, "page_subscription", s.withFeatures(r.Context(), TemplateData{
		Title:              "订阅管理 - Realms",
		User:               userViewFromUser(u),
		Notice:             notice,
		Error:              errMsg,
		CSRFToken:          csrfToken(p),
		BaseURL:            s.baseURLFromRequest(r),
		Subscription:       subView,
		Subscriptions:      subViews,
		Plans:              planViews,
		SubscriptionOrders: orderViews,
	}))
}

func (s *Server) ModelsPage(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	csrf := csrfToken(p)
	baseURL := s.baseURLFromRequest(r)

	ms, err := s.store.ListEnabledManagedModelsWithBindings(r.Context())
	if err != nil {
		s.Render(w, "page_models", s.withFeatures(r.Context(), TemplateData{
			Title:     "模型列表 - Realms",
			User:      userViewFromUser(u),
			CSRFToken: csrf,
			BaseURL:   baseURL,
			Error:     "查询模型目录失败",
		}))
		return
	}

	var models []ModelView
	for _, m := range ms {
		if strings.TrimSpace(m.PublicID) == "" {
			continue
		}
		ownedBy := "realms"
		if m.OwnedBy != nil && strings.TrimSpace(*m.OwnedBy) != "" {
			ownedBy = *m.OwnedBy
		}
		models = append(models, ModelView{
			ID:                  m.PublicID,
			OwnedBy:             ownedBy,
			InputUSDPer1M:       formatUSDPlain(m.InputUSDPer1M),
			OutputUSDPer1M:      formatUSDPlain(m.OutputUSDPer1M),
			CacheInputUSDPer1M:  formatUSDPlain(m.CacheInputUSDPer1M),
			CacheOutputUSDPer1M: formatUSDPlain(m.CacheOutputUSDPer1M),
		})
	}

	s.Render(w, "page_models", s.withFeatures(r.Context(), TemplateData{
		Title:     "模型列表 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrf,
		BaseURL:   baseURL,
		Models:    models,
	}))
}

func (s *Server) CreateToken(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	var namePtr *string
	if name != "" {
		namePtr = &name
	}

	raw, err := auth.NewRandomToken("rlm_", 32)
	if err != nil {
		http.Error(w, "生成令牌失败", http.StatusInternalServerError)
		return
	}
	if _, _, err := s.store.CreateUserToken(r.Context(), u.ID, namePtr, raw); err != nil {
		http.Error(w, "创建令牌失败", http.StatusInternalServerError)
		return
	}
	s.Render(w, "page_token_created", s.withFeatures(r.Context(), TemplateData{
		Title:     "令牌已创建 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrfToken(p),
		Token:     raw,
	}))
}

func (s *Server) RotateToken(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	u, err := s.store.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		http.Error(w, "用户查询失败", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	tokenID, err := parseInt64(r.FormValue("token_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	raw, err := auth.NewRandomToken("rlm_", 32)
	if err != nil {
		http.Error(w, "生成令牌失败", http.StatusInternalServerError)
		return
	}
	if err := s.store.RotateUserToken(r.Context(), u.ID, tokenID, raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "令牌不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "重新生成失败", http.StatusInternalServerError)
		return
	}

	s.Render(w, "page_token_created", s.withFeatures(r.Context(), TemplateData{
		Title:     "令牌已重新生成 - Realms",
		User:      userViewFromUser(u),
		CSRFToken: csrfToken(p),
		Token:     raw,
	}))
}

func (s *Server) RevokeToken(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	tokenID, err := parseInt64(r.FormValue("token_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := s.store.RevokeUserToken(r.Context(), p.UserID, tokenID); err != nil {
		http.Error(w, "撤销失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tokens", http.StatusFound)
}

func (s *Server) DeleteToken(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	tokenID, err := parseInt64(r.FormValue("token_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteUserToken(r.Context(), p.UserID, tokenID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tokens", http.StatusFound)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(SessionCookieName)
	if err == nil && c.Value != "" {
		_ = s.store.DeleteSessionByRaw(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) forceLogoutUser(w http.ResponseWriter, r *http.Request, userID int64, msg string) {
	_ = s.store.DeleteSessionsByUserID(r.Context(), userID)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	target := "/login"
	if strings.TrimSpace(msg) != "" {
		target = "/login?msg=" + url.QueryEscape(msg)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) renderAccountPage(w http.ResponseWriter, r *http.Request, u store.User, errMsg string) {
	p, _ := auth.PrincipalFromContext(r.Context())
	s.Render(w, "page_account", s.withFeatures(r.Context(), TemplateData{
		Title:                    "账号设置 - Realms",
		User:                     userViewFromUser(u),
		CSRFToken:                csrfToken(p),
		EmailVerificationEnabled: s.emailVerificationEnabled(r.Context()),
		Error:                    errMsg,
	}))
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	rawSession, err := auth.NewRandomToken("s_", 32)
	if err != nil {
		return err
	}
	csrfToken, err := auth.NewRandomToken("csrf_", 32)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	if _, err := s.store.CreateSession(r.Context(), userID, rawSession, csrfToken, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawSession,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   !s.disableSecureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) baseURLFromRequest(r *http.Request) string {
	if r != nil {
		if v, ok, err := s.store.GetStringAppSetting(r.Context(), store.SettingSiteBaseURL); err == nil && ok {
			if normalized, err := config.NormalizeHTTPBaseURL(v, "site_base_url"); err == nil && normalized != "" {
				return normalized
			}
		}
	}
	if strings.TrimSpace(s.publicBaseURL) != "" {
		return s.publicBaseURL
	}
	return security.DeriveBaseURLFromRequest(r, s.trustProxyHeaders, s.trustedProxies)
}

func csrfToken(p auth.Principal) string {
	if p.CSRFToken == nil {
		return ""
	}
	return *p.CSRFToken
}

func sanitizeNextPath(raw string) string {
	next := strings.TrimSpace(raw)
	if next == "" {
		return ""
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, "\\") {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return ""
	}
	if u.IsAbs() || u.Host != "" || u.Scheme != "" {
		return ""
	}
	return u.String()
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("非数字")
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	lr := io.LimitReader(r, limit+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("响应过大")
	}
	return b, nil
}

func (s *Server) selectOpenAISelection(ctx context.Context, userID int64) (scheduler.Selection, error) {
	sel, err := s.sched.Select(ctx, userID, "")
	if err == nil && sel.CredentialType == scheduler.CredentialTypeOpenAI {
		return sel, nil
	}

	channels, err2 := s.store.ListUpstreamChannels(ctx)
	if err2 != nil {
		return scheduler.Selection{}, err2
	}
	for _, ch := range channels {
		if ch.Status != 1 || ch.Type != store.UpstreamTypeOpenAICompatible {
			continue
		}
		eps, err := s.store.ListUpstreamEndpointsByChannel(ctx, ch.ID)
		if err != nil {
			return scheduler.Selection{}, err
		}
		for _, ep := range eps {
			if ep.Status != 1 {
				continue
			}
			creds, err := s.store.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
			if err != nil {
				return scheduler.Selection{}, err
			}
			for _, c := range creds {
				if c.Status != 1 {
					continue
				}
				return scheduler.Selection{
					ChannelID:      ch.ID,
					ChannelType:    ch.Type,
					EndpointID:     ep.ID,
					BaseURL:        ep.BaseURL,
					CredentialType: scheduler.CredentialTypeOpenAI,
					CredentialID:   c.ID,
				}, nil
			}
		}
	}
	if err != nil {
		return scheduler.Selection{}, err
	}
	return scheduler.Selection{}, fmt.Errorf("当前未配置可用 openai_compatible 上游，无法展示模型列表")
}
