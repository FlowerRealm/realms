package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type TeamInviteClient struct {
	httpClient *http.Client
}

type TeamConfig struct {
	Name       string `json:"name"`
	AccountID  string `json:"account_id"`
	AuthToken  string `json:"auth_token"`
	MaxInvites int    `json:"max_invites"`
}

type InviteTracker struct {
	Teams map[string][]string `json:"teams"`
	mu    sync.RWMutex
}

var globalTracker = &InviteTracker{
	Teams: make(map[string][]string),
}

func NewTeamInviteClient() *TeamInviteClient {
	return &TeamInviteClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *TeamInviteClient) InviteToTeam(ctx context.Context, email string, team TeamConfig) error {
	payload := map[string]interface{}{
		"email_addresses": []string{email},
		"role":            "standard-user",
		"resend_emails":   true,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://chatgpt.com/backend-api/accounts/%s/invites", team.AccountID),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", team.AuthToken)
	req.Header.Set("chatgpt-account-id", team.AccountID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to invite to team: status %d", resp.StatusCode)
	}

	var result struct {
		AccountInvites []interface{} `json:"account_invites"`
		ErroredEmails  []interface{} `json:"errored_emails"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if len(result.ErroredEmails) > 0 {
		return fmt.Errorf("invite error: %v", result.ErroredEmails)
	}

	recordTeamInvite(team.AccountID, email)
	return nil
}

func getAvailableTeam(teams []TeamConfig) *TeamConfig {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()

	for i := range teams {
		invited := globalTracker.Teams[teams[i].AccountID]
		if len(invited) < teams[i].MaxInvites {
			return &teams[i]
		}
	}
	return nil
}

func recordTeamInvite(teamAccountID, email string) {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()

	globalTracker.Teams[teamAccountID] = append(
		globalTracker.Teams[teamAccountID],
		email,
	)
}
