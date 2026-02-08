// Package upstream 封装对上游的 HTTP 调用：构造目标 URL、注入鉴权、控制超时与禁止重定向。
package upstream

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"realms/internal/codexoauth"
	"realms/internal/config"
	"realms/internal/scheduler"
	"realms/internal/security"
	"realms/internal/store"
)

type Executor struct {
	st upstreamStore

	client    *http.Client
	clientsMu sync.Mutex
	clients   map[string]*http.Client

	upstreamTimeout time.Duration

	refreshMu   sync.Mutex
	lastRefresh map[int64]time.Time
}

type upstreamStore interface {
	GetOpenAICompatibleCredentialSecret(ctx context.Context, credentialID int64) (store.OpenAICredentialSecret, error)
	GetAnthropicCredentialSecret(ctx context.Context, credentialID int64) (store.AnthropicCredentialSecret, error)
	GetCodexOAuthSecret(ctx context.Context, accountID int64) (store.CodexOAuthSecret, error)
	UpdateCodexOAuthAccountTokens(ctx context.Context, accountID int64, accessToken, refreshToken string, idToken *string, expiresAt *time.Time) error
	SetCodexOAuthAccountStatus(ctx context.Context, accountID int64, status int) error
	SetCodexOAuthAccountCooldown(ctx context.Context, accountID int64, until time.Time) error
}

