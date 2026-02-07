// Package openai 实现北向 OpenAI 兼容接口（responses/models），并负责 failover 与 SSE 透传边界。
package openai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/proxylog"
	"realms/internal/quota"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"
)

type ModelCatalog interface {
	GetEnabledManagedModelByPublicID(ctx context.Context, publicID string) (store.ManagedModel, error)
	GetManagedModelByPublicID(ctx context.Context, publicID string) (store.ManagedModel, error)
	ListEnabledManagedModelsWithBindings(ctx context.Context) ([]store.ManagedModel, error)
	ListEnabledChannelModelBindingsByPublicID(ctx context.Context, publicID string) ([]store.ChannelModelBinding, error)
}

type FeatureResolver interface {
	FeatureStateEffective(ctx context.Context, selfMode bool) store.FeatureState
}

type Handler struct {
	models ModelCatalog
	groups scheduler.ChannelGroupStore
	sched  *scheduler.Scheduler
	exec   Doer

	proxyLog *proxylog.Writer

	features FeatureResolver
	selfMode bool

	quota quota.Provider
	audit AuditSink
	usage UsageEventSink

	sseOpts upstream.SSEPumpOptions
}

type Doer interface {
	Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)
}

type AuditSink interface {
	InsertAuditEvent(ctx context.Context, in store.AuditEventInput) error
}

type UsageEventSink interface {
	FinalizeUsageEvent(ctx context.Context, in store.FinalizeUsageEventInput) error
}

func NewHandler(models ModelCatalog, groups scheduler.ChannelGroupStore, sched *scheduler.Scheduler, exec Doer, proxyLog *proxylog.Writer, features FeatureResolver, selfMode bool, qp quota.Provider, audit AuditSink, usage UsageEventSink, sseOpts upstream.SSEPumpOptions) *Handler {
	return &Handler{
		models:   models,
		groups:   groups,
		sched:    sched,
		exec:     exec,
		proxyLog: proxyLog,
		features: features,
		selfMode: selfMode,
		quota:    qp,
		audit:    audit,
		usage:    usage,
		sseOpts:  sseOpts,
	}
}

func (h *Handler) Responses(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r)
}

