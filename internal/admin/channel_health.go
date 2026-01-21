package admin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"realms/internal/scheduler"
	"realms/internal/store"
)

func (s *Server) TestChannel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusUnauthorized, "未登录")
			return
		}
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if s.exec == nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "上游执行器未初始化")
			return
		}
		http.Error(w, "上游执行器未初始化", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "表单解析失败")
			return
		}
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	returnTo := safeAdminReturnTo(r.FormValue("return_to"), "/admin/channels")

	pathID := strings.TrimSpace(r.PathValue("channel_id"))
	formID := strings.TrimSpace(r.FormValue("channel_id"))
	rawID := pathID
	channelID, err := parseInt64(rawID)
	if err != nil || channelID <= 0 {
		if formID != "" && formID != pathID {
			if id2, err2 := parseInt64(formID); err2 == nil && id2 > 0 {
				channelID = id2
				err = nil
			}
		}
	}
	if err != nil || channelID <= 0 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "channel_id 不合法")
			return
		}
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	ch, err := s.st.GetUpstreamChannelByID(r.Context(), channelID)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusNotFound, "channel 不存在")
			return
		}
		http.Error(w, "channel 不存在", http.StatusNotFound)
		return
	}

	var specified []store.ChannelModel
	if raw := strings.TrimSpace(r.FormValue("binding_id")); raw != "" {
		id, err := parseInt64(raw)
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "binding_id 不合法")
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("binding_id 不合法"), http.StatusFound)
			return
		}
		cm, err := s.st.GetChannelModelByID(r.Context(), id)
		if err != nil || cm.ChannelID != ch.ID {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "绑定不存在")
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("绑定不存在"), http.StatusFound)
			return
		}
		specified = append(specified, cm)
	}

	msg, err := runChannelTest(r.Context(), s.st, s.exec, ch, specified)
	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "\n", " ")
		if len(errMsg) > 200 {
			errMsg = errMsg[:200] + "..."
		}
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, errMsg)
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape(errMsg), http.StatusFound)
		return
	}
	if strings.TrimSpace(msg) != "" {
		if isAjax(r) {
			ajaxOK(w, msg)
			return
		}
		http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape(msg), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "测试完成（结果已更新）")
		return
	}
	http.Redirect(w, r, returnTo, http.StatusFound)
}

type channelTestStore interface {
	ListUpstreamEndpointsByChannel(ctx context.Context, channelID int64) ([]store.UpstreamEndpoint, error)
	ListOpenAICompatibleCredentialsByEndpoint(ctx context.Context, endpointID int64) ([]store.OpenAICompatibleCredential, error)
	ListCodexOAuthAccountsByEndpoint(ctx context.Context, endpointID int64) ([]store.CodexOAuthAccount, error)
	ListChannelModelsByChannelID(ctx context.Context, channelID int64) ([]store.ChannelModel, error)
	GetEnabledManagedModelByPublicID(ctx context.Context, publicID string) (store.ManagedModel, error)
	UpdateUpstreamChannelTest(ctx context.Context, channelID int64, ok bool, latencyMS int) error
}

func runChannelTest(ctx context.Context, st channelTestStore, exec UpstreamDoer, ch store.UpstreamChannel, specified []store.ChannelModel) (string, error) {
	candidates, auto, err := gatherChannelTestModels(ctx, st, ch, specified)
	if err != nil {
		_ = st.UpdateUpstreamChannelTest(ctx, ch.ID, false, 0)
		return "", err
	}
	timeout := calcChannelTestTimeout(len(candidates))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	sels, err := listChannelTestSelections(ctx, st, ch, start)
	if err != nil {
		if upErr := st.UpdateUpstreamChannelTest(ctx, ch.ID, false, 0); upErr != nil {
			return "", upErr
		}
		return "", fmt.Errorf("测试准备失败: %w", err)
	}

	results := make([]channelTestResult, 0, len(candidates))
	for _, cm := range candidates {
		perCtx, perCancel := context.WithTimeout(ctx, 20*time.Second)
		res, used := runSingleModelSSETestWithFailover(perCtx, exec, sels, cm.PublicID, cm.UpstreamModel)
		perCancel()
		results = append(results, res)
		if res.OK && used > 0 && used < len(sels) {
			sels[0], sels[used] = sels[used], sels[0]
		}
	}

	ok, latencyMS := summarizeChannelTestResults(results, auto)
	_ = st.UpdateUpstreamChannelTest(ctx, ch.ID, ok, latencyMS)

	if ok {
		return trimSummary(channelTestOKMessage(results, latencyMS, auto)), nil
	}
	return "", fmt.Errorf(trimSummary(channelTestFailMessage(results, latencyMS, auto)))
}

func defaultTestInput() string {
	return "对话：\nUser: 你好\nAssistant: 你好！\nUser: 请只回复 pong（小写），不要输出任何解释。\nAssistant:"
}

