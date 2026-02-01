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

	"realms/internal/config"
)

type Client struct {
	cfg  config.CodexOAuthConfig
	http *http.Client

	refreshBackoffs []time.Duration
}

type TokenResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    *time.Time
}

type TokenEndpointError struct {
	StatusCode       int
	ErrorCode        string
	ErrorDescription string
	BodySnippet      string
}

func (e *TokenEndpointError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.ErrorCode != "" || e.ErrorDescription != "" {
		return fmt.Sprintf("token exchange failed: %d %s %s", e.StatusCode, e.ErrorCode, e.ErrorDescription)
	}
	if e.BodySnippet != "" {
		return fmt.Sprintf("token exchange failed: %d %s", e.StatusCode, e.BodySnippet)
	}
	return fmt.Sprintf("token exchange failed: %d", e.StatusCode)
}

func (e *TokenEndpointError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

func cloneDefaultTransport() *http.Transport {
	if t, ok := http.DefaultTransport.(*http.Transport); ok && t != nil {
		return t.Clone()
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   0,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   0,
		ExpectContinueTimeout: 0,
	}
}

func NewClient(cfg config.CodexOAuthConfig) *Client {
	t := cloneDefaultTransport()
	// 移除超时限制：Dial/TLS/HTTP 均允许无限等待。
	t.DialContext = (&net.Dialer{Timeout: 0, KeepAlive: 30 * time.Second}).DialContext

	c := &Client{
		cfg: cfg,
		http: &http.Client{
			Transport: t,
			Timeout:   0,
		},
		refreshBackoffs: []time.Duration{2 * time.Second, 5 * time.Second},
	}
	return c
}

func (c *Client) BuildAuthorizeURL(state string, codeChallenge string) (string, error) {
	u, err := url.Parse(c.cfg.AuthorizeURL)
	if err != nil {
		return "", fmt.Errorf("解析 authorize_url 失败: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", c.cfg.RedirectURI)
	q.Set("scope", c.cfg.Scope)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	if c.cfg.Prompt != "" {
		q.Set("prompt", c.cfg.Prompt)
	}

	// 对齐 Codex CLI 常见参数（提升兼容性）。
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("id_token_add_organizations", "true")

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) ExchangeCode(ctx context.Context, code string, codeVerifier string) (TokenResult, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("redirect_uri", c.cfg.RedirectURI)
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	out, err := c.token(ctx, form)
	if err != nil && ctx.Err() == nil && isRetryableCodeExchangeError(err) {
		out, err = c.token(ctx, form)
	}
	if err != nil {
		return TokenResult{}, err
	}
	if out.RefreshToken == "" {
		return TokenResult{}, errors.New("token 响应缺少 refresh_token")
	}
	if out.IDToken == "" {
		return TokenResult{}, errors.New("token 响应缺少 id_token")
	}
	return out, nil
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (TokenResult, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("refresh_token", refreshToken)
	// 对齐 CLIProxyAPI 的 refresh 请求形态（OpenAI 侧不一定要求，但有助于保持一致性）。
	form.Set("scope", "openid profile email")
	backoffs := c.refreshBackoffs
	if len(backoffs) == 0 {
		backoffs = []time.Duration{2 * time.Second, 5 * time.Second}
	}
	var lastErr error
	for attempt := 0; attempt <= len(backoffs); attempt++ {
		out, err := c.token(ctx, form)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isRetryableRefreshError(err) || attempt == len(backoffs) {
			return TokenResult{}, Wrap(ErrRefreshFailed, err)
		}
		t := time.NewTimer(backoffs[attempt])
		select {
		case <-ctx.Done():
			t.Stop()
			return TokenResult{}, ctx.Err()
		case <-t.C:
		}
	}
	return TokenResult{}, Wrap(ErrRefreshFailed, lastErr)
}

func (c *Client) token(ctx context.Context, form url.Values) (TokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return TokenResult{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var parsed struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &parsed); err == nil && (parsed.Error != "" || parsed.ErrorDescription != "") {
			return TokenResult{}, &TokenEndpointError{
				StatusCode:       resp.StatusCode,
				ErrorCode:        parsed.Error,
				ErrorDescription: parsed.ErrorDescription,
			}
		}
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		if msg == "" {
			msg = "(empty)"
		}
		return TokenResult{}, &TokenEndpointError{
			StatusCode:  resp.StatusCode,
			BodySnippet: msg,
		}
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return TokenResult{}, err
	}
	if out.AccessToken == "" {
		return TokenResult{}, errors.New("token 响应缺少 access_token")
	}
	var expiresAt *time.Time
	if out.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	return TokenResult{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		IDToken:      out.IDToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func isRetryableRefreshError(err error) bool {
	var te *TokenEndpointError
	if errors.As(err, &te) {
		return te.Retryable()
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
	if strings.Contains(err.Error(), "TLS handshake timeout") {
		return true
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe != nil && oe.Op == "dial" {
		return true
	}
	return false
}

func isRetryableCodeExchangeError(err error) bool {
	if err == nil {
		return false
	}
	var te *TokenEndpointError
	if errors.As(err, &te) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// 仅对“明确未建立连接”的错误做一次重试，避免授权码被消费两次的风险。
	if strings.Contains(err.Error(), "TLS handshake timeout") {
		return true
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe != nil && oe.Op == "dial" {
		return true
	}
	return false
}