func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	if h.models == nil {
		http.Error(w, "服务未配置模型目录", http.StatusBadGateway)
		return
	}
	ms, err := h.models.ListEnabledManagedModelsWithBindings(r.Context())
	if err != nil {
		http.Error(w, "查询模型目录失败", http.StatusBadGateway)
		return
	}

	type item struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	out := struct {
		Object string `json:"object"`
		Data   []item `json:"data"`
	}{
		Object: "list",
	}
	for _, m := range ms {
		if strings.TrimSpace(m.PublicID) == "" {
			continue
		}
		ownedBy := "realms"
		if m.OwnedBy != nil && strings.TrimSpace(*m.OwnedBy) != "" {
			ownedBy = *m.OwnedBy
		}
		out.Data = append(out.Data, item{
			ID:      m.PublicID,
			Object:  "model",
			Created: 0,
			OwnedBy: ownedBy,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

func (h *Handler) proxyJSON(w http.ResponseWriter, r *http.Request) {
	reqStart := time.Now()
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	body := middleware.CachedBody(r.Context())
	if len(body) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}
	rawBody := body

	payload, err := sanitizeResponsesPayload(body)
	if err != nil {
		if errors.Is(err, errInvalidJSON) {
			http.Error(w, "请求体不是有效 JSON", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stream := boolFromAny(payload["stream"])
	publicModel := stringFromAny(payload["model"])
	maxOut := intFromAny(payload["max_output_tokens"])
	if maxOut == nil {
		maxOut = intFromAny(payload["max_tokens"])
	}
	if maxOut == nil {
		maxOut = intFromAny(payload["max_completion_tokens"])
	}

	freeMode := h.selfMode
	modelPassthrough := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context(), h.selfMode)
		freeMode = fs.BillingDisabled
		modelPassthrough = fs.ModelsDisabled
	}

	if strings.TrimSpace(publicModel) == "" {
		http.Error(w, "model 不能为空", http.StatusBadRequest)
		return
	}
	if h.models == nil {
		http.Error(w, "服务未配置模型目录", http.StatusBadGateway)
		return
	}
	var cons scheduler.Constraints
	var rewriteBody func(sel scheduler.Selection) ([]byte, error)

	var bindings []store.ChannelModelBinding
	var upstreamByChannel map[int64]string

	if modelPassthrough {
		// 非 free_mode 下仍要求模型定价存在（用于配额预留与计费口径），但不要求“启用”。
		if !freeMode {
			if _, err := h.models.GetManagedModelByPublicID(r.Context(), publicModel); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, "模型不存在", http.StatusBadRequest)
					return
				}
				http.Error(w, "查询模型失败", http.StatusBadGateway)
				return
			}
		}
		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			if sel.PassThroughBodyEnabled {
				return rawBody, nil
			}
			out := clonePayload(payload)
			applyChannelSystemPromptToResponsesPayload(out, sel)
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, err
			}
			raw, err = applyResponsesModelSuffixTransforms(raw, sel, publicModel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelRequestPolicy(raw, sel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelBodyFilters(raw, sel)
			if err != nil {
				return nil, err
			}
			ctx := buildParamOverrideContext(sel, publicModel, stringFromAny(out["model"]), r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	} else {
		_, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), publicModel)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "模型未启用", http.StatusBadRequest)
				return
			}
			http.Error(w, "查询模型失败", http.StatusBadGateway)
			return
		}
		bindings, err = h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), publicModel)
		if err != nil {
			http.Error(w, "查询模型绑定失败", http.StatusBadGateway)
			return
		}

		upstreamByChannel = make(map[int64]string, len(bindings))
		for _, b := range bindings {
			if strings.TrimSpace(b.UpstreamModel) == "" {
				continue
			}
			upstreamByChannel[b.ChannelID] = b.UpstreamModel
		}

		// 统一使用“渠道绑定模型”配置：无绑定即不可用（避免 legacy 字段导致的调度歧义）。
		if len(upstreamByChannel) == 0 {
			http.Error(w, "模型未配置可用上游", http.StatusBadGateway)
			return
		}

		cons.AllowChannelIDs = make(map[int64]struct{}, len(upstreamByChannel))
		for id := range upstreamByChannel {
			cons.AllowChannelIDs[id] = struct{}{}
		}

		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			if sel.PassThroughBodyEnabled {
				return rawBody, nil
			}
			up, ok := upstreamByChannel[sel.ChannelID]
			if !ok {
				return nil, errors.New("选中渠道未配置该模型")
			}
			out := clonePayload(payload)
			out["model"] = up
			applyChannelSystemPromptToResponsesPayload(out, sel)
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, err
			}
			raw, err = applyResponsesModelSuffixTransforms(raw, sel, publicModel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelRequestPolicy(raw, sel)
			if err != nil {
				return nil, err
			}
			raw, err = applyChannelBodyFilters(raw, sel)
			if err != nil {
				return nil, err
			}
			ctx := buildParamOverrideContext(sel, publicModel, up, r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	}

	allowGroups := p.Groups
	if len(allowGroups) == 0 {
		allowGroups = []string{"default"}
	}

	cons.AllowGroups = make(map[string]struct{}, len(allowGroups))
	for _, g := range allowGroups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		cons.AllowGroups[g] = struct{}{}
	}

	routeKey := extractRouteKeyFromPayload(payload)
	if routeKey == "" {
		routeKey = extractRouteKeyFromRawBody(rawBody)
	}
	if routeKey == "" {
		routeKey = extractRouteKey(r)
	}
	routeKeyHash := h.sched.RouteKeyHash(routeKey)
	usageID := int64(0)
	if h.quota != nil {
		res, err := h.quota.Reserve(r.Context(), quota.ReserveInput{
			RequestID:       middleware.GetRequestID(r.Context()),
			UserID:          p.UserID,
			TokenID:         *p.TokenID,
			Model:           optionalString(publicModel),
			MaxOutputTokens: maxOut,
		})
		if err != nil {
			if errors.Is(err, quota.ErrSubscriptionRequired) || errors.Is(err, quota.ErrQuotaExceeded) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			}
			if errors.Is(err, quota.ErrInsufficientBalance) {
				http.Error(w, err.Error(), http.StatusPaymentRequired)
				return
			}
			http.Error(w, "配额预留失败", http.StatusTooManyRequests)
			return
		}
		usageID = res.UsageEventID
	}
	reqBytes := int64(len(body))

	if h.groups == nil {
		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.quota.Void(bookCtx, usageID)
			cancel()
		}
		h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", 0)
		cw := &countingResponseWriter{ResponseWriter: w}
		http.Error(cw, "上游不可用", http.StatusBadGateway)
		h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream)
		h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream, reqBytes, cw.bytes)
		return
	}

	router := scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, routeKeyHash, cons)
	const absoluteMaxAttempts = 1000
	for i := 0; i < absoluteMaxAttempts; i++ {
		sel, err := router.Next(r.Context())
		if err != nil {
			break
		}
		rewritten, err := rewriteBody(sel)
		if err != nil {
			if usageID != 0 && h.quota != nil {
				bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = h.quota.Void(bookCtx, usageID)
				cancel()
			}
			cw := &countingResponseWriter{ResponseWriter: w}
			http.Error(cw, "请求体处理失败", http.StatusInternalServerError)
			h.finalizeUsageEvent(r, usageID, &sel, http.StatusInternalServerError, "rewrite_body", "请求体处理失败", time.Since(reqStart), stream, reqBytes, cw.bytes)
			return
		}
		if h.tryWithSelection(w, r, p, sel, rewritten, stream, optionalString(publicModel), usageID, reqStart, reqBytes, 1) {
			return
		}
	}

	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.quota.Void(bookCtx, usageID)
		cancel()
	}
	h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", 0)
	cw := &countingResponseWriter{ResponseWriter: w}
	http.Error(cw, "上游不可用", http.StatusBadGateway)
	h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream)
	h.finalizeUsageEvent(r, usageID, nil, http.StatusBadGateway, "upstream_unavailable", "上游不可用", time.Since(reqStart), stream, reqBytes, cw.bytes)
}