type channelTestModel struct {
	PublicID      string
	UpstreamModel string
}

type channelTestResult struct {
	PublicID      string
	UpstreamModel string
	OK            bool
	TTFTMS        int
	Sample        string
	Err           string
}

func calcChannelTestTimeout(n int) time.Duration {
	if n <= 0 {
		return 20 * time.Second
	}
	total := time.Duration(n) * 20 * time.Second
	if total > 90*time.Second {
		total = 90 * time.Second
	}
	if total < 20*time.Second {
		total = 20 * time.Second
	}
	return total
}

func gatherChannelTestModels(ctx context.Context, st channelTestStore, ch store.UpstreamChannel, specified []store.ChannelModel) ([]channelTestModel, bool, error) {
	if len(specified) > 0 {
		out := make([]channelTestModel, 0, len(specified))
		for _, cm := range specified {
			up := strings.TrimSpace(cm.UpstreamModel)
			if up == "" {
				up = strings.TrimSpace(cm.PublicID)
			}
			out = append(out, channelTestModel{PublicID: cm.PublicID, UpstreamModel: up})
		}
		return out, false, nil
	}

	cms, err := st.ListChannelModelsByChannelID(ctx, ch.ID)
	if err != nil {
		return nil, false, fmt.Errorf("查询渠道模型绑定失败: %w", err)
	}
	var out []channelTestModel
	for _, cm := range cms {
		if cm.Status != 1 {
			continue
		}
		if _, err := st.GetEnabledManagedModelByPublicID(ctx, cm.PublicID); err != nil {
			continue
		}
		up := strings.TrimSpace(cm.UpstreamModel)
		if up == "" {
			up = strings.TrimSpace(cm.PublicID)
		}
		out = append(out, channelTestModel{PublicID: cm.PublicID, UpstreamModel: up})
	}
	if len(out) > 0 {
		return out, true, nil
	}

	m := defaultTestModel(ch.Type)
	return []channelTestModel{{PublicID: m, UpstreamModel: m}}, true, nil
}

func runSingleModelSSETestWithFailover(ctx context.Context, exec UpstreamDoer, sels []scheduler.Selection, publicID string, upstreamModel string) (channelTestResult, int) {
	r := channelTestResult{PublicID: publicID, UpstreamModel: upstreamModel}
	if len(sels) == 0 {
		r.Err = "未找到可用上游 credential/account"
		return r, -1
	}

	lastUsed := -1
	for i, sel := range sels {
		lastUsed = i
		out, retryable := runSingleModelSSETest(ctx, exec, sel, publicID, upstreamModel)
		if out.OK {
			return out, i
		}
		r = out
		if !retryable {
			return out, i
		}
		if ctx.Err() != nil {
			break
		}
	}
	return r, lastUsed
}

func runSingleModelSSETest(ctx context.Context, exec UpstreamDoer, sel scheduler.Selection, publicID string, upstreamModel string) (channelTestResult, bool) {
	start := time.Now()
	r := channelTestResult{PublicID: publicID, UpstreamModel: upstreamModel}

	payload := map[string]any{
		"model":             upstreamModel,
		"input":             defaultTestInput(),
		"max_output_tokens": int64(16),
		"stream":            true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		r.Err = "构造测试请求失败"
		return r, false
	}

	downstream, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/v1/responses", http.NoBody)
	downstream.Header.Set("Content-Type", "application/json")
	downstream.Header.Set("Accept", "text/event-stream")

	resp, err := exec.Do(ctx, sel, downstream, body)
	if err != nil {
		r.TTFTMS = int(time.Since(start).Milliseconds())
		r.Err = "请求上游失败"
		return r, true
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
		r.TTFTMS = int(time.Since(start).Milliseconds())
		if msg := summarizeUpstreamErrorBody(b); msg != "" {
			r.Err = fmt.Sprintf("上游状态码 %d: %s", resp.StatusCode, msg)
		} else {
			r.Err = fmt.Sprintf("上游状态码 %d", resp.StatusCode)
		}
		return r, isRetriableTestStatus(resp.StatusCode)
	}
	prefix := newCappedBuffer(256 << 10)
	ttftMS, sample, err := readSSESample(io.TeeReader(resp.Body, prefix), start)
	if ttftMS > 0 {
		r.TTFTMS = ttftMS
	} else {
		r.TTFTMS = int(time.Since(start).Milliseconds())
	}
	r.Sample = sample
	if err != nil {
		r.Err = err.Error()
		return r, true
	}
	if strings.TrimSpace(sample) == "" {
		// 某些上游会返回 SSE 事件体但缺少 Content-Type 头；这里优先按内容尝试解析，解析不到再按 header 判定错误原因。
		if !isSSE {
			if msg := summarizeUpstreamErrorBody(prefix.Bytes()); msg != "" {
				r.Err = fmt.Sprintf("未返回 SSE（Content-Type=%q）：%s", contentType, msg)
			} else {
				r.Err = fmt.Sprintf("未返回 SSE（Content-Type=%q）", contentType)
			}
			return r, true
		}
		r.Err = "SSE 未收到有效输出"
		return r, true
	}
	r.OK = true
	return r, false
}

