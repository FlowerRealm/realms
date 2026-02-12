package codexoauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

type OpenAIAuthClaims struct {
	ChatgptAccountID               string              `json:"chatgpt_account_id"`
	ChatgptUserID                  string              `json:"chatgpt_user_id"`
	UserID                         string              `json:"user_id"`
	ChatgptPlanType                string              `json:"chatgpt_plan_type"`
	ChatgptSubscriptionActiveStart any                 `json:"chatgpt_subscription_active_start"`
	ChatgptSubscriptionActiveUntil any                 `json:"chatgpt_subscription_active_until"`
	Organizations                  []OrganizationClaim `json:"organizations"`
}

type OrganizationClaim struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Title     string `json:"title"`
	IsDefault bool   `json:"is_default"`
}

type IDTokenClaims struct {
	AccountID               string
	ChatgptUserID           string
	UserID                  string
	OrganizationID          string
	Organizations           []OrganizationClaim
	Email                   string
	PlanType                string
	SubscriptionActiveStart any
	SubscriptionActiveUntil any
}

func ParseIDTokenClaims(raw string) (*IDTokenClaims, error) {
	payload, err := jwtPayload(raw)
	if err != nil {
		return nil, err
	}
	parsed := struct {
		Email                          string              `json:"email"`
		ChatgptAccountID               string              `json:"chatgpt_account_id"`
		AccountID                      string              `json:"account_id"`
		ChatgptUserID                  string              `json:"chatgpt_user_id"`
		UserID                         string              `json:"user_id"`
		PlanType                       string              `json:"plan_type"`
		OrganizationID                 string              `json:"organization_id"`
		Organizations                  []OrganizationClaim `json:"organizations"`
		ChatgptSubscriptionActiveStart any                 `json:"chatgpt_subscription_active_start"`
		ChatgptSubscriptionActiveUntil any                 `json:"chatgpt_subscription_active_until"`
		OpenAIAuth                     OpenAIAuthClaims    `json:"https://api.openai.com/auth"`
	}{
		OpenAIAuth: OpenAIAuthClaims{},
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}

	accountID := strings.TrimSpace(parsed.OpenAIAuth.ChatgptAccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(parsed.ChatgptAccountID)
	}
	if accountID == "" {
		accountID = strings.TrimSpace(parsed.AccountID)
	}

	planType := strings.TrimSpace(parsed.OpenAIAuth.ChatgptPlanType)
	if planType == "" {
		planType = strings.TrimSpace(parsed.PlanType)
	}

	chatgptUserID := strings.TrimSpace(parsed.OpenAIAuth.ChatgptUserID)
	if chatgptUserID == "" {
		chatgptUserID = strings.TrimSpace(parsed.ChatgptUserID)
	}

	userID := strings.TrimSpace(parsed.OpenAIAuth.UserID)
	if userID == "" {
		userID = strings.TrimSpace(parsed.UserID)
	}

	organizations := parsed.OpenAIAuth.Organizations
	if len(organizations) == 0 {
		organizations = parsed.Organizations
	}

	organizationID := strings.TrimSpace(parsed.OrganizationID)
	if organizationID == "" {
		organizationID = defaultOrganizationID(organizations)
	}

	subscriptionStart := parsed.OpenAIAuth.ChatgptSubscriptionActiveStart
	if subscriptionStart == nil {
		subscriptionStart = parsed.ChatgptSubscriptionActiveStart
	}
	subscriptionUntil := parsed.OpenAIAuth.ChatgptSubscriptionActiveUntil
	if subscriptionUntil == nil {
		subscriptionUntil = parsed.ChatgptSubscriptionActiveUntil
	}

	return &IDTokenClaims{
		AccountID:               accountID,
		ChatgptUserID:           chatgptUserID,
		UserID:                  userID,
		OrganizationID:          organizationID,
		Organizations:           organizations,
		Email:                   strings.TrimSpace(parsed.Email),
		PlanType:                planType,
		SubscriptionActiveStart: subscriptionStart,
		SubscriptionActiveUntil: subscriptionUntil,
	}, nil
}

func defaultOrganizationID(organizations []OrganizationClaim) string {
	for _, org := range organizations {
		if org.IsDefault && strings.TrimSpace(org.ID) != "" {
			return strings.TrimSpace(org.ID)
		}
	}
	if len(organizations) == 0 {
		return ""
	}
	return strings.TrimSpace(organizations[0].ID)
}

func jwtPayload(raw string) ([]byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid jwt")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}