func (h *Handler) tryWithSelection(w http.ResponseWriter, r *http.Request, p auth.Principal, sel scheduler.Selection, body []byte, wantStream bool, model *string, usageID int64, reqStart time.Time, reqBytes int64, retries int) bool {
	for i := 0; i < retries; i++ {
		ok := h.proxyOnce(w, r, sel, body, wantStream, model, p, usageID, reqStart, reqBytes)
		if ok {
			return true
		}
		// 当下游已经开始写回（SSE/非流式）时，proxyOnce 会直接返回 true 并结束；这里仅处理“未写回的失败”。
	}
	return false
}

func (h *Handler) proxyOnce(w http.ResponseWriter, r *http.Request, sel scheduler.Selection, body []byte, wantStream bool, model *string, p auth.Principal, usageID int64, reqStart time.Time, reqBytes int64) bool {
	attemptStart := time.Now()

	resp, err := h.exec.Do(r.Context(), sel, r, body)
	if err != nil {
		h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "network"})
		h.auditFailover(r.Context(), r.URL.Path, p, &sel, model, 0, "network", time.Since(attemptStart))
		return false
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	// 失败分支：根据状态码决定是否 failover。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retriable := isRetriableStatus(resp.StatusCode)
		if retriable {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, StatusCode: resp.StatusCode, ErrorClass: "upstream_status"})
			h.auditFailover(r.Context(), r.URL.Path, p, &sel, model, resp.StatusCode, "upstream_status", time.Since(attemptStart))
			return false
		}
		if h.sched != nil {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: false, StatusCode: resp.StatusCode, ErrorClass: "upstream_status"})
		}
		h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, model, resp.StatusCode, "upstream_status", time.Since(attemptStart))
		cw := &countingResponseWriter{ResponseWriter: w}
		downstreamStatus := resetStatusCode(resp.StatusCode, sel.StatusCodeMapping)
		bodyBytes, _ := readLimited(resp.Body, 0)
		downstreamBody := bodyBytes
		filtered := false
		if sel.CredentialType == scheduler.CredentialTypeCodex {
			downstreamBody = sanitizeCodexErrorBody(bodyBytes)
			filtered = true
		}
		copyResponseHeaders(cw.Header(), resp.Header)
		if filtered {
			cw.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		cw.WriteHeader(downstreamStatus)
		n, _ := cw.Write(downstreamBody)
		respBytes := int64(n)

		// A 模式：仅过滤下游返回文案，内部记录仍保留上游原始错误信息用于排障。
		failMsg := summarizeUpstreamErrorBody(bodyBytes)
		logMsg := resp.Status
		if strings.TrimSpace(failMsg) != "" {
			logMsg = failMsg
		}
		h.maybeLogProxyFailure(r.Context(), r, p, &sel, model, resp.StatusCode, "upstream_status", logMsg, time.Since(attemptStart), wantStream || isSSE)
		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.quota.Void(bookCtx, usageID)
			cancel()
		}
		h.finalizeUsageEventWithUpstreamBodies(r, usageID, &sel, resp.StatusCode, "upstream_status", failMsg, time.Since(reqStart), wantStream || isSSE, reqBytes, respBytes, middleware.CachedBody(r.Context()), body, bodyBytes)
		return true
	}

	// SSE 分支：开始写回后禁止 failover。
	if wantStream || isSSE {
		type usageAcc struct {
			in        *int64
			out       *int64
			cachedIn  *int64
			cachedOut *int64
			seen      bool
		}
		var acc usageAcc

		cw := &countingResponseWriter{ResponseWriter: w}
		copyResponseHeaders(cw.Header(), resp.Header)
		cw.Header().Set("X-Accel-Buffering", "no")
		if isSSE {
			cw.Header().Set("Content-Type", "text/event-stream")
		}
		cw.WriteHeader(resp.StatusCode)

		hooks := upstream.SSEPumpHooks{
			OnData: func(data string) {
				// 避免对每个 delta 事件反复 JSON 解析：仅在疑似包含 usage 时尝试。
				if !strings.Contains(data, "usage") && !strings.Contains(data, "input_tokens") && !strings.Contains(data, "prompt_tokens") {
					return
				}
				var evt any
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					return
				}
				usage := findUsageMap(evt, 10)
				if usage == nil {
					return
				}
				inTok, outTok, cachedInTok, cachedOutTok := extractUsageTokensFromUsageMap(usage)
				if inTok != nil {
					acc.in = inTok
					acc.seen = true
				}
				if outTok != nil {
					acc.out = outTok
					acc.seen = true
				}
				if cachedInTok != nil {
					acc.cachedIn = cachedInTok
					acc.seen = true
				}
				if cachedOutTok != nil {
					acc.cachedOut = cachedOutTok
					acc.seen = true
				}
			},
		}
		if sel.ThinkingToContent {
			hooks.TransformData = newThinkingToContentTransformer()
		}

		pumpRes, _ := upstream.PumpSSE(r.Context(), cw, resp.Body, h.sseOpts, hooks)

		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.quota.Commit(bookCtx, quota.CommitInput{
				UsageEventID:       usageID,
				Model:              model,
				UpstreamChannelID:  &sel.ChannelID,
				InputTokens:        acc.in,
				CachedInputTokens:  acc.cachedIn,
				OutputTokens:       acc.out,
				CachedOutputTokens: acc.cachedOut,
			})
			cancel()
		}
		if h.sched != nil {
			total := int64(0)
			if acc.in != nil {
				total += *acc.in
			}
			if acc.out != nil {
				total += *acc.out
			}
			if acc.cachedIn != nil {
				total += *acc.cachedIn
			}
			if acc.cachedOut != nil {
				total += *acc.cachedOut
			}
			if total > 0 {
				h.sched.RecordTokens(sel.CredentialKey(), int(total))
			}
		}

		// SSE 已写回后不再 failover，但仍记录结果以用于后续调度权重。
		if pumpRes.ErrorClass == "" || pumpRes.ErrorClass == "client_disconnect" || pumpRes.ErrorClass == "stream_max_duration" {
			h.sched.Report(sel, scheduler.Result{Success: true})
		} else {
			retriable := pumpRes.ErrorClass == "stream_idle_timeout" || pumpRes.ErrorClass == "stream_read_error"
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: retriable, StatusCode: resp.StatusCode, ErrorClass: pumpRes.ErrorClass})
		}
		if pumpRes.ErrorClass != "" && pumpRes.ErrorClass != "client_disconnect" && pumpRes.ErrorClass != "stream_max_duration" {
			h.maybeLogProxyFailure(r.Context(), r, p, &sel, model, resp.StatusCode, pumpRes.ErrorClass, "", time.Since(attemptStart), true)
		}
		h.finalizeUsageEvent(r, usageID, &sel, resp.StatusCode, pumpRes.ErrorClass, "", time.Since(reqStart), true, reqBytes, cw.bytes)
		return true
	}

	// 非流式：完整转发并尝试提取 usage。
	bodyBytes, err := readLimited(resp.Body, 0)
	if err != nil {
		h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "read_upstream"})
		h.auditFailover(r.Context(), r.URL.Path, p, &sel, model, 0, "read_upstream", time.Since(attemptStart))
		return false
	}
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	respBytes, _ := w.Write(bodyBytes)

	inTok, outTok, cachedInTok, cachedOutTok := extractUsageTokens(bodyBytes)
	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.quota.Commit(bookCtx, quota.CommitInput{
			UsageEventID:       usageID,
			Model:              model,
			UpstreamChannelID:  &sel.ChannelID,
			InputTokens:        inTok,
			CachedInputTokens:  cachedInTok,
			OutputTokens:       outTok,
			CachedOutputTokens: cachedOutTok,
		})
		cancel()
	}
	if h.sched != nil {
		total := int64(0)
		if inTok != nil {
			total += *inTok
		}
		if outTok != nil {
			total += *outTok
		}
		if cachedInTok != nil {
			total += *cachedInTok
		}
		if cachedOutTok != nil {
			total += *cachedOutTok
		}
		if total > 0 {
			h.sched.RecordTokens(sel.CredentialKey(), int(total))
		}
	}
	h.sched.Report(sel, scheduler.Result{Success: true})
	h.finalizeUsageEvent(r, usageID, &sel, resp.StatusCode, "", "", time.Since(reqStart), false, reqBytes, int64(respBytes))
	return true
}