func summarizeChannelTestResults(results []channelTestResult, auto bool) (bool, int) {
	allOK := true
	hasOK := false
	minOKTTFT := 0
	minAnyTTFT := 0
	for _, r := range results {
		if r.OK {
			hasOK = true
			if minOKTTFT == 0 || (r.TTFTMS > 0 && r.TTFTMS < minOKTTFT) {
				minOKTTFT = r.TTFTMS
			}
		} else {
			allOK = false
		}
		if minAnyTTFT == 0 || (r.TTFTMS > 0 && r.TTFTMS < minAnyTTFT) {
			minAnyTTFT = r.TTFTMS
		}
	}
	if auto && len(results) > 1 {
		if allOK {
			return true, minOKTTFT
		}
		if hasOK {
			return false, minOKTTFT
		}
		return false, minAnyTTFT
	}
	if allOK {
		return true, minOKTTFT
	}
	if hasOK {
		return false, minOKTTFT
	}
	return false, minAnyTTFT
}

func channelTestOKMessage(results []channelTestResult, latencyMS int, auto bool) string {
	if len(results) == 1 {
		r := results[0]
		return fmt.Sprintf("流式测试成功：model=%s（%s），TTFT %dms，示例输出：%s", r.PublicID, r.UpstreamModel, latencyMS, r.Sample)
	}
	return fmt.Sprintf("流式测试成功：共 %d 个模型，最快 TTFT %dms", len(results), latencyMS)
}

func channelTestFailMessage(results []channelTestResult, latencyMS int, auto bool) string {
	if len(results) == 1 {
		r := results[0]
		return fmt.Sprintf("流式测试失败：model=%s（%s），TTFT %dms，原因：%s", r.PublicID, r.UpstreamModel, latencyMS, trimSummary(r.Err))
	}
	okCount := 0
	var failed []string
	for _, r := range results {
		if r.OK {
			okCount++
			continue
		}
		reason := trimSummary(r.Err)
		if reason == "" {
			reason = "unknown"
		}
		failed = append(failed, fmt.Sprintf("%s(%s)", r.PublicID, reason))
		if len(failed) >= 3 {
			break
		}
	}
	return fmt.Sprintf("流式测试失败：成功 %d/%d，最快 TTFT %dms，失败示例：%s", okCount, len(results), latencyMS, strings.Join(failed, "；"))
}

func safeAdminReturnTo(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if strings.Contains(raw, "://") {
		return fallback
	}
	if strings.ContainsAny(raw, "\r\n") {
		return fallback
	}
	if !strings.HasPrefix(raw, "/admin/") {
		return fallback
	}
	return raw
}

func extractTestSample(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// SSE（event-stream）: 尝试从 data: JSON 事件中提取增量文本。
	if bytes.Contains(b, []byte("data:")) {
		if s := extractSampleFromSSE(b); s != "" {
			return s
		}
	}
	// JSON: 尝试从 Responses 的常见结构里提取文本。
	if s := extractSampleFromJSON(b); s != "" {
		return s
	}
	return trimSummary(string(b))
}

type cappedBuffer struct {
	buf       bytes.Buffer
	remaining int
}

func newCappedBuffer(limit int) *cappedBuffer {
	if limit < 0 {
		limit = 0
	}
	return &cappedBuffer{remaining: limit}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	if b.remaining > 0 && len(p) > 0 {
		if len(p) <= b.remaining {
			_, _ = b.buf.Write(p)
			b.remaining -= len(p)
		} else {
			_, _ = b.buf.Write(p[:b.remaining])
			b.remaining = 0
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.buf.Bytes()
}

func readSSESample(r io.Reader, start time.Time) (int, string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 256<<10)

	ttftMS := 0
	var out strings.Builder
	events := 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if ttftMS == 0 {
			ttftMS = int(time.Since(start).Milliseconds())
		}
		if data == "[DONE]" {
			break
		}

		events++
		var evt map[string]any
		if err := json.Unmarshal([]byte(data), &evt); err == nil {
			// OpenAI Responses streaming: {"type":"response.output_text.delta","delta":"..."}
			if delta, ok := evt["delta"].(string); ok && strings.TrimSpace(delta) != "" {
				out.WriteString(delta)
			}
			// 兜底：部分实现可能给 {"text":"..."}
			if out.Len() == 0 {
				if text, ok := evt["text"].(string); ok && strings.TrimSpace(text) != "" {
					out.WriteString(text)
				}
			}
		}
		if out.Len() >= 200 || events >= 12 {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return ttftMS, trimSummary(out.String()), fmt.Errorf("SSE 读取失败: %w", err)
	}
	return ttftMS, trimSummary(out.String()), nil
}

func extractSampleFromSSE(b []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64<<10), 256<<10)
	var out strings.Builder
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		if delta, ok := evt["delta"].(string); ok && strings.TrimSpace(delta) != "" {
			out.WriteString(delta)
		}
		if text, ok := evt["text"].(string); ok && strings.TrimSpace(text) != "" && out.Len() == 0 {
			out.WriteString(text)
		}
		if out.Len() >= 200 {
			break
		}
	}
	return trimSummary(out.String())
}

