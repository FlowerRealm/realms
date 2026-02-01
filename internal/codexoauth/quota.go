package codexoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	u.Path = strings.TrimSuffix(u.Path, "/v1")

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
	usageURLs, err := codexUsageURLCandidates(baseURL)
	if err != nil {
		return Quota{}, err
	}

	hc := *c.http
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

	var lastErr error
	var resp *http.Response
	var body []byte
	for _, usageURL := range usageURLs {
		resp = nil
		body = nil
		lastErr = nil

		for attempt := 0; attempt < 2; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
			if err != nil {
				return Quota{}, err
			}
			req.Close = true
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
			if strings.TrimSpace(accountID) != "" {
				req.Header.Set("Chatgpt-Account-Id", strings.TrimSpace(accountID))
			}

			resp, err = hc.Do(req)
			if err != nil {
				lastErr = err
				if attempt == 0 && ctx.Err() == nil && isRetryableQuotaFetchError(err) {
					continue
				}
				break
			}

			body, err = io.ReadAll(io.LimitReader(resp.Body, 256<<10))
			_ = resp.Body.Close()
			if err != nil {
				lastErr = fmt.Errorf("读取 codex usage 响应失败: %w", err)
				if attempt == 0 && ctx.Err() == nil && isRetryableQuotaFetchError(err) {
					continue
				}
				break
			}
			break
		}

		if resp == nil {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := strings.TrimSpace(string(body))
			if len(msg) > 200 {
				msg = msg[:200] + "..."
			}
			lastErr = &HTTPStatusError{StatusCode: resp.StatusCode, BodySnippet: msg}
			continue
		}
		lastErr = nil
		break
	}

	if lastErr != nil {
		return Quota{}, lastErr
	}
	if resp == nil {
		return Quota{}, errors.New("codex usage 请求失败")
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

func codexUsageURLCandidates(baseURL string) ([]string, error) {
	primary, err := CodexUsageURL(baseURL)
	if err != nil {
		return nil, err
	}
	out := []string{primary}

	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return out, nil
	}
	u.RawQuery = ""
	u.Fragment = ""
	hasBackendAPI := strings.Contains(u.Path, "/backend-api")
	if !hasBackendAPI {
		return out, nil
	}

	origin := *u
	origin.Path = ""
	origin.RawQuery = ""
	origin.Fragment = ""
	backend := *u
	backend.Path = strings.TrimRight(backend.Path, "/")
	if idx := strings.Index(backend.Path, "/backend-api"); idx >= 0 {
		backend.Path = backend.Path[:idx+len("/backend-api")]
	}
	if alt, err := url.JoinPath(backend.String(), "codex", "usage"); err == nil && alt != "" && alt != primary {
		out = append(out, alt)
	}
	if alt, err := url.JoinPath(origin.String(), "api", "codex", "usage"); err == nil && alt != "" && alt != primary {
		out = append(out, alt)
	}

	seen := make(map[string]struct{}, len(out))
	deduped := make([]string, 0, len(out))
	for _, u := range out {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		deduped = append(deduped, u)
	}
	return deduped, nil
}

func isRetryableQuotaFetchError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	// 兼容极少数场景：HTTP/2/代理在握手阶段提前断开，net/http 可能包成字符串错误。
	if strings.Contains(err.Error(), "TLS handshake timeout") {
		return true
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe != nil && (oe.Op == "dial" || oe.Op == "read") {
		return true
	}
	return false
}