type countingResponseWriter struct {
	http.ResponseWriter
	bytes int64
}

func (w *countingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *countingResponseWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func (h *Handler) maybeLogProxyFailure(ctx context.Context, r *http.Request, p auth.Principal, sel *scheduler.Selection, model *string, status int, class string, msg string, latency time.Duration, stream bool) {
	if h.proxyLog == nil || !h.proxyLog.Enabled() {
		return
	}
	if r == nil {
		return
	}
	tokenID := int64(0)
	if p.TokenID != nil {
		tokenID = *p.TokenID
	}
	reqID := middleware.GetRequestID(ctx)
	path := r.URL.Path
	method := r.Method

	var channelID int64
	var channelType string
	var credType string
	var credID int64
	var baseURL string
	if sel != nil {
		channelID = sel.ChannelID
		channelType = sel.ChannelType
		credType = string(sel.CredentialType)
		credID = sel.CredentialID
		baseURL = sel.BaseURL
	}

	entry := proxylog.Entry{
		Time:      time.Now(),
		RequestID: reqID,
		Path:      path,
		Method:    method,
		UserID:    p.UserID,
		TokenID:   tokenID,
		Model:     model,
		Stream:    stream,

		ChannelID:       channelID,
		ChannelType:     channelType,
		CredentialType:  credType,
		CredentialID:    credID,
		UpstreamBaseURL: baseURL,

		StatusCode: status,
		ErrorClass: class,
		ErrorMsg:   strings.TrimSpace(msg),
		LatencyMS:  int(latency.Milliseconds()),
	}
	h.proxyLog.WriteFailure(ctx, entry)
}

func (h *Handler) finalizeUsageEvent(r *http.Request, usageID int64, sel *scheduler.Selection, status int, class string, msg string, latency time.Duration, stream bool, reqBytes, respBytes int64) {
	if usageID == 0 || h.usage == nil {
		return
	}

	bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep := r.URL.Path
	method := r.Method

	var upstreamChannelID *int64
	var upstreamEndpointID *int64
	var upstreamCredID *int64
	if sel != nil {
		if sel.ChannelID > 0 {
			id := sel.ChannelID
			upstreamChannelID = &id
		}
		if sel.EndpointID > 0 {
			id := sel.EndpointID
			upstreamEndpointID = &id
		}
		if sel.CredentialID > 0 {
			id := sel.CredentialID
			upstreamCredID = &id
		}
	}

	var classPtr *string
	if strings.TrimSpace(class) != "" {
		c := class
		classPtr = &c
	}

	if strings.TrimSpace(msg) == "" && status > 0 && (status < 200 || status >= 300) {
		msg = http.StatusText(status)
	}
	var msgPtr *string
	if strings.TrimSpace(msg) != "" {
		m := msg
		msgPtr = &m
	}

	_ = h.usage.FinalizeUsageEvent(bookCtx, store.FinalizeUsageEventInput{
		UsageEventID:       usageID,
		Endpoint:           ep,
		Method:             method,
		StatusCode:         status,
		LatencyMS:          int(latency.Milliseconds()),
		ErrorClass:         classPtr,
		ErrorMessage:       msgPtr,
		UpstreamChannelID:  upstreamChannelID,
		UpstreamEndpointID: upstreamEndpointID,
		UpstreamCredID:     upstreamCredID,
		IsStream:           stream,
		RequestBytes:       reqBytes,
		ResponseBytes:      respBytes,
	})
}

func (h *Handler) finalizeUsageEventWithUpstreamBodies(r *http.Request, usageID int64, sel *scheduler.Selection, status int, class string, msg string, latency time.Duration, stream bool, reqBytes, respBytes int64, downstreamReqBody []byte, upstreamReqBody []byte, upstreamRespBody []byte) {
	if usageID == 0 || h.usage == nil {
		return
	}

	bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep := r.URL.Path
	method := r.Method

	var upstreamChannelID *int64
	var upstreamEndpointID *int64
	var upstreamCredID *int64
	if sel != nil {
		if sel.ChannelID > 0 {
			id := sel.ChannelID
			upstreamChannelID = &id
		}
		if sel.EndpointID > 0 {
			id := sel.EndpointID
			upstreamEndpointID = &id
		}
		if sel.CredentialID > 0 {
			id := sel.CredentialID
			upstreamCredID = &id
		}
	}

	var classPtr *string
	if strings.TrimSpace(class) != "" {
		c := class
		classPtr = &c
	}

	if strings.TrimSpace(msg) == "" && status > 0 && (status < 200 || status >= 300) {
		msg = http.StatusText(status)
	}
	var msgPtr *string
	if strings.TrimSpace(msg) != "" {
		m := msg
		msgPtr = &m
	}

	var downPtr *string
	if len(downstreamReqBody) > 0 {
		s := string(downstreamReqBody)
		downPtr = &s
	}
	var reqPtr *string
	if len(upstreamReqBody) > 0 {
		s := string(upstreamReqBody)
		reqPtr = &s
	}
	var respPtr *string
	if len(upstreamRespBody) > 0 {
		s := string(upstreamRespBody)
		respPtr = &s
	}

	_ = h.usage.FinalizeUsageEvent(bookCtx, store.FinalizeUsageEventInput{
		UsageEventID:          usageID,
		Endpoint:              ep,
		Method:                method,
		StatusCode:            status,
		LatencyMS:             int(latency.Milliseconds()),
		ErrorClass:            classPtr,
		ErrorMessage:          msgPtr,
		UpstreamChannelID:     upstreamChannelID,
		UpstreamEndpointID:    upstreamEndpointID,
		UpstreamCredID:        upstreamCredID,
		IsStream:              stream,
		RequestBytes:          reqBytes,
		ResponseBytes:         respBytes,
		DownstreamRequestBody: downPtr,
		UpstreamRequestBody:   reqPtr,
		UpstreamResponseBody:  respPtr,
	})
}

func summarizeUpstreamErrorBody(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err == nil && len(parsed) > 0 {
		if msg, ok := parsed["detail"].(string); ok && strings.TrimSpace(msg) != "" {
			return trimSummary(msg)
		}
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

const codexSanitizedErrorMessage = "Codex upstream request failed. Please retry later."

func sanitizeCodexErrorBody(b []byte) []byte {
	sanitized := map[string]any{
		"error": map[string]any{
			"message": codexSanitizedErrorMessage,
			"type":    "upstream_error",
			"code":    "upstream_error",
		},
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err == nil && len(parsed) > 0 {
		if errObj, ok := parsed["error"].(map[string]any); ok {
			copyOptionalErrorField(errObj, sanitized["error"].(map[string]any), "type")
			copyOptionalErrorField(errObj, sanitized["error"].(map[string]any), "code")
			copyOptionalErrorField(errObj, sanitized["error"].(map[string]any), "param")
			copyOptionalErrorField(errObj, sanitized["error"].(map[string]any), "request_id")
		}
		copyOptionalErrorField(parsed, sanitized["error"].(map[string]any), "type")
		copyOptionalErrorField(parsed, sanitized["error"].(map[string]any), "code")
		copyOptionalErrorField(parsed, sanitized["error"].(map[string]any), "param")
		copyOptionalErrorField(parsed, sanitized["error"].(map[string]any), "request_id")
	}

	out, err := json.Marshal(sanitized)
	if err != nil {
		return []byte(`{"error":{"message":"Codex upstream request failed. Please retry later.","type":"upstream_error","code":"upstream_error"}}`)
	}
	return out
}

func copyOptionalErrorField(src map[string]any, dst map[string]any, key string) {
	if src == nil || dst == nil {
		return
	}
	v, ok := src[key]
	if !ok || v == nil {
		return
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return
	}
	dst[key] = v
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

func readLimited(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		return io.ReadAll(r)
	}
	var buf bytes.Buffer
	_, err := io.CopyN(&buf, r, max+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if int64(buf.Len()) > max {
		return nil, errors.New("响应体过大")
	}
	return buf.Bytes(), nil
}

func copyResponseHeaders(dst, src http.Header) {
	for k, vs := range src {
		kl := strings.ToLower(k)
		if kl == "content-length" || kl == "connection" || kl == "keep-alive" || kl == "transfer-encoding" || kl == "upgrade" {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func isRetriableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		// 常见为上游 base_url/path 不匹配或渠道能力缺失；切换 channel 有意义。
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

func resetStatusCode(status int, statusCodeMapping string) int {
	statusCodeMapping = strings.TrimSpace(statusCodeMapping)
	if statusCodeMapping == "" || statusCodeMapping == "{}" {
		return status
	}
	if status == http.StatusOK {
		return status
	}

	mapping := make(map[string]string)
	if err := json.Unmarshal([]byte(statusCodeMapping), &mapping); err != nil {
		return status
	}
	to, ok := mapping[strconv.Itoa(status)]
	if !ok {
		return status
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return status
	}
	v, err := strconv.Atoi(to)
	if err != nil || v <= 0 {
		return status
	}
	return v
}

func extractRouteKey(r *http.Request) string {
	keys := []string{
		"prompt_cache_key",
		"Prompt-Cache-Key",
		"X-Prompt-Cache-Key",
		"X-RC-Route-Key",
		"X-Session-Id",
		"session-id",
		"Conversation_id",
		"Conversation-Id",
		"conversation_id",
		"Session_id",
		"Session-Id",
		"session_id",
		"Idempotency-Key",
	}
	for _, k := range keys {
		if v := strings.TrimSpace(r.Header.Get(k)); v != "" {
			// 仅用于 hash（不落库/不打日志），限制长度避免异常输入拖慢请求。
			const maxLen = 1024
			if len(v) > maxLen {
				return ""
			}
			return v
		}
	}
	return ""
}

func extractRouteKeyFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{
		"prompt_cache_key",
		"session_id",
		"conversation_id",
		"previous_response_id",
	} {
		if s := normalizeRouteKey(payload[key]); s != "" {
			return s
		}
	}

	metaAny, ok := payload["metadata"]
	if !ok {
		return ""
	}
	meta, ok := metaAny.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{
		"prompt_cache_key",
		"session_id",
		"conversation_id",
	} {
		if s := normalizeRouteKey(meta[key]); s != "" {
			return s
		}
	}
	return ""
}

func extractRouteKeyFromRawBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return extractRouteKeyFromPayload(payload)
}

func normalizeRouteKey(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 仅用于 hash（不落库/不打日志），限制长度避免异常输入拖慢请求。
	const maxLen = 1024
	if len(s) > maxLen {
		return ""
	}
	return s
}

func (h *Handler) auditFailover(ctx context.Context, path string, p auth.Principal, sel *scheduler.Selection, model *string, status int, class string, latency time.Duration) {
	h.auditEvent(ctx, path, p, sel, model, "failover", status, class, latency)
}

func (h *Handler) auditUpstreamError(ctx context.Context, path string, p auth.Principal, sel *scheduler.Selection, model *string, status int, class string, latency time.Duration) {
	h.auditEvent(ctx, path, p, sel, model, "upstream_error", status, class, latency)
}

func (h *Handler) auditEvent(ctx context.Context, path string, p auth.Principal, sel *scheduler.Selection, model *string, action string, status int, class string, latency time.Duration) {
	if h.audit == nil {
		return
	}

	var userID *int64
	if p.UserID > 0 {
		uid := p.UserID
		userID = &uid
	}
	tokenID := p.TokenID

	var upstreamChannelID *int64
	var upstreamEndpointID *int64
	var upstreamCredID *int64
	if sel != nil {
		if sel.ChannelID > 0 {
			id := sel.ChannelID
			upstreamChannelID = &id
		}
		if sel.EndpointID > 0 {
			id := sel.EndpointID
			upstreamEndpointID = &id
		}
		if sel.CredentialID > 0 {
			id := sel.CredentialID
			upstreamCredID = &id
		}
	}

	var classPtr *string
	if strings.TrimSpace(class) != "" {
		c := class
		classPtr = &c
	}

	// 审计事件仅用于追溯路由行为，避免写入任何可能包含敏感信息的错误 body。
	msg := ""
	if status > 0 {
		msg = http.StatusText(status)
	}
	var msgPtr *string
	if strings.TrimSpace(msg) != "" {
		m := msg
		msgPtr = &m
	}

	_ = h.audit.InsertAuditEvent(ctx, store.AuditEventInput{
		RequestID:          middleware.GetRequestID(ctx),
		ActorType:          string(p.ActorType),
		UserID:             userID,
		TokenID:            tokenID,
		Action:             action,
		Endpoint:           path,
		Model:              model,
		UpstreamChannelID:  upstreamChannelID,
		UpstreamEndpointID: upstreamEndpointID,
		UpstreamCredID:     upstreamCredID,
		StatusCode:         status,
		LatencyMS:          int(latency.Milliseconds()),
		ErrorClass:         classPtr,
		ErrorMessage:       msgPtr,
	})
}

func boolFromAny(v any) bool {
	switch vv := v.(type) {
	case bool:
		return vv
	case string:
		return vv == "true" || vv == "1"
	default:
		return false
	}
}

func stringFromAny(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	default:
		return ""
	}
}

func optionalString(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	ss := s
	return &ss
}

func extractUsageTokens(body []byte) (*int64, *int64, *int64, *int64) {
	// 兼容常见两类：Responses usage.input_tokens/output_tokens；Chat usage.prompt_tokens/completion_tokens。
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, nil, nil
	}
	usage := findUsageMap(root, 10)
	if usage == nil {
		return nil, nil, nil, nil
	}
	in, out, cachedIn, cachedOut := extractUsageTokensFromUsageMap(usage)
	return in, out, cachedIn, cachedOut
}

func intFromAny(v any) *int64 {
	switch vv := v.(type) {
	case float64:
		n := int64(vv)
		return &n
	case int64:
		n := vv
		return &n
	case json.Number:
		if i, err := vv.Int64(); err == nil {
			return &i
		}
	case string:
		vv = strings.TrimSpace(vv)
		if vv == "" {
			return nil
		}
		if i, err := strconv.ParseInt(vv, 10, 64); err == nil {
			return &i
		}
	}
	return nil
}

func extractUsageTokensFromUsageMap(usage map[string]any) (*int64, *int64, *int64, *int64) {
	if usage == nil {
		return nil, nil, nil, nil
	}
	in := intFromAny(usage["input_tokens"])
	out := intFromAny(usage["output_tokens"])
	if in == nil && out == nil {
		in = intFromAny(usage["prompt_tokens"])
		out = intFromAny(usage["completion_tokens"])
	}

	// prompt caching：常见字段为 usage.{input_tokens_details|prompt_tokens_details}.cached_tokens。
	cachedIn := intFromAny(usage["cached_input_tokens"])
	if cachedIn == nil {
		if det, ok := usage["input_tokens_details"].(map[string]any); ok {
			cachedIn = intFromAny(det["cached_tokens"])
		}
	}
	if cachedIn == nil {
		if det, ok := usage["prompt_tokens_details"].(map[string]any); ok {
			cachedIn = intFromAny(det["cached_tokens"])
		}
	}
	if cachedIn == nil {
		// Anthropic Messages：usage.cache_read_input_tokens + usage.cache_creation_input_tokens。
		read := intFromAny(usage["cache_read_input_tokens"])
		create := intFromAny(usage["cache_creation_input_tokens"])
		if read != nil || create != nil {
			sum := int64(0)
			if read != nil {
				sum += *read
			}
			if create != nil {
				sum += *create
			}
			cachedIn = &sum
		}
	}

	// 目前上游很少返回 cached output，但为了兼容/扩展做同样的提取。
	cachedOut := intFromAny(usage["cached_output_tokens"])
	if cachedOut == nil {
		if det, ok := usage["output_tokens_details"].(map[string]any); ok {
			cachedOut = intFromAny(det["cached_tokens"])
		}
	}
	if cachedOut == nil {
		if det, ok := usage["completion_tokens_details"].(map[string]any); ok {
			cachedOut = intFromAny(det["cached_tokens"])
		}
	}

	return in, out, cachedIn, cachedOut
}

func findUsageMap(v any, depth int) map[string]any {
	if v == nil || depth <= 0 {
		return nil
	}
	switch vv := v.(type) {
	case map[string]any:
		if usageAny, ok := vv["usage"]; ok {
			if usage, ok := usageAny.(map[string]any); ok {
				return usage
			}
		}
		for _, child := range vv {
			if u := findUsageMap(child, depth-1); u != nil {
				return u
			}
		}
	case []any:
		for _, child := range vv {
			if u := findUsageMap(child, depth-1); u != nil {
				return u
			}
		}
	}
	return nil
}
