package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CRSClient struct {
	apiBase    string
	adminToken string
	httpClient *http.Client
}

type CodexTokens struct {
	IDToken      string `json:"idToken"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expires_in"`
}

type CodexAccountInfo struct {
	Email string `json:"email,omitempty"`
}

func NewCRSClient(apiBase, adminToken string) *CRSClient {
	return &CRSClient{
		apiBase:    apiBase,
		adminToken: adminToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CRSClient) GenerateAuthURL(ctx context.Context) (authURL, sessionID string, err error) {
	reqBody := map[string]interface{}{}
	jsonData, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/admin/openai-accounts/generate-auth-url", c.apiBase),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.adminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", c.apiBase)
	req.Header.Set("Referer", fmt.Sprintf("%s/admin-next/accounts", c.apiBase))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to generate auth URL: status %d", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			AuthURL   string `json:"authUrl"`
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	if !result.Success {
		return "", "", fmt.Errorf("CRS API returned success=false")
	}

	return result.Data.AuthURL, result.Data.SessionID, nil
}

func (c *CRSClient) ExchangeCode(ctx context.Context, code, sessionID string) (*CodexTokens, error) {
	reqBody := map[string]interface{}{
		"code":      code,
		"sessionId": sessionID,
	}
	jsonData, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/admin/openai-accounts/exchange-code", c.apiBase),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.adminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", c.apiBase)
	req.Header.Set("Referer", fmt.Sprintf("%s/admin-next/accounts", c.apiBase))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to exchange code: status %d", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Tokens       CodexTokens      `json:"tokens"`
			AccountInfo  CodexAccountInfo `json:"accountInfo"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("CRS API returned success=false")
	}

	return &result.Data.Tokens, nil
}

func (c *CRSClient) AddAccount(ctx context.Context, email string, tokens *CodexTokens, accountInfo *CodexAccountInfo) error {
	reqBody := map[string]interface{}{
		"name":        email,
		"description": "",
		"accountType": "shared",
		"proxy":       nil,
		"openaiOauth": map[string]interface{}{
			"idToken":      tokens.IDToken,
			"accessToken":  tokens.AccessToken,
			"refreshToken": tokens.RefreshToken,
			"expires_in":   tokens.ExpiresIn,
		},
		"accountInfo": accountInfo,
		"priority":    50,
	}
	jsonData, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/admin/openai-accounts", c.apiBase),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.adminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", c.apiBase)
	req.Header.Set("Referer", fmt.Sprintf("%s/admin-next/accounts", c.apiBase))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to add account to CRS: status %d", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("CRS API returned success=false")
	}

	return nil
}
