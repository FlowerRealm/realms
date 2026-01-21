package codexoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Quota struct {
	PlanType string

	CreditsHasCredits *bool
	CreditsUnlimited  *bool
	CreditsBalance    *string

	PrimaryUsedPercent   *int
	PrimaryResetAt       *time.Time
	SecondaryUsedPercent *int
	SecondaryResetAt     *time.Time
}

type HTTPStatusError struct {
	StatusCode  int
	BodySnippet string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "<nil>"
	}
	msg := strings.TrimSpace(e.BodySnippet)
	if msg == "" {
		msg = "(empty)"
	}
	return fmt.Sprintf("上游状态码 %d: %s", e.StatusCode, msg)
}

func CodexUsageURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("解析 codex base_url 失败: %w", err)
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/v1") {
		u.Path = strings.TrimSuffix(u.Path, "/v1")
	}

	switch {
	case strings.Contains(u.Path, "/backend-api"):
		// 对齐 Codex CLI：ChatGPT 后端的 WHAM 路径通常挂在 /backend-api 下（而不是 /backend-api/codex 下）。
		// 因此这里把 base_url 的 path 归一化到 /backend-api，然后拼接 /wham/usage。
		if idx := strings.Index(u.Path, "/backend-api"); idx >= 0 {
			u.Path = u.Path[:idx+len("/backend-api")]
		}
		return url.JoinPath(u.String(), "wham", "usage")
	case strings.HasSuffix(u.Path, "/api/codex"):
		return url.JoinPath(u.String(), "usage")
	default:
		return url.JoinPath(u.String(), "api", "codex", "usage")
	}
}

func (c *Client) FetchQuota(ctx context.Context, baseURL string, accessToken string, accountID string) (Quota, error) {
	if strings.TrimSpace(accessToken) == "" {
		return Quota{}, errors.New("access_token 为空")
	}
	usageURL, err := CodexUsageURL(baseURL)
	if err != nil {
		return Quota{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return Quota{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("User-Agent", "codex_cli_rs/0.50.0 (Mac OS 26.0.1; arm64) Apple_Terminal/464")
	req.Header.Set("Originator", "codex_cli_rs")
	if strings.TrimSpace(accountID) != "" {
		req.Header.Set("Chatgpt-Account-Id", strings.TrimSpace(accountID))
	}

	hc := *c.http
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		return Quota{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		return Quota{}, &HTTPStatusError{StatusCode: resp.StatusCode, BodySnippet: msg}
	}

	var parsed struct {
		PlanType  string `json:"plan_type"`
		RateLimit *struct {
			PrimaryWindow *struct {
				UsedPercent int   `json:"used_percent"`
				ResetAt     int64 `json:"reset_at"`
			} `json:"primary_window"`
			SecondaryWindow *struct {
				UsedPercent int   `json:"used_percent"`
				ResetAt     int64 `json:"reset_at"`
			} `json:"secondary_window"`
		} `json:"rate_limit"`
		Credits *struct {
			HasCredits bool    `json:"has_credits"`
			Unlimited  bool    `json:"unlimited"`
			Balance    *string `json:"balance"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		return Quota{}, fmt.Errorf("解析 codex usage 响应失败: %w (%s)", err, msg)
	}

	out := Quota{PlanType: strings.TrimSpace(parsed.PlanType)}

	if parsed.Credits != nil {
		has := parsed.Credits.HasCredits
		unlimited := parsed.Credits.Unlimited
		out.CreditsHasCredits = &has
		out.CreditsUnlimited = &unlimited
		if parsed.Credits.Balance != nil && strings.TrimSpace(*parsed.Credits.Balance) != "" {
			b := strings.TrimSpace(*parsed.Credits.Balance)
			out.CreditsBalance = &b
		}
	}

	if parsed.RateLimit != nil {
		if parsed.RateLimit.PrimaryWindow != nil {
			used := parsed.RateLimit.PrimaryWindow.UsedPercent
			out.PrimaryUsedPercent = &used
			if parsed.RateLimit.PrimaryWindow.ResetAt > 0 {
				t := time.Unix(parsed.RateLimit.PrimaryWindow.ResetAt, 0).UTC()
				out.PrimaryResetAt = &t
			}
		}
		if parsed.RateLimit.SecondaryWindow != nil {
			used := parsed.RateLimit.SecondaryWindow.UsedPercent
			out.SecondaryUsedPercent = &used
			if parsed.RateLimit.SecondaryWindow.ResetAt > 0 {
				t := time.Unix(parsed.RateLimit.SecondaryWindow.ResetAt, 0).UTC()
				out.SecondaryResetAt = &t
			}
		}
	}

	return out, nil
}
