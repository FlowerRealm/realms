// Package store 定义数据库层的核心数据结构，避免在 handler/scheduler 中散落 SQL 字段细节。
package store

import (
	"time"

	"github.com/shopspring/decimal"
)

type User struct {
	ID           int64
	Email        string
	Username     string
	PasswordHash []byte
	Role         string
	Groups       []string
	Status       int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ChannelGroup struct {
	ID              int64
	Name            string
	Description     *string
	PriceMultiplier decimal.Decimal
	MaxAttempts     int
	Status          int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ChannelGroupMember struct {
	ID              int64
	ParentGroupID   int64
	MemberGroupID   *int64
	MemberChannelID *int64
	Priority        int
	Promotion       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UserToken struct {
	ID         int64
	UserID     int64
	Name       *string
	TokenHash  []byte
	TokenHint  *string
	Status     int
	CreatedAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

type UserSession struct {
	ID          int64
	UserID      int64
	SessionHash []byte
	CSRFToken   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	LastSeenAt  time.Time
}

type EmailVerification struct {
	ID         int64
	UserID     *int64
	Email      string
	CodeHash   []byte
	ExpiresAt  time.Time
	VerifiedAt *time.Time
	CreatedAt  time.Time
}

type UpstreamChannel struct {
	ID        int64
	Type      string
	Name      string
	Groups    string
	Status    int
	Priority  int
	Promotion bool

	LimitSessions *int
	LimitRPM      *int
	LimitTPM      *int

	LastTestAt        *time.Time
	LastTestLatencyMS int
	LastTestOK        bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UpstreamEndpoint struct {
	ID        int64
	ChannelID int64
	BaseURL   string
	Status    int
	Priority  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OpenAICompatibleCredential struct {
	ID            int64
	EndpointID    int64
	Name          *string
	APIKeyEnc     []byte
	APIKeyHint    *string
	Status        int
	LimitSessions *int
	LimitRPM      *int
	LimitTPM      *int
	LastUsedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CodexOAuthPending struct {
	State        string
	EndpointID   int64
	ActorUserID  int64
	CodeVerifier string
	CreatedAt    time.Time
}

type CodexOAuthAccount struct {
	ID              int64
	EndpointID      int64
	AccountID       string
	Email           *string
	AccessTokenEnc  []byte
	RefreshTokenEnc []byte
	IDTokenEnc      []byte
	ExpiresAt       *time.Time
	LastRefreshAt   *time.Time
	Status          int
	LimitSessions   *int
	LimitRPM        *int
	LimitTPM        *int
	CooldownUntil   *time.Time
	LastUsedAt      *time.Time

	BalanceTotalGrantedUSD   *decimal.Decimal
	BalanceTotalUsedUSD      *decimal.Decimal
	BalanceTotalAvailableUSD *decimal.Decimal
	BalanceUpdatedAt         *time.Time
	BalanceError             *string

	QuotaCreditsHasCredits    *bool
	QuotaCreditsUnlimited     *bool
	QuotaCreditsBalance       *string
	QuotaPrimaryUsedPercent   *int
	QuotaPrimaryResetAt       *time.Time
	QuotaSecondaryUsedPercent *int
	QuotaSecondaryResetAt     *time.Time
	QuotaUpdatedAt            *time.Time
	QuotaError                *string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type OAuthApp struct {
	ID               int64
	ClientID         string
	Name             string
	ClientSecretHash []byte
	Status           int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type OAuthAppRedirectURI struct {
	ID          int64
	AppID       int64
	RedirectURI string
	CreatedAt   time.Time
}

type OAuthUserGrant struct {
	ID        int64
	UserID    int64
	AppID     int64
	Scope     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OAuthAuthCode struct {
	ID                  int64
	CodeHash            []byte
	AppID               int64
	UserID              int64
	RedirectURI         string
	Scope               string
	CodeChallenge       *string
	CodeChallengeMethod *string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	CreatedAt           time.Time
}

type OAuthAppToken struct {
	ID        int64
	AppID     int64
	UserID    int64
	TokenID   int64
	Scope     string
	CreatedAt time.Time
}

type ManagedModel struct {
	ID             int64
	PublicID       string
	UpstreamModel  *string
	OwnedBy        *string
	InputUSDPer1M  decimal.Decimal
	OutputUSDPer1M decimal.Decimal
	CacheUSDPer1M  decimal.Decimal
	Status         int
	CreatedAt      time.Time
}

type ChannelModel struct {
	ID            int64
	ChannelID     int64
	PublicID      string
	UpstreamModel string
	Status        int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ChannelModelBinding struct {
	ID            int64
	ChannelID     int64
	ChannelType   string
	ChannelGroups string
	PublicID      string
	UpstreamModel string
	Status        int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UsageEvent struct {
	ID                 int64
	Time               time.Time
	RequestID          string
	Endpoint           *string
	Method             *string
	UserID             int64
	SubscriptionID     *int64
	TokenID            int64
	UpstreamChannelID  *int64
	UpstreamEndpointID *int64
	UpstreamCredID     *int64
	State              string
	Model              *string
	InputTokens        *int64
	CachedInputTokens  *int64
	OutputTokens       *int64
	CachedOutputTokens *int64
	ReservedUSD        decimal.Decimal
	CommittedUSD       decimal.Decimal
	ReserveExpiresAt   time.Time
	StatusCode         int
	LatencyMS          int
	ErrorClass         *string
	ErrorMessage       *string
	IsStream           bool
	RequestBytes       int64
	ResponseBytes      int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type SubscriptionPlan struct {
	ID           int64
	Code         string
	Name         string
	GroupName    string
	PriceCNY     decimal.Decimal
	Limit5HUSD   decimal.Decimal
	Limit1DUSD   decimal.Decimal
	Limit7DUSD   decimal.Decimal
	Limit30DUSD  decimal.Decimal
	DurationDays int
	Status       int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserSubscription struct {
	ID        int64
	UserID    int64
	PlanID    int64
	StartAt   time.Time
	EndAt     time.Time
	Status    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UserBalance struct {
	UserID    int64
	USD       decimal.Decimal
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PaymentChannel struct {
	ID     int64
	Type   string
	Name   string
	Status int

	StripeCurrency      *string
	StripeSecretKey     *string
	StripeWebhookSecret *string

	EPayGateway   *string
	EPayPartnerID *string
	EPayKey       *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionOrder struct {
	ID             int64
	UserID         int64
	PlanID         int64
	AmountCNY      decimal.Decimal
	Status         int
	PaidAt         *time.Time
	PaidMethod     *string
	PaidRef        *string
	PaidChannelID  *int64
	ApprovedAt     *time.Time
	ApprovedBy     *int64
	SubscriptionID *int64
	Note           *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type TopupOrder struct {
	ID            int64
	UserID        int64
	AmountCNY     decimal.Decimal
	CreditUSD     decimal.Decimal
	Status        int
	PaidAt        *time.Time
	PaidMethod    *string
	PaidRef       *string
	PaidChannelID *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Ticket struct {
	ID            int64
	UserID        int64
	Subject       string
	Status        int
	LastMessageAt time.Time
	ClosedAt      *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type TicketMessage struct {
	ID          int64
	TicketID    int64
	ActorType   string
	ActorUserID *int64
	Body        string
	CreatedAt   time.Time
}

type TicketAttachment struct {
	ID             int64
	TicketID       int64
	MessageID      int64
	UploaderUserID *int64
	OriginalName   string
	ContentType    *string
	SizeBytes      int64
	SHA256         []byte
	StorageRelPath string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}