func extractSampleFromJSON(b []byte) string {
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return ""
	}
	// Responses: output_text
	if s, ok := root["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return trimSummary(s)
	}
	// Responses: output[*].content[*].text
	if outputAny, ok := root["output"]; ok {
		if outs, ok := outputAny.([]any); ok {
			for _, itemAny := range outs {
				item, ok := itemAny.(map[string]any)
				if !ok {
					continue
				}
				contentAny, ok := item["content"]
				if !ok {
					continue
				}
				parts, ok := contentAny.([]any)
				if !ok {
					continue
				}
				for _, partAny := range parts {
					part, ok := partAny.(map[string]any)
					if !ok {
						continue
					}
					if s, ok := part["text"].(string); ok && strings.TrimSpace(s) != "" {
						return trimSummary(s)
					}
				}
			}
		}
	}
	return ""
}

func defaultTestModel(channelType string) string {
	switch channelType {
	case store.UpstreamTypeCodexOAuth:
		return "gpt-5.2"
	default:
		// 用户期望测试走 gpt-5.2（避免“测试模型与生产模型不一致”导致的误判）。
		return "gpt-5.2"
	}
}

func listChannelTestSelections(ctx context.Context, st channelTestStore, ch store.UpstreamChannel, now time.Time) ([]scheduler.Selection, error) {
	endpoints, err := st.ListUpstreamEndpointsByChannel(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	var sels []scheduler.Selection
	for _, ep := range endpoints {
		if ep.Status != 1 {
			continue
		}

		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			creds, err := st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
			if err != nil {
				return nil, err
			}
			for _, c := range creds {
				if c.Status != 1 {
					continue
				}
				sels = append(sels, scheduler.Selection{
					ChannelID:      ch.ID,
					ChannelType:    ch.Type,
					EndpointID:     ep.ID,
					BaseURL:        ep.BaseURL,
					CredentialType: scheduler.CredentialTypeOpenAI,
					CredentialID:   c.ID,
				})
			}
		case store.UpstreamTypeCodexOAuth:
			accs, err := st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
			if err != nil {
				return nil, err
			}
			for _, a := range accs {
				if a.Status != 1 {
					continue
				}
				if a.CooldownUntil != nil && now.Before(*a.CooldownUntil) {
					continue
				}
				sels = append(sels, scheduler.Selection{
					ChannelID:      ch.ID,
					ChannelType:    ch.Type,
					EndpointID:     ep.ID,
					BaseURL:        ep.BaseURL,
					CredentialType: scheduler.CredentialTypeCodex,
					CredentialID:   a.ID,
				})
			}
		default:
			return nil, fmt.Errorf("不支持的 channel type: %s", ch.Type)
		}
	}
	if len(sels) == 0 {
		return nil, fmt.Errorf("未找到可用 endpoint/credential")
	}
	return sels, nil
}

func selectChannelTestSelection(ctx context.Context, st channelTestStore, ch store.UpstreamChannel, now time.Time) (scheduler.Selection, error) {
	sels, err := listChannelTestSelections(ctx, st, ch, now)
	if err != nil {
		return scheduler.Selection{}, err
	}
	return sels[0], nil
}

func summarizeUpstreamErrorBody(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err == nil && len(parsed) > 0 {
		// 常见错误结构：{"error": {"message": "..."}}
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
				return trimSummary(msg)
			}
			if msg, ok := errObj["error_description"].(string); ok && strings.TrimSpace(msg) != "" {
				return trimSummary(msg)
			}
		}
		// 兼容：{"message": "..."} / {"error_description": "..."}
		if msg, ok := parsed["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return trimSummary(msg)
		}
		if msg, ok := parsed["error_description"].(string); ok && strings.TrimSpace(msg) != "" {
			return trimSummary(msg)
		}
	}
	return trimSummary(string(b))
}

func trimSummary(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func isRetriableTestStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		// 很多场景是凭据失效/配额/余额问题，切换 key/账号有意义。
		return true
	default:
		if code >= 500 {
			return true
		}
		return false
	}
}