func NewExecutor(st *store.Store, cfg config.Config) *Executor {
	transport := defaultTransportWithProxy(http.ProxyFromEnvironment, (&net.Dialer{
		Timeout:   0,
		KeepAlive: 30 * time.Second,
	}).DialContext)
	client := &http.Client{
		Transport: transport,
		Timeout:   0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Executor{
		st:              st,
		client:          client,
		clients:         make(map[string]*http.Client),
		upstreamTimeout: 0,
		lastRefresh:     make(map[int64]time.Time),
	}
}

func (e *Executor) Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
	ctx, cancel := e.wrapTimeout(ctx, sel, downstream, body)
	if cancel != nil {
		defer func() {
			if ctx.Err() != nil {
				cancel()
			}
		}()
	}

	req, err := e.buildRequest(ctx, sel, downstream, body)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	client := e.clientForSelection(sel)
	resp, err := client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	// OpenAI-compatible：部分上游只接受 legacy max_tokens，返回 400 "Unsupported parameter: max_output_tokens"。
	// 为减少误报/兼容更多代理实现，这里对该错误做一次无损改写重试（max_output_tokens -> max_tokens）。
	if sel.CredentialType == scheduler.CredentialTypeOpenAI && resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 {
		b, _ := peekResponseBody(resp, 32<<10)
		// 兼容两种常见形态：
		// - upstream 不支持 Responses 风格 max_output_tokens，只接受 max_tokens
		// - upstream 只支持 Responses 风格 max_output_tokens，不接受 legacy max_tokens
		//
		// NOTE: 不再根据“报错字段名与请求体字段名不一致”的异常实现做反向猜测（如：请求体仅含 max_output_tokens
		// 但报错提示为 max_tokens），避免把正确的请求改写成必然失败的形态并污染最终错误信息。
		// 重试仅在请求体确实包含被提示为 unsupported 的字段时触发。
		candidates := make([][]byte, 0, 1)
		switch unsupportedParameterName(b) {
		case "max_output_tokens":
			if bytes.Contains(body, []byte(`"max_output_tokens"`)) {
				candidates = append(candidates, rewriteMaxOutputTokensToMaxTokens(body))
			}
		case "max_tokens":
			if bytes.Contains(body, []byte(`"max_tokens"`)) {
				candidates = append(candidates, rewriteMaxTokensToMaxOutputTokens(body))
			}
		case "max_completion_tokens":
			if bytes.Contains(body, []byte(`"max_completion_tokens"`)) {
				candidates = append(candidates, rewriteMaxCompletionTokensToMaxTokens(body))
			}
		case "stream_options":
			if bytes.Contains(body, []byte(`"stream_options"`)) {
				candidates = append(candidates, rewriteRemoveStreamOptions(body))
			}
		}

		tried := make([][]byte, 0, len(candidates))
		for _, body2 := range candidates {
			if len(body2) == 0 || bytes.Equal(body2, body) {
				continue
			}
			dup := false
			for _, prev := range tried {
				if bytes.Equal(prev, body2) {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			tried = append(tried, body2)

			req2, err2 := e.buildRequest(ctx, sel, downstream, body2)
			if err2 != nil {
				continue
			}
			resp2, err2 := e.client.Do(req2)
			if err2 != nil || resp2 == nil {
				if resp2 != nil && resp2.Body != nil {
					_ = resp2.Body.Close()
				}
				continue
			}

			// 若已成功则直接替换并结束；失败时继续保持原始响应（避免“越改越错”导致错误信息被覆盖）。
			if resp2.StatusCode >= 200 && resp2.StatusCode < 300 {
				if resp.Body != nil {
					_ = resp.Body.Close()
				}
				resp = resp2
				break
			}
			if resp2.Body != nil {
				_ = resp2.Body.Close()
			}
		}
	}
	if cancel != nil && resp != nil && resp.Body != nil {
		resp.Body = cancelOnClose(resp.Body, cancel)
	}
	return resp, nil
}

func defaultTransportWithProxy(proxyFn func(*http.Request) (*url.URL, error), dialContext func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	return &http.Transport{
		Proxy:               proxyFn,
		DialContext:         dialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 0,
		// 对 SSE/长连接类请求，不使用 header timeout（由上层 timeout/idle 控制）。
		ResponseHeaderTimeout: 0,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func (e *Executor) clientForSelection(sel scheduler.Selection) *http.Client {
	raw := strings.TrimSpace(sel.Proxy)
	if raw == "" {
		return e.client
	}
	// 显式禁用代理：对齐 new-api 的“按渠道设置 proxy”的语义，同时保留 env 代理默认值。
	if strings.EqualFold(raw, "direct") || strings.EqualFold(raw, "none") {
		return e.clientForProxyKey("direct", func() (*http.Client, error) {
			dialer := &net.Dialer{Timeout: 0, KeepAlive: 30 * time.Second}
			tr := defaultTransportWithProxy(nil, dialer.DialContext)
			return &http.Client{
				Transport: tr,
				Timeout:   0,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}, nil
		})
	}

	u, err := url.Parse(raw)
	if err != nil {
		// 回退：proxy 配置错误时，不阻断请求（保持 env proxy 行为）。
		return e.client
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return e.clientForProxyKey(u.String(), func() (*http.Client, error) {
			dialer := &net.Dialer{Timeout: 0, KeepAlive: 30 * time.Second}
			tr := defaultTransportWithProxy(http.ProxyURL(u), dialer.DialContext)
			return &http.Client{
				Transport: tr,
				Timeout:   0,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}, nil
		})
	case "socks5", "socks5h":
		return e.clientForProxyKey(u.String(), func() (*http.Client, error) {
			host := u.Host
			if !strings.Contains(host, ":") {
				host += ":1080"
			}
			var auth *proxy.Auth
			if u.User != nil {
				pass, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: pass}
			}
			baseDialer := &net.Dialer{Timeout: 0, KeepAlive: 30 * time.Second}
			d, err := proxy.SOCKS5("tcp", host, auth, baseDialer)
			if err != nil {
				return nil, err
			}
			dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
				// proxy.Dialer 没有 DialContext；这里遵循 ctx 的取消语义（弱保证）。
				type dialRes struct {
					c   net.Conn
					err error
				}
				ch := make(chan dialRes, 1)
				go func() {
					c, err := d.Dial(network, addr)
					ch <- dialRes{c: c, err: err}
				}()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case r := <-ch:
					return r.c, r.err
				}
			}
			tr := defaultTransportWithProxy(nil, dialContext)
			return &http.Client{
				Transport: tr,
				Timeout:   0,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}, nil
		})
	default:
		return e.client
	}
}

func (e *Executor) clientForProxyKey(key string, factory func() (*http.Client, error)) *http.Client {
	e.clientsMu.Lock()
	defer e.clientsMu.Unlock()
	if c, ok := e.clients[key]; ok && c != nil {
		return c
	}
	c, err := factory()
	if err != nil {
		return e.client
	}
	e.clients[key] = c
	return c
}

type restoringReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (r *restoringReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *restoringReadCloser) Close() error {
	return r.closer.Close()
}

func peekResponseBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	if resp == nil || resp.Body == nil || maxBytes <= 0 {
		return nil, nil
	}
	origBody := resp.Body
	limited := io.LimitReader(origBody, maxBytes)
	b, err := io.ReadAll(limited)
	resp.Body = &restoringReadCloser{
		reader: io.MultiReader(bytes.NewReader(b), origBody),
		closer: origBody,
	}
	return b, err
}

