package server

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"realms/internal/security"
	"realms/internal/store"
)

func testChannelOnce(ctx context.Context, st *store.Store, channelID int64) (ok bool, latencyMS int, message string) {
	if st == nil {
		return false, 0, "store 未初始化"
	}
	ch, err := st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0, "channel 不存在"
		}
		return false, 0, "查询 channel 失败"
	}
	if ch.Type == store.UpstreamTypeCodexOAuth {
		return false, 0, "codex_oauth Channel 不支持测试"
	}

	ep, err := st.GetUpstreamEndpointByChannelID(ctx, ch.ID)
	if err != nil || ep.ID <= 0 {
		return false, 0, "endpoint 不存在"
	}

	start := time.Now()
	ok, msg := probeUpstream(ctx, st, ch, ep)
	latencyMS = int(time.Since(start).Milliseconds())
	if latencyMS < 0 {
		latencyMS = 0
	}
	_ = st.UpdateUpstreamChannelTest(ctx, ch.ID, ok, latencyMS)
	return ok, latencyMS, msg
}

func probeUpstream(ctx context.Context, st *store.Store, ch store.UpstreamChannel, ep store.UpstreamEndpoint) (bool, string) {
	targetPath := "/v1/models"
	method := http.MethodGet
	var body io.Reader

	h := make(http.Header)
	h.Set("Accept", "application/json")
	h.Set("User-Agent", "realms-channel-test/1.0")

	switch ch.Type {
	case store.UpstreamTypeOpenAICompatible:
		creds, err := st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
		if err != nil || len(creds) == 0 {
			return false, "暂无可用 key"
		}
		sec, err := st.GetOpenAICompatibleCredentialSecret(ctx, creds[0].ID)
		if err != nil || strings.TrimSpace(sec.APIKey) == "" {
			return false, "读取 key 失败"
		}
		h.Set("Authorization", "Bearer "+strings.TrimSpace(sec.APIKey))
	case store.UpstreamTypeAnthropic:
		// 只做连通性探测：POST /v1/messages 发送空 JSON，期望返回 400（参数错误）或 2xx。
		method = http.MethodPost
		targetPath = "/v1/messages"
		body = strings.NewReader(`{}`)
		h.Set("Content-Type", "application/json; charset=utf-8")
		h.Set("anthropic-version", "2023-06-01")
		creds, err := st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
		if err != nil || len(creds) == 0 {
			return false, "暂无可用 key"
		}
		sec, err := st.GetAnthropicCredentialSecret(ctx, creds[0].ID)
		if err != nil || strings.TrimSpace(sec.APIKey) == "" {
			return false, "读取 key 失败"
		}
		h.Set("x-api-key", strings.TrimSpace(sec.APIKey))
	default:
		return false, "不支持的渠道类型"
	}

	u, err := buildUpstreamURL(ep.BaseURL, targetPath)
	if err != nil {
		return false, "base_url 不合法"
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return false, "创建请求失败"
	}
	for k, vs := range h {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "请求失败"
	}
	defer resp.Body.Close()

	switch ch.Type {
	case store.UpstreamTypeAnthropic:
		// 400 表示服务可达但参数不完整；401/403 表示 key 不可用。
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true, "OK"
		}
		if resp.StatusCode == http.StatusBadRequest {
			return true, "OK（400 参数错误，连通性正常）"
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return false, "鉴权失败（" + strconv.Itoa(resp.StatusCode) + "）"
		}
		if len(b) > 0 {
			return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）: " + strings.TrimSpace(string(b))
		}
		return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）"
	default:
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true, "OK"
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return false, "鉴权失败（" + strconv.Itoa(resp.StatusCode) + "）"
		}
		if len(b) > 0 {
			return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）: " + strings.TrimSpace(string(b))
		}
		return false, "失败（" + strconv.Itoa(resp.StatusCode) + "）"
	}
}

func buildUpstreamURL(baseURL string, targetPath string) (string, error) {
	base, err := security.ValidateBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(base.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(targetPath, "/v1/") {
		targetPath = strings.TrimPrefix(targetPath, "/v1")
		if targetPath == "" {
			targetPath = "/"
		}
	}
	return base.ResolveReference(&url.URL{Path: targetPath}).String(), nil
}
