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
	"strings"
	"sync"
	"time"

	"realms/internal/codexoauth"
	"realms/internal/config"
	"realms/internal/scheduler"
	"realms/internal/security"
	"realms/internal/store"
)

type Executor struct {
	st upstreamStore

	client *http.Client

	upstreamTimeout time.Duration

	codexOAuth              *codexoauth.Client
	codexRequestPassthrough bool
	refreshMu               sync.Mutex
	lastRefresh             map[int64]time.Time
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
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.Limits.UpstreamDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   cfg.Limits.UpstreamTLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.Limits.UpstreamResponseHeaderTimout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var codexClient *codexoauth.Client
	if cfg.CodexOAuth.Enable {
		codexClient = codexoauth.NewClient(cfg.CodexOAuth)
	}
	return &Executor{
		st:                      st,
		client:                  client,
		upstreamTimeout:         cfg.Limits.UpstreamRequestTimeout,
		codexOAuth:              codexClient,
		codexRequestPassthrough: cfg.CodexOAuth.RequestPassthrough,
		lastRefresh:             make(map[int64]time.Time),
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
	resp, err := e.client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	// Codex OAuth：部分上游的 path 仍停留在旧版 /responses；也可能反过来只接受 /v1/responses。
	// 为减少“配置正确但路径不兼容”导致的误报，这里在 404（以及部分返回 HTML 的 403）时做一次互斥形态的兜底重试。
	if sel.CredentialType == scheduler.CredentialTypeCodex && resp != nil &&
		(resp.StatusCode == http.StatusNotFound ||
			(resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html"))) {
		altPassthrough := !e.codexRequestPassthrough
		req2, err2 := e.buildRequestWithCodexPassthrough(ctx, sel, downstream, body, altPassthrough)
		if err2 == nil {
			resp2, err2 := e.client.Do(req2)
			if err2 == nil && resp2 != nil {
				if resp2.StatusCode != http.StatusNotFound {
					if resp.Body != nil {
						_ = resp.Body.Close()
					}
					resp = resp2
				} else if resp2.Body != nil {
					_ = resp2.Body.Close()
				}
			} else if resp2 != nil && resp2.Body != nil {
				_ = resp2.Body.Close()
			}
		}
	}
	if cancel != nil && resp != nil && resp.Body != nil {
		resp.Body = cancelOnClose(resp.Body, cancel)
	}
	return resp, nil
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
	return e.buildRequestWithCodexPassthrough(ctx, sel, downstream, body, e.codexRequestPassthrough)
}

func (e *Executor) buildRequestWithCodexPassthrough(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte, codexRequestPassthrough bool) (*http.Request, error) {
	base, err := security.ValidateBaseURL(sel.BaseURL)
	if err != nil {
		return nil, err
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
		// 旧版兼容逻辑：将 /v1/responses 映射到 Codex 后端的 /responses，并对请求体做兼容改写。
		// 当 request_passthrough=true 时，保持 URL path 与请求体不变，直接透传给上游。
		if !codexRequestPassthrough {
			// codex oauth 上游的路径约定为 /backend-api/codex/responses。
			targetPath = "/responses"
			body = normalizeCodexRequestBody(body)
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
	req.Header.Set("Accept-Encoding", "identity")

	switch sel.CredentialType {
	case scheduler.CredentialTypeOpenAI:
		sec, err := e.st.GetOpenAICompatibleCredentialSecret(ctx, sel.CredentialID)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+sec.APIKey)
	case scheduler.CredentialTypeAnthropic:
		sec, err := e.st.GetAnthropicCredentialSecret(ctx, sel.CredentialID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Header.Get("anthropic-version")) == "" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		req.Header.Set("x-api-key", sec.APIKey)
	case scheduler.CredentialTypeCodex:
		sec, err := e.st.GetCodexOAuthSecret(ctx, sel.CredentialID)
		if err != nil {
			return nil, err
		}

		applyCodexHeaders(req.Header, sec.AccountID)

		accessToken := sec.AccessToken
		if e.codexOAuth != nil && sec.ExpiresAt != nil && time.Until(*sec.ExpiresAt) < 5*time.Minute {
			now := time.Now()
			if e.shouldAttemptRefresh(sel.CredentialID, now) {
				refreshed, err := e.codexOAuth.Refresh(ctx, sec.RefreshToken)
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

		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
	}

	return req, nil
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

func normalizeCodexRequestBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	// 对齐 CLIProxyAPI：Codex 上游始终启用 stream，并补齐必要字段。
	payload["stream"] = true
	payload["store"] = false
	payload["parallel_tool_calls"] = true
	payload["include"] = []string{"reasoning.encrypted_content"}

	delete(payload, "max_output_tokens")
	delete(payload, "max_completion_tokens")
	delete(payload, "temperature")
	delete(payload, "top_p")
	delete(payload, "service_tier")

	model := strings.TrimSpace(stringFromAny(payload["model"]))
	officialInstructions := codexInstructionsForModel(model)
	rawInstructions, _ := payload["instructions"].(string)
	userInstructions := strings.TrimSpace(rawInstructions)
	switch {
	case userInstructions == "":
		payload["instructions"] = officialInstructions
	case isOfficialCodexInstructions(rawInstructions):
		// 已是官方/有效 instructions，保持不变。
	default:
		// Codex 上游会校验 instructions（必须为官方 prompt）。把用户自定义 instructions 转移到 input 内，并注入官方 instructions。
		payload["instructions"] = officialInstructions
		input := normalizeCodexInput(payload["input"])
		payload["input"] = prependUserInstructionsToCodexInput(input, userInstructions)
	}

	if input, ok := payload["input"]; ok {
		payload["input"] = normalizeCodexInput(input)
	}

	delete(payload, "previous_response_id")
	delete(payload, "prompt_cache_retention")

	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

func prependUserInstructionsToCodexInput(input any, instructions string) any {
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return input
	}

	var arr []any
	if v, ok := input.([]any); ok {
		arr = v
	} else if input != nil {
		arr = []any{input}
	} else {
		arr = []any{}
	}

	msg := map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{
			map[string]any{
				"type": "input_text",
				"text": "EXECUTE ACCORDING TO THE FOLLOWING INSTRUCTIONS!!!",
			},
			map[string]any{
				"type": "input_text",
				"text": instructions,
			},
		},
	}
	return append([]any{msg}, arr...)
}

func normalizeCodexInput(input any) any {
	switch v := input.(type) {
	case string:
		return []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": v,
					},
				},
			},
		}
	case map[string]any:
		return []any{normalizeCodexInputItem(v)}
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, normalizeCodexInputItem(m))
				continue
			}
			out = append(out, item)
		}
		return out
	default:
		return input
	}
}

func normalizeCodexInputItem(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	if _, ok := m["type"]; !ok {
		// OpenAI Responses 里常见的是 {"role":"user","content":...}，Codex 上游通常要求显式 type=message。
		m["type"] = "message"
	}
	switch c := m["content"].(type) {
	case string:
		m["content"] = []any{
			map[string]any{
				"type": "input_text",
				"text": c,
			},
		}
	case []any:
		// 兼容常见的 {"type":"text","text":"..."}，在 Codex 侧映射为 input_text。
		for i := range c {
			part, ok := c[i].(map[string]any)
			if !ok {
				continue
			}
			if t, ok := part["type"].(string); ok && t == "text" {
				part["type"] = "input_text"
			}
			c[i] = part
		}
		m["content"] = c
	}
	return m
}

func stringFromAny(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	default:
		return ""
	}
}

func applyCodexHeaders(h http.Header, accountID string) {
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "text/event-stream")
	h.Set("Connection", "Keep-Alive")

	if strings.TrimSpace(h.Get("Version")) == "" {
		h.Set("Version", "0.21.0")
	}
	if strings.TrimSpace(h.Get("Session_id")) == "" {
		if v := newUUIDv4(); v != "" {
			h.Set("Session_id", v)
		}
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
