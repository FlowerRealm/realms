package codexoauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
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
	ChatgptAccountID               string `json:"chatgpt_account_id"`
	ChatgptPlanType                string `json:"chatgpt_plan_type"`
	ChatgptSubscriptionActiveStart any    `json:"chatgpt_subscription_active_start"`
	ChatgptSubscriptionActiveUntil any    `json:"chatgpt_subscription_active_until"`
}

type IDTokenClaims struct {
	AccountID               string
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
		Email            string           `json:"email"`
		ChatgptAccountID string           `json:"chatgpt_account_id"`
		AccountID        string           `json:"account_id"`
		PlanType         string           `json:"plan_type"`
		OpenAIAuth       OpenAIAuthClaims `json:"https://api.openai.com/auth"`
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

	return &IDTokenClaims{
		AccountID:               accountID,
		Email:                   strings.TrimSpace(parsed.Email),
		PlanType:                planType,
		SubscriptionActiveStart: parsed.OpenAIAuth.ChatgptSubscriptionActiveStart,
		SubscriptionActiveUntil: parsed.OpenAIAuth.ChatgptSubscriptionActiveUntil,
	}, nil
}

func jwtPayload(raw string) ([]byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid jwt")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}
