package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

type TempEmailClient struct {
	workerDomain string
	adminToken   string
	httpClient   *http.Client
}

func NewTempEmailClient(workerDomain, adminToken string) *TempEmailClient {
	return &TempEmailClient{
		workerDomain: workerDomain,
		adminToken:   adminToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *TempEmailClient) CreateEmail(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/generate", c.workerDomain), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.adminToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create email: status %d", resp.StatusCode)
	}

	var result struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Email == "" {
		return "", errors.New("empty email returned")
	}

	return result.Email, nil
}

func (c *TempEmailClient) FetchVerificationCode(ctx context.Context, email string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			code, err := c.checkEmailsOnce(ctx, email)
			if err == nil && code != "" {
				return code, nil
			}
		}
	}
	return "", errors.New("timeout waiting for verification email")
}

func (c *TempEmailClient) checkEmailsOnce(ctx context.Context, email string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/emails?mailbox=%s", c.workerDomain, email), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.adminToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch emails: status %d", resp.StatusCode)
	}

	var emails []struct {
		ID      string `json:"id"`
		From    string `json:"from"`
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, email := range emails {
		if c.isOpenAIEmail(email.From, email.Subject) {
			code, err := c.fetchEmailDetail(ctx, email.ID)
			if err == nil && code != "" {
				return code, nil
			}
		}
	}

	return "", nil
}

func (c *TempEmailClient) isOpenAIEmail(from, subject string) bool {
	return regexp.MustCompile(`(?i)openai`).MatchString(from) ||
		regexp.MustCompile(`(?i)chatgpt`).MatchString(subject)
}

func (c *TempEmailClient) fetchEmailDetail(ctx context.Context, emailID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/email/%s", c.workerDomain, emailID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.adminToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch email detail: status %d", resp.StatusCode)
	}

	var detail struct {
		HTMLContent string `json:"html_content"`
		Content     string `json:"content"`
		Text        string `json:"text"`
		Subject     string `json:"subject"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return "", err
	}

	content := detail.HTMLContent
	if content == "" {
		content = detail.Content
	}
	if content == "" {
		content = detail.Text
	}
	if content == "" {
		content = detail.Subject
	}

	return c.extractVerificationCode(content), nil
}

func (c *TempEmailClient) extractVerificationCode(content string) string {
	patterns := []string{
		`代码为\s*(\d{6})`,
		`code is\s*(\d{6})`,
		`(\d{6})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}