var unsupportedParamRegexp = regexp.MustCompile(`(?i)unsupported parameter[^a-z0-9_]+([a-z0-9_]+)`)

func unsupportedParameterName(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		// 兼容 SSE/纯文本：部分上游会返回 `data: {...}` 或直接返回纯文本错误。
		m := unsupportedParamRegexp.FindStringSubmatch(string(body))
		if len(m) < 2 {
			return ""
		}
		return strings.ToLower(m[1])
	}
	msg := strings.TrimSpace(extractUpstreamErrorMessage(payload))
	if msg == "" {
		// 兜底：有些上游错误并非标准 error/message/detail 结构，直接在原始 body 内查找。
		m := unsupportedParamRegexp.FindStringSubmatch(string(body))
		if len(m) < 2 {
			return ""
		}
		return strings.ToLower(m[1])
	}
	m := unsupportedParamRegexp.FindStringSubmatch(msg)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(m[1])
}

func extractUpstreamErrorMessage(payload any) string {
	m, _ := payload.(map[string]any)
	if m == nil {
		return ""
	}
	if s, ok := m["detail"].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if errObj, ok := m["error"].(map[string]any); ok {
		if s, ok := errObj["message"].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	if s, ok := m["message"].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return ""
}

func rewriteMaxOutputTokensToMaxTokens(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	v, ok := payload["max_output_tokens"]
	if !ok {
		return nil
	}
	delete(payload, "max_output_tokens")

	// 兼容旧形态：把 max_output_tokens 挪到 max_tokens（若上游不接受该字段，会返回新的 400 以便继续排障）。
	if _, ok := payload["max_tokens"]; !ok {
		if n, ok := int64FromAny(v); ok {
			payload["max_tokens"] = n
		}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return out
}

func rewriteMaxTokensToMaxOutputTokens(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	v, ok := payload["max_tokens"]
	if !ok {
		return nil
	}
	delete(payload, "max_tokens")

	// 兼容旧形态：把 max_tokens 挪到 max_output_tokens（覆盖已有值，避免“客户端 max_tokens + 服务端默认 max_output_tokens”导致语义偏差）。
	if n, ok := int64FromAny(v); ok {
		payload["max_output_tokens"] = n
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return out
}

func rewriteMaxCompletionTokensToMaxTokens(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	v, ok := payload["max_completion_tokens"]
	if !ok {
		return nil
	}
	delete(payload, "max_completion_tokens")

	// 兼容旧形态：把 max_completion_tokens 挪到 max_tokens（若上游不接受该字段，会返回新的 400 以便继续排障）。
	if _, ok := payload["max_tokens"]; !ok {
		if n, ok := int64FromAny(v); ok {
			payload["max_tokens"] = n
		}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return out
}

func rewriteRemoveStreamOptions(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if _, ok := payload["stream_options"]; !ok {
		return nil
	}
	delete(payload, "stream_options")

	out, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return out
}

func int64FromAny(v any) (int64, bool) {
	switch vv := v.(type) {
	case int:
		return int64(vv), true
	case int64:
		return vv, true
	case float64:
		return int64(vv), true
	case string:
		vv = strings.TrimSpace(vv)
		if vv == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(vv, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func (e *Executor) wrapTimeout(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (context.Context, context.CancelFunc) {
	if e.upstreamTimeout <= 0 {
		return ctx, nil
	}
	// codex_oauth 上游通常以 SSE 形态返回；避免上游请求级 timeout 误伤流式长连接。
	//（更细粒度的限制由外层的 StreamAwareRequestTimeout 与 SSE idle/max duration 控制。）
	if sel.CredentialType == scheduler.CredentialTypeCodex {
		return ctx, nil
	}
	if isStreamRequest(downstream, body) {
		return ctx, nil
	}
	d := e.upstreamTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if rem := time.Until(deadline); rem > 0 && rem < d {
			d = rem
		}
	}
	ctx2, cancel := context.WithTimeout(ctx, d)
	return ctx2, cancel
}

func isStreamRequest(downstream *http.Request, body []byte) bool {
	if downstream == nil {
		return false
	}
	switch downstream.URL.Path {
	case "/v1/responses", "/v1/messages":
	default:
		return false
	}
	if strings.Contains(strings.ToLower(downstream.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	if len(body) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	switch v := payload["stream"].(type) {
	case bool:
		return v
	case string:
		v = strings.TrimSpace(v)
		return v == "1" || strings.EqualFold(v, "true")
	default:
		return false
	}
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func cancelOnClose(rc io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	return &cancelReadCloser{ReadCloser: rc, cancel: cancel}
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func (e *Executor) buildRequest(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Request, error) {
	base, err := security.ValidateBaseURL(sel.BaseURL)
	if err != nil {
		return nil, err
	}
	if sel.CredentialType == scheduler.CredentialTypeAnthropic {
		if rewritten, changed := applyAnthropicCacheTTLPreference(body, sel.CacheTTLPreference); changed {
			body = rewritten
		}
	}

	targetPath := downstream.URL.Path
	switch sel.CredentialType {
	case scheduler.CredentialTypeOpenAI:
		// 直接透传 /v1/*。
	case scheduler.CredentialTypeAnthropic:
		if targetPath != "/v1/messages" {
			return nil, errors.New("anthropic 上游仅支持 /v1/messages")
		}
	case scheduler.CredentialTypeCodex:
		if targetPath != "/v1/responses" {
			return nil, errors.New("codex_oauth 上游仅支持 /v1/responses")
		}
	default:
		return nil, errors.New("未知 credential 类型")
	}

	u := *base
	joined, err := url.JoinPath(base.String(), normalizeUpstreamPath(base, targetPath))
	if err != nil {
		return nil, fmt.Errorf("拼接上游 URL 失败: %w", err)
	}
	uu, err := url.Parse(joined)
	if err != nil {
		return nil, fmt.Errorf("解析上游 URL 失败: %w", err)
	}
	u = *uu
	u.RawQuery = downstream.URL.RawQuery
	if u.RawQuery != "" && (targetPath == "/v1/responses" || targetPath == "/v1/messages") {
		q := u.Query()
		changed := false
		for _, k := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
			if _, ok := q[k]; ok {
				q.Del(k)
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}

	req, err := http.NewRequestWithContext(ctx, downstream.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建上游请求失败: %w", err)
	}
	copyHeaders(req.Header, downstream.Header)

	// 禁止把下游鉴权与压缩语义带到上游。
	req.Header.Del("Authorization")
	req.Header.Del("X-Api-Key")
	req.Header.Del("x-api-key")
	req.Header.Del("Accept-Encoding")

	if sel.CredentialType == scheduler.CredentialTypeOpenAI && sel.OpenAIOrganization != nil && strings.TrimSpace(*sel.OpenAIOrganization) != "" {
		req.Header.Set("OpenAI-Organization", strings.TrimSpace(*sel.OpenAIOrganization))
	}

	switch sel.CredentialType {
	case scheduler.CredentialTypeOpenAI:
		sec, err := e.st.GetOpenAICompatibleCredentialSecret(ctx, sel.CredentialID)
		if err != nil {
			return nil, err
		}
		if err := applyHeaderOverride(req.Header, sel.HeaderOverride, sec.APIKey); err != nil {
			return nil, err
		}
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("Authorization", "Bearer "+sec.APIKey)
	case scheduler.CredentialTypeAnthropic:
		sec, err := e.st.GetAnthropicCredentialSecret(ctx, sel.CredentialID)
		if err != nil {
			return nil, err
		}
		if err := applyHeaderOverride(req.Header, sel.HeaderOverride, sec.APIKey); err != nil {
			return nil, err
		}
		req.Header.Set("Accept-Encoding", "identity")
		if strings.TrimSpace(req.Header.Get("anthropic-version")) == "" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		applyAnthropicBetaHeader(req.Header, sel.CacheTTLPreference)
		req.Header.Set("x-api-key", sec.APIKey)
	case scheduler.CredentialTypeCodex:
		sec, err := e.st.GetCodexOAuthSecret(ctx, sel.CredentialID)
		if err != nil {
			return nil, err
		}

		accessToken := sec.AccessToken
		if sec.ExpiresAt != nil && time.Until(*sec.ExpiresAt) < 5*time.Minute {
			now := time.Now()
			if e.shouldAttemptRefresh(sel.CredentialID, now) {
				client := codexoauth.NewClient(codexoauth.DefaultConfig(""))
				refreshed, err := client.Refresh(ctx, sec.RefreshToken)
				if err == nil {
					refreshToken := refreshed.RefreshToken
					if refreshToken == "" {
						refreshToken = sec.RefreshToken
					}
					idTokenPtr := sec.IDToken
					if refreshed.IDToken != "" {
						idToken := refreshed.IDToken
						idTokenPtr = &idToken
					}
					expiresAt := refreshed.ExpiresAt
					if expiresAt == nil {
						expiresAt = sec.ExpiresAt
					}
					_ = e.st.UpdateCodexOAuthAccountTokens(ctx, sec.ID, refreshed.AccessToken, refreshToken, idTokenPtr, expiresAt)
					accessToken = refreshed.AccessToken
				} else {
					e.recordCodexOAuthRefreshFailure(ctx, sec.ID, err, now)
				}
			}
		}

		if err := applyHeaderOverride(req.Header, sel.HeaderOverride, accessToken); err != nil {
			return nil, err
		}
		req.Header.Set("Accept-Encoding", "identity")
		applyCodexHeaders(req.Header, sec.AccountID)
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
	}

	return req, nil
}

func applyHeaderOverride(headers http.Header, headerOverride string, apiKey string) error {
	headerOverride = strings.TrimSpace(headerOverride)
	if headerOverride == "" || headerOverride == "{}" {
		return nil
	}
	parsed := make(map[string]string)
	if err := json.Unmarshal([]byte(headerOverride), &parsed); err != nil {
		return fmt.Errorf("header_override 不是有效 JSON: %w", err)
	}
	for k, v := range parsed {
		if strings.Contains(v, "{api_key}") {
			v = strings.ReplaceAll(v, "{api_key}", apiKey)
		}
		headers.Set(k, v)
	}
	return nil
}

func normalizeUpstreamPath(base *url.URL, targetPath string) string {
	basePath := strings.TrimRight(base.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(targetPath, "/v1/") {
		out := strings.TrimPrefix(targetPath, "/v1")
		if out == "" {
			return "/"
		}
		return out
	}
	return targetPath
}

func normalizeCacheTTLPreference(pref string) string {
	pref = strings.ToLower(strings.TrimSpace(pref))
	switch pref {
	case "5m", "1h":
		return pref
	default:
		return ""
	}
}

func applyAnthropicCacheTTLPreference(body []byte, pref string) ([]byte, bool) {
	ttl := normalizeCacheTTLPreference(pref)
	if ttl == "" || len(body) == 0 {
		return body, false
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false
	}
	messages, ok := payload["messages"].([]any)
	if !ok {
		return body, false
	}

	changed := false
	for i := range messages {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for j := range content {
			block, ok := content[j].(map[string]any)
			if !ok {
				continue
			}
			cacheControl, ok := block["cache_control"].(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(strings.ToLower(stringFromAny(cacheControl["type"]))) != "ephemeral" {
				continue
			}
			if strings.TrimSpace(strings.ToLower(stringFromAny(cacheControl["ttl"]))) == ttl {
				continue
			}
			cacheControl["ttl"] = ttl
			content[j] = block
			changed = true
		}
		msg["content"] = content
		messages[i] = msg
	}
	if !changed {
		return body, false
	}
	payload["messages"] = messages
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return rewritten, true
}

func applyAnthropicBetaHeader(h http.Header, pref string) {
	if normalizeCacheTTLPreference(pref) != "1h" {
		return
	}
	const extendedTTLFlag = "extended-cache-ttl-2025-04-11"
	existing := strings.TrimSpace(h.Get("anthropic-beta"))
	if existing == "" {
		h.Set("anthropic-beta", extendedTTLFlag)
		return
	}
	parts := strings.Split(existing, ",")
	set := make(map[string]struct{}, len(parts)+1)
	out := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		flag := strings.TrimSpace(part)
		if flag == "" {
			continue
		}
		lower := strings.ToLower(flag)
		if _, ok := set[lower]; ok {
			continue
		}
		set[lower] = struct{}{}
		out = append(out, flag)
	}
	if _, ok := set[strings.ToLower(extendedTTLFlag)]; !ok {
		out = append(out, extendedTTLFlag)
	}
	h.Set("anthropic-beta", strings.Join(out, ", "))
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func applyCodexHeaders(h http.Header, accountID string) {
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "text/event-stream")
	h.Set("Connection", "Keep-Alive")

	if strings.TrimSpace(h.Get("Version")) == "" {
		h.Set("Version", "0.21.0")
	}
	sessionID := strings.TrimSpace(h.Get("Session_id"))
	if sessionID == "" {
		for _, key := range []string{"Session-Id", "X-Session-Id"} {
			if v := strings.TrimSpace(h.Get(key)); v != "" {
				sessionID = v
				break
			}
		}
	}
	if sessionID == "" {
		sessionID = newUUIDv4()
	}
	if sessionID != "" {
		h.Set("Session_id", sessionID)
	}

	h.Set("User-Agent", "codex_cli_rs/0.50.0 (Mac OS 26.0.1; arm64) Apple_Terminal/464")
	h.Set("Openai-Beta", "responses=experimental")
	h.Set("Originator", "codex_cli_rs")
	if strings.TrimSpace(accountID) != "" {
		h.Set("Chatgpt-Account-Id", accountID)
	}
}

func newUUIDv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:])
}

func (e *Executor) shouldAttemptRefresh(accountID int64, now time.Time) bool {
	e.refreshMu.Lock()
	defer e.refreshMu.Unlock()
	if last, ok := e.lastRefresh[accountID]; ok {
		if now.Sub(last) < 30*time.Second {
			return false
		}
	}
	e.lastRefresh[accountID] = now
	return true
}

func (e *Executor) recordCodexOAuthRefreshFailure(ctx context.Context, accountID int64, err error, now time.Time) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	var te *codexoauth.TokenEndpointError
	if errors.As(err, &te) && te.StatusCode == http.StatusBadRequest && te.ErrorCode == "invalid_grant" {
		_ = e.st.SetCodexOAuthAccountStatus(ctx, accountID, 0)
		return
	}
	_ = e.st.SetCodexOAuthAccountCooldown(ctx, accountID, now.Add(2*time.Minute))
}

func copyHeaders(dst, src http.Header) {
	skip := map[string]struct{}{
		// Host/Content-Length 由 net/http 管理。
		"Host":           {},
		"Content-Length": {},

		// 明确禁止透传的敏感头：下游 Cookie 可能包含 realms_session 等会话信息。
		"Cookie": {},

		// RFC 7230 6.1 hop-by-hop 头（以及常见非标准头）。
		"Connection":          {},
		"Proxy-Connection":    {},
		"Keep-Alive":          {},
		"Proxy-Authenticate":  {},
		"Proxy-Authorization": {},
		"Te":                  {},
		"Trailer":             {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
	}

	// Connection 头还可以点名附加 hop-by-hop 头，必须一并剥离。
	for _, v := range src.Values("Connection") {
		for _, token := range strings.Split(v, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			skip[http.CanonicalHeaderKey(token)] = struct{}{}
		}
	}

	for k, vs := range src {
		if _, ok := skip[http.CanonicalHeaderKey(k)]; ok {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
