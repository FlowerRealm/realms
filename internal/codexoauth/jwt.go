package codexoauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

func ParseJWTClaims(raw string) (map[string]any, error) {
	payload, err := jwtPayload(raw)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, err
	}
	return m, nil
}

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

func (c *IDTokenClaims) HasActivePlusSubscription(now time.Time) bool {
	if c == nil {
		return false
	}
	if until, ok := parseEpochOrTime(c.SubscriptionActiveUntil); ok && !until.After(now) {
		return false
	}
	if start, ok := parseEpochOrTime(c.SubscriptionActiveStart); ok && start.After(now) {
		return false
	}
	return true
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

func parseEpochOrTime(v any) (time.Time, bool) {
	switch value := v.(type) {
	case float64:
		return unixTimeFromInt64(int64(value))
	case int64:
		return unixTimeFromInt64(value)
	case int:
		return unixTimeFromInt64(int64(value))
	case json.Number:
		n, err := value.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return unixTimeFromInt64(n)
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return time.Time{}, false
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return unixTimeFromInt64(n)
		}
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	default:
		return time.Time{}, false
	}
}

func unixTimeFromInt64(v int64) (time.Time, bool) {
	if v <= 0 {
		return time.Time{}, false
	}
	// 兼容毫秒级时间戳。
	if v > 1_000_000_000_000 {
		return time.UnixMilli(v), true
	}
	return time.Unix(v, 0), true
}

func jwtPayload(raw string) ([]byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid jwt")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}
