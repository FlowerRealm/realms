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
	"realms/internal/obs"
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
	FeatureStateEffective(ctx context.Context) store.FeatureState
}

type Handler struct {
	models ModelCatalog
	groups scheduler.ChannelGroupStore
	sched  *scheduler.Scheduler
	exec   Doer

	proxyLog *proxylog.Writer

	features FeatureResolver

	gateway           gatewayOptions
	gatewayConfigured bool
	concurrency       concurrencyManager
	errorPassthrough  errorPassthroughMatcher

	quota quota.Provider
	audit AuditSink
	usage UsageEventSink

	refs OpenAIObjectRefStore

	sseOpts upstream.SSEPumpOptions

	codexRouteCache *codexSessionRouteCache
	sessionBindings SessionBindingStore

	sub2api *upstream.Sub2APIClient
}

type Doer interface {
	Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)
}

type CodexCooldownSetter interface {
	SetCodexOAuthAccountCooldown(ctx context.Context, accountID int64, until time.Time) error
}

type CodexStatusSetter interface {
	SetCodexOAuthAccountStatus(ctx context.Context, accountID int64, status int) error
}

type CodexQuotaErrorSetter interface {
	SetCodexOAuthAccountQuotaError(ctx context.Context, accountID int64, msg *string) error
}

type CodexQuotaPatcher interface {
	PatchCodexOAuthAccountQuota(ctx context.Context, accountID int64, patch store.CodexOAuthQuotaPatch, updatedAt time.Time) error
}

type AuditSink interface {
	InsertAuditEvent(ctx context.Context, in store.AuditEventInput) error
}

type UsageEventSink interface {
	FinalizeUsageEvent(ctx context.Context, in store.FinalizeUsageEventInput) error
}

type SessionBindingStore interface {
	GetSessionBindingPayload(ctx context.Context, userID int64, routeKeyHash string, now time.Time) (string, bool, error)
	UpsertSessionBindingPayload(ctx context.Context, userID int64, routeKeyHash string, payload string, expiresAt time.Time) error
	DeleteSessionBinding(ctx context.Context, userID int64, routeKeyHash string) error
}

type OpenAIObjectRefStore interface {
	UpsertOpenAIObjectRef(ctx context.Context, ref store.OpenAIObjectRef) error
	GetOpenAIObjectRefForUser(ctx context.Context, userID int64, objectType string, objectID string) (store.OpenAIObjectRef, bool, error)
	ListOpenAIObjectRefsByUser(ctx context.Context, userID int64, objectType string, limit int) ([]store.OpenAIObjectRef, error)
	DeleteOpenAIObjectRef(ctx context.Context, objectType string, objectID string) error
}

func NewHandler(models ModelCatalog, groups scheduler.ChannelGroupStore, sched *scheduler.Scheduler, exec Doer, proxyLog *proxylog.Writer, features FeatureResolver, qp quota.Provider, audit AuditSink, usage UsageEventSink, refs OpenAIObjectRefStore, sseOpts upstream.SSEPumpOptions, sub2api *upstream.Sub2APIClient) *Handler {
	var sessionBindings SessionBindingStore
	for _, candidate := range []any{models, groups, features, audit, usage, refs} {
		if candidate == nil {
			continue
		}
		if sb, ok := candidate.(SessionBindingStore); ok && sb != nil {
			sessionBindings = sb
			break
		}
	}
	return &Handler{
		models:          models,
		groups:          groups,
		sched:           sched,
		exec:            exec,
		proxyLog:        proxyLog,
		features:        features,
		gateway:         defaultGatewayOptions(),
		quota:           qp,
		audit:           audit,
		usage:           usage,
		refs:            refs,
		sseOpts:         sseOpts,
		codexRouteCache: newCodexSessionRouteCache(),
		sessionBindings: sessionBindings,
		sub2api:         sub2api,
	}
}

func (h *Handler) Responses(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r)
}

func (h *Handler) patchCodexQuotaBestEffort(sel scheduler.Selection, headers http.Header) {
	if h == nil || h.exec == nil {
		return
	}
	if sel.CredentialType != scheduler.CredentialTypeCodex || sel.CredentialID <= 0 {
		return
	}
	patcher, ok := h.exec.(CodexQuotaPatcher)
	if !ok || patcher == nil {
		return
	}
	now := time.Now()
	patch, ok := codexQuotaPatchFromResponseHeaders(headers, now)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = patcher.PatchCodexOAuthAccountQuota(ctx, sel.CredentialID, patch, now)
	}()
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

	ags := allowGroupsFromPrincipal(p)
	allowSet := ags.Set
	if len(ags.Order) == 0 {
		http.Error(w, "Token 未配置渠道组", http.StatusBadRequest)
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
		groupName := managedModelGroupName(m)
		if allowSet != nil {
			if _, ok := allowSet[groupName]; !ok {
				continue
			}
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

func (h *Handler) finalizeClientDisconnect(r *http.Request, usageID int64, sel *scheduler.Selection, reqStart time.Time, stream bool, reqBytes int64) {
	if r == nil {
		return
	}
	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.quota.Void(bookCtx, usageID)
	}
	h.finalizeUsageEvent(r, usageID, sel, 0, "client_disconnect", "", time.Since(reqStart), 0, stream, reqBytes, 0)
}

func (h *Handler) finalizeIfCanceled(r *http.Request, usageID int64, sel *scheduler.Selection, reqStart time.Time, stream bool, reqBytes int64) bool {
	if r == nil {
		return false
	}
	if r.Context().Err() == nil {
		return false
	}
	h.finalizeClientDisconnect(r, usageID, sel, reqStart, stream, reqBytes)
	return true
}

func (h *Handler) voidQuotaBestEffort(usageID int64) {
	if h == nil || h.quota == nil || usageID == 0 {
		return
	}
	bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.quota.Void(bookCtx, usageID)
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
	completionKey := ""
	completionGenerated := false
	if changed, completedKey, generated := completeCodexSessionIdentifiers(payload, r, p); changed {
		normalizedBody, mErr := json.Marshal(payload)
		if mErr == nil {
			rawBody = normalizedBody
			body = normalizedBody
			completionKey = completedKey
			completionGenerated = generated
		}
	}

	rawBody, serviceTier, err := normalizeRequestServiceTier(rawBody, payload)
	if err != nil {
		http.Error(w, "service_tier 非法", http.StatusBadRequest)
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

	freeMode := false
	modelPassthrough := false
	if h.features != nil {
		fs := h.features.FeatureStateEffective(r.Context())
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
	if r != nil && r.URL != nil && r.URL.Path != "/v1/responses" {
		cons.RequireChannelType = store.UpstreamTypeOpenAICompatible
	}

	ags := allowGroupsFromPrincipal(p)
	allowSet := ags.Set
	if len(ags.Order) == 0 {
		http.Error(w, "Token 未配置渠道组", http.StatusBadRequest)
		return
	}
	cons.AllowGroups = allowSet
	cons.AllowGroupOrder = ags.Order
	cons.SequentialChannelFailover = true

	var rewriteBody func(sel scheduler.Selection) ([]byte, error)

	var bindings []store.ChannelModelBinding
	var upstreamByChannel map[int64]string

	if modelPassthrough {
		// 非 free_mode 下仍要求模型定价存在（用于配额预留与计费口径），但不要求“启用”。
		if !freeMode || isPriorityServiceTier(serviceTier) {
			mm, err := h.models.GetManagedModelByPublicID(r.Context(), publicModel)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, "模型不存在", http.StatusBadRequest)
					return
				}
				http.Error(w, "查询模型失败", http.StatusBadGateway)
				return
			}
			groupName := managedModelGroupName(mm)
			if allowSet != nil {
				if _, ok := allowSet[groupName]; !ok {
					http.Error(w, "无权限使用该模型", http.StatusBadRequest)
					return
				}
			}
			if err := validateManagedModelServiceTier(mm, serviceTier); err != nil {
				http.Error(w, serviceTierBadRequestMessage(err), http.StatusBadRequest)
				return
			}
		}
		// passthrough 模式下仍尝试使用“渠道绑定模型”做 model 转发（best-effort）；
		// 但不强制要求存在绑定（无绑定时直接透传 model）。
		if bindings, err := h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), publicModel); err == nil {
			requireType := strings.TrimSpace(cons.RequireChannelType)
			m := make(map[int64]string, len(bindings))
			for _, b := range bindings {
				if requireType != "" && strings.TrimSpace(b.ChannelType) != requireType {
					continue
				}
				if strings.TrimSpace(b.UpstreamModel) == "" {
					continue
				}
				m[b.ChannelID] = b.UpstreamModel
			}
			if len(m) > 0 {
				upstreamByChannel = m
				cons.AllowChannelIDs = make(map[int64]struct{}, len(upstreamByChannel))
				for id := range upstreamByChannel {
					cons.AllowChannelIDs[id] = struct{}{}
				}
			}
		}
		rewriteBody = func(sel scheduler.Selection) ([]byte, error) {
			if sel.PassThroughBodyEnabled {
				return rawBody, nil
			}
			upstreamModel := publicModel
			if upstreamByChannel != nil {
				if up, ok := upstreamByChannel[sel.ChannelID]; ok && strings.TrimSpace(up) != "" {
					upstreamModel = up
				}
			}
			out := clonePayload(payload)
			out["model"] = upstreamModel
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
			ctx := buildParamOverrideContext(sel, publicModel, upstreamModel, r.URL.Path)
			raw, err = applyChannelParamOverride(raw, sel, ctx)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	} else {
		mm, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), publicModel)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "模型未启用", http.StatusBadRequest)
				return
			}
			http.Error(w, "查询模型失败", http.StatusBadGateway)
			return
		}
		groupName := managedModelGroupName(mm)
		if allowSet != nil {
			if _, ok := allowSet[groupName]; !ok {
				http.Error(w, "无权限使用该模型", http.StatusBadRequest)
				return
			}
		}
		if err := validateManagedModelServiceTier(mm, serviceTier); err != nil {
			http.Error(w, serviceTierBadRequestMessage(err), http.StatusBadRequest)
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

	applyServiceTierConstraints(&cons, serviceTier)

	routeKey := extractRouteKeyFromPayload(payload)
	routeKeySource := "payload"
	if routeKey == "" {
		routeKeySource = "unknown"
	}
	if routeKey == "" {
		routeKey = extractRouteKeyFromRawBody(rawBody)
		if routeKey != "" {
			routeKeySource = "raw_body"
		}
	}
	if routeKey == "" {
		routeKey = extractRouteKey(r)
		if routeKey != "" {
			routeKeySource = "header"
		}
	}
	if routeKey != "" && completionGenerated && completionKey != "" && routeKey == completionKey {
		routeKeySource = "generated"
	}
	if routeKey == "" {
		routeKey = normalizeRouteKey(deriveRouteKeyFromConversationPayload(payload))
		if routeKey != "" {
			routeKeySource = "derived"
		}
	}
	if routeKey == "" {
		routeKeySource = "missing"
	}
	w.Header().Set("X-Realms-Route-Key-Source", routeKeySource)
	routeKeyHash := h.sched.RouteKeyHash(routeKey)
	// Codex CLI（wire_api=responses）通常使用 input 数组并依赖 prompt_cache_key/session_id 做远程压缩（compaction）。
	// 这类“有状态输入”要求记住当前会话已经转移到哪个上游 channel，
	// 否则 encrypted_content 等续链状态可能无法复用。
	//
	// 为了避免影响普通 OpenAI SDK（input 为 string / 非 codex 形态）请求的负载均衡策略，这里仅对“显式会话键”的请求
	// 启用 routeKeyHash 参与调度（例如 Codex 自带 session_id/prompt_cache_key）。
	stickyRouteKeyHash := ""
	if shouldEnableStickyRouting(payload, r, routeKeySource) {
		stickyRouteKeyHash = routeKeyHash
	}
	if stickyRouteKeyHash != "" {
		r = r.WithContext(withCodexStickyRouteKeyHash(r.Context(), stickyRouteKeyHash))
	}

	boundRoute, boundOK := h.loadCodexStickyBinding(r.Context(), p.UserID, stickyRouteKeyHash, time.Now())
	bindingActive := false
	bindingMovedHeaderSet := false
	bindingCredentialPinned := false
	if stickyRouteKeyHash != "" && boundOK && boundRoute.channelID > 0 {
		bindingActive = true
		cons.StartChannelID = boundRoute.channelID
		if strings.TrimSpace(boundRoute.credentialKey) != "" {
			bindingCredentialPinned = true
			cons.RequireChannelID = boundRoute.channelID
			cons.RequireCredentialKey = boundRoute.credentialKey
		}
	}

	usageID := int64(0)
	userRelease, userSlotErr := h.acquireUserSlot(r.Context(), p.UserID)
	if userSlotErr != nil {
		fail := classifyConcurrencyAcquireFailure(userSlotErr)
		resp := h.buildFailoverExhaustedResponse("openai", fail)
		cw := &countingResponseWriter{ResponseWriter: w}
		writeOpenAIErrorWithRetryAfter(cw, resp.Status, resp.ErrType, resp.Message, resp.RetryAfterSeconds)
		if !resp.SkipMonitoring {
			h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, resp.Message, time.Since(reqStart), stream)
		}
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if h.quota != nil && r != nil && r.URL != nil && r.URL.Path == "/v1/responses" {
		res, err := h.quota.Reserve(r.Context(), quota.ReserveInput{
			RequestID:       middleware.GetRequestID(r.Context()),
			UserID:          p.UserID,
			TokenID:         *p.TokenID,
			Model:           optionalString(publicModel),
			ServiceTier:     serviceTier,
			MaxOutputTokens: maxOut,
		})
		if err != nil {
			if msg := reserveBadRequestMessage(err); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
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
		h.voidQuotaBestEffort(usageID)
		resp := h.buildFailoverExhaustedResponse("openai", proxyFailureInfo{})
		if !resp.SkipMonitoring {
			h.auditUpstreamError(r.Context(), r.URL.Path, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, 0)
		}
		cw := &countingResponseWriter{ResponseWriter: w}
		writeOpenAIErrorWithRetryAfter(cw, resp.Status, resp.ErrType, resp.Message, resp.RetryAfterSeconds)
		if !resp.SkipMonitoring {
			h.maybeLogProxyFailure(r.Context(), r, p, nil, optionalString(publicModel), resp.Status, resp.ErrorClass, resp.Message, time.Since(reqStart), stream)
		}
		finalClass := resp.ErrorClass
		if resp.SkipMonitoring {
			finalClass = ""
		}
		h.finalizeUsageEvent(r, usageID, nil, resp.Status, finalClass, resp.UsageMessage, time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
		return
	}

	router := scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, stickyRouteKeyHash, cons)
	var lastSel *scheduler.Selection
	bestFailure := proxyFailureInfo{}
	loopStart := time.Now()
	switches := 0
	backoff := h.initialBackoff()
	for {
		if h.failoverExhausted(loopStart, switches) {
			break
		}
		sel, err := router.Next(r.Context())
		if err != nil {
			if h.finalizeIfCanceled(r, usageID, nil, reqStart, stream, reqBytes) {
				return
			}
			if msg := serviceTierSelectionBadRequestMessage(err); msg != "" {
				h.voidQuotaBestEffort(usageID)
				http.Error(w, msg, http.StatusBadRequest)
				h.finalizeUsageEvent(r, usageID, nil, http.StatusBadRequest, "service_tier", msg, time.Since(reqStart), 0, stream, reqBytes, 0)
				return
			}
			if bindingCredentialPinned {
				bindingCredentialPinned = false
				cons.RequireChannelID = 0
				cons.RequireCredentialKey = ""
				cons.StartChannelID = boundRoute.channelID
				router = scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, stickyRouteKeyHash, cons)
				continue
			}
			break
		}
		selCopy := sel
		lastSel = &selCopy
		if bindingActive && !bindingMovedHeaderSet && boundRoute.channelID > 0 && sel.ChannelID != boundRoute.channelID {
			bindingMovedHeaderSet = true
			w.Header().Set("X-Realms-Codex-Sticky-Cleared", "1")
			w.Header().Set("X-Realms-Codex-Prev-Channel", strconv.FormatInt(boundRoute.channelID, 10))
			if strings.TrimSpace(boundRoute.credentialKey) != "" {
				w.Header().Set("X-Realms-Codex-Prev-Credential", boundRoute.credentialKey)
			}
		}
		rewritten, err := rewriteBody(sel)
		if err != nil {
			h.voidQuotaBestEffort(usageID)
			if msg := serviceTierSelectionBadRequestMessage(err); msg != "" {
				cw := &countingResponseWriter{ResponseWriter: w}
				http.Error(cw, msg, http.StatusBadRequest)
				h.finalizeUsageEvent(r, usageID, &sel, http.StatusBadRequest, "service_tier", msg, time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
				return
			}
			cw := &countingResponseWriter{ResponseWriter: w}
			http.Error(cw, "请求体处理失败", http.StatusInternalServerError)
			h.finalizeUsageEvent(r, usageID, &sel, http.StatusInternalServerError, "rewrite_body", "请求体处理失败", time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
			return
		}

		if h.tryWithSelection(w, r, p, sel, rewritten, stream, optionalString(publicModel), usageID, reqStart, reqBytes, loopStart, 2, &bestFailure) {
			return
		}
		if bindingCredentialPinned {
			bindingCredentialPinned = false
			cons.RequireChannelID = 0
			cons.RequireCredentialKey = ""
			cons.StartChannelID = boundRoute.channelID
			router = scheduler.NewGroupRouter(h.groups, h.sched, p.UserID, stickyRouteKeyHash, cons)
			continue
		}
		switches++
		if h.failoverExhausted(loopStart, switches) {
			break
		}
		if !h.waitBackoffWithinRetryElapsed(r.Context(), loopStart, backoff) {
			if h.finalizeIfCanceled(r, usageID, lastSel, reqStart, stream, reqBytes) {
				return
			}
			break
		}
		backoff = h.nextBackoff(backoff)
	}

	h.voidQuotaBestEffort(usageID)
	resp := h.buildFailoverExhaustedResponse("openai", bestFailure)
	if !resp.SkipMonitoring {
		h.auditUpstreamError(r.Context(), r.URL.Path, p, lastSel, optionalString(publicModel), resp.Status, resp.ErrorClass, 0)
	}
	cw := &countingResponseWriter{ResponseWriter: w}
	writeOpenAIErrorWithRetryAfter(cw, resp.Status, resp.ErrType, resp.Message, resp.RetryAfterSeconds)
	if !resp.SkipMonitoring {
		h.maybeLogProxyFailure(r.Context(), r, p, lastSel, optionalString(publicModel), resp.Status, resp.ErrorClass, resp.Message, time.Since(reqStart), stream)
	}
	finalClass := resp.ErrorClass
	if resp.SkipMonitoring {
		finalClass = ""
	}
	h.finalizeUsageEvent(r, usageID, lastSel, resp.Status, finalClass, resp.UsageMessage, time.Since(reqStart), 0, stream, reqBytes, cw.bytes)
}

type proxyAttemptDecision int

const (
	proxyAttemptDone proxyAttemptDecision = iota
	proxyAttemptRetrySameSelection
	proxyAttemptFailover
)

type proxyFailureInfo struct {
	Valid      bool
	Class      string
	StatusCode int
	Message    string
	Body       []byte
}

func (fi proxyFailureInfo) score() int {
	if !fi.Valid || strings.TrimSpace(fi.Message) == "" {
		return 0
	}
	if fi.StatusCode > 0 && strings.HasPrefix(strings.TrimSpace(fi.Class), "upstream_") {
		return 3
	}
	if fi.StatusCode > 0 {
		return 2
	}
	return 1
}

func recordProxyFailure(best *proxyFailureInfo, next proxyFailureInfo) {
	if best == nil || !next.Valid || strings.TrimSpace(next.Message) == "" {
		return
	}
	if len(next.Body) > 0 {
		next.Body = cloneProxyFailureBody(next.Body)
	}
	if !best.Valid || next.score() > best.score() || next.score() == best.score() {
		*best = next
	}
}

func cloneProxyFailureBody(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	if len(body) > failoverErrorBodyMaxBytes {
		body = body[:failoverErrorBodyMaxBytes]
	}
	return append([]byte(nil), body...)
}

func formatProxyFailureDetail(fi proxyFailureInfo) string {
	parts := make([]string, 0, 3)
	if cls := strings.TrimSpace(fi.Class); cls != "" {
		parts = append(parts, cls)
	}
	if fi.StatusCode > 0 {
		parts = append(parts, strconv.Itoa(fi.StatusCode))
	}
	if msg := strings.TrimSpace(fi.Message); msg != "" {
		parts = append(parts, msg)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func upstreamUnavailableUsageMessage(best proxyFailureInfo) string {
	if !best.Valid || strings.TrimSpace(best.Message) == "" {
		return "上游不可用"
	}
	detail := formatProxyFailureDetail(best)
	if detail == "" {
		return "上游不可用"
	}
	return "上游不可用；最后一次失败: " + detail
}

func (h *Handler) tryWithSelection(w http.ResponseWriter, r *http.Request, p auth.Principal, sel scheduler.Selection, body []byte, wantStream bool, model *string, usageID int64, reqStart time.Time, reqBytes int64, loopStart time.Time, retries int, bestFailure *proxyFailureInfo) bool {
	retries = h.sameSelectionRetries(retries)
	backoff := h.initialBackoff()
	for i := 0; i < retries; i++ {
		releaseCred, err := h.acquireCredentialSlot(r.Context(), sel)
		if err != nil {
			if h.finalizeIfCanceled(r, usageID, &sel, reqStart, wantStream, reqBytes) {
				return true
			}
			recordProxyFailure(bestFailure, classifyConcurrencyAcquireFailure(err))
			return false
		}
		decision, failure := h.proxyOnce(w, r, sel, body, wantStream, model, p, usageID, reqStart, reqBytes)
		if releaseCred != nil {
			releaseCred()
		}
		if failure.Valid {
			recordProxyFailure(bestFailure, failure)
		}
		switch decision {
		case proxyAttemptDone:
			return true
		case proxyAttemptRetrySameSelection:
			if i+1 < retries {
				if !h.waitBackoffWithinRetryElapsed(r.Context(), loopStart, backoff) {
					if h.finalizeIfCanceled(r, usageID, &sel, reqStart, wantStream, reqBytes) {
						return true
					}
					return false
				}
				backoff = h.nextBackoff(backoff)
				continue
			}
			return false
		case proxyAttemptFailover:
			return false
		default:
			return false
		}
		// 当下游已经开始写回（SSE/非流式）时，proxyOnce 会返回 proxyAttemptDone；这里仅处理“未写回的失败”。
	}
	return false
}

func (h *Handler) proxyOnce(w http.ResponseWriter, r *http.Request, sel scheduler.Selection, body []byte, wantStream bool, model *string, p auth.Principal, usageID int64, reqStart time.Time, reqBytes int64) (proxyAttemptDecision, proxyFailureInfo) {
	attemptStart := time.Now()
	serviceTier := requestedServiceTierFromJSONBytes(body)
	var resp *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		resp, err = h.exec.Do(r.Context(), sel, r, body)
		if err != nil {
			if h.finalizeIfCanceled(r, usageID, &sel, reqStart, wantStream, reqBytes) {
				return proxyAttemptDone, proxyFailureInfo{}
			}
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "network"})
			h.auditFailover(r.Context(), r.URL.Path, p, &sel, model, 0, "network", time.Since(attemptStart))
			return proxyAttemptRetrySameSelection, proxyFailureInfo{
				Valid:   true,
				Class:   "network",
				Message: trimSummary(err.Error()),
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			contentType := resp.Header.Get("Content-Type")
			isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

			bodyBytes := readPrefixBestEffort(resp.Body, upstreamErrorBodyMaxBytes)
			if attempt == 0 && isInvalidEncryptedContentUpstreamError(bodyBytes) {
				resetRes, rErr := resetCodexStatefulContinuation(body)
				if rErr == nil && resetRes.changed {
					_ = resp.Body.Close()
					h.voidQuotaBestEffort(usageID)
					h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, model, http.StatusConflict, "session_reset_required", time.Since(attemptStart))
					cw := &countingResponseWriter{ResponseWriter: w}
					writeOpenAIError(cw, http.StatusConflict, "session_reset_required", "上游返回续链错误（invalid encrypted_content）：该会话的 compaction 上下文无法复用。为避免静默丢上下文，服务端不会自动重置续链；请重启会话/新开对话后重试。")
					h.maybeLogProxyFailure(r.Context(), r, p, &sel, model, http.StatusConflict, "session_reset_required", "session_reset_required", time.Since(attemptStart), wantStream || isSSE)
					h.finalizeUsageEvent(r, usageID, &sel, http.StatusConflict, "session_reset_required", "session_reset_required", time.Since(reqStart), 0, wantStream || isSSE, reqBytes, cw.bytes)
					return proxyAttemptDone, proxyFailureInfo{}
				}
			}
			_ = resp.Body.Close()

			codexErr := classifyCodexOAuthUpstreamError(sel, resp.StatusCode, bodyBytes)
			retriable := isRetriableStatus(resp.StatusCode) || codexErr.retriable()
			errorClass := "upstream_status"
			var cooldownUntil *time.Time
			if codexErr.Kind != codexOAuthErrNone {
				if cls := codexErr.errorClass(); cls != "" {
					errorClass = cls
				}
				if codexErr.DisableAccount {
					h.setCodexCredentialDisabled(r.Context(), sel)
				}
				if codexErr.MarkBalanceDepleted {
					h.setCodexCredentialQuotaError(r.Context(), sel, "余额用尽")
				}
				if until := codexErr.cooldownUntil(time.Now(), bodyBytes); until != nil {
					cooldownUntil = until
					h.setCodexCredentialCooldown(r.Context(), sel, *until)
				}
			}
			if retriable {
				failMsg := summarizeUpstreamErrorBody(bodyBytes)
				if strings.TrimSpace(failMsg) == "" {
					failMsg = strings.TrimSpace(resp.Status)
				}
				h.sched.Report(sel, scheduler.Result{
					Success:       false,
					Retriable:     true,
					StatusCode:    resp.StatusCode,
					ErrorClass:    errorClass,
					CooldownUntil: cooldownUntil,
				})
				h.auditFailover(r.Context(), r.URL.Path, p, &sel, model, resp.StatusCode, errorClass, time.Since(attemptStart))
				return proxyAttemptFailover, proxyFailureInfo{
					Valid:      true,
					Class:      errorClass,
					StatusCode: resp.StatusCode,
					Message:    failMsg,
					Body:       bodyBytes,
				}
			}

			if h.sched != nil {
				h.sched.Report(sel, scheduler.Result{Success: false, Retriable: false, StatusCode: resp.StatusCode, ErrorClass: "upstream_status"})
			}
			h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, model, resp.StatusCode, "upstream_status", time.Since(attemptStart))
			cw := &countingResponseWriter{ResponseWriter: w}
			downstreamStatus := resetStatusCode(resp.StatusCode, sel.StatusCodeMapping)
			copyResponseHeaders(cw.Header(), resp.Header)
			cw.WriteHeader(downstreamStatus)
			n, _ := cw.Write(bodyBytes)
			respBytes := int64(n)

			failMsg := summarizeUpstreamErrorBody(bodyBytes)
			logMsg := resp.Status
			if strings.TrimSpace(failMsg) != "" {
				logMsg = failMsg
			}
			h.maybeLogProxyFailure(r.Context(), r, p, &sel, model, resp.StatusCode, "upstream_status", logMsg, time.Since(attemptStart), wantStream || isSSE)
			if usageID != 0 && h.quota != nil {
				bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = h.quota.Void(bookCtx, usageID)
			}
			h.finalizeUsageEvent(r, usageID, &sel, resp.StatusCode, "upstream_status", failMsg, time.Since(reqStart), 0, wantStream || isSSE, reqBytes, respBytes)
			return proxyAttemptDone, proxyFailureInfo{}
		}
		break
	}

	if resp == nil {
		return proxyAttemptDone, proxyFailureInfo{}
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	recordCreatedObjectType := ""
	recordCreatedObject := false
	if r != nil && r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/v1/responses":
			recordCreatedObjectType = openAIObjectTypeResponse
			recordCreatedObject = true
		case "/v1/chat/completions":
			if chatCompletionRequestStoresObject(body) {
				recordCreatedObjectType = openAIObjectTypeChatCompletion
				recordCreatedObject = true
			}
		}
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
		var (
			acc              usageAcc
			responseRouteKey string
			createdObjectID  string
		)

		cw := &countingResponseWriter{ResponseWriter: w}
		h.patchCodexQuotaBestEffort(sel, resp.Header)
		copyResponseHeaders(cw.Header(), resp.Header)
		cw.Header().Set("X-Accel-Buffering", "no")
		if isSSE {
			cw.Header().Set("Content-Type", "text/event-stream")
		}
		cw.WriteHeader(resp.StatusCode)

		firstTokenLatencyMS := 0
		hooks := upstream.SSEPumpHooks{
			OnData: func(data string) {
				if recordCreatedObject && createdObjectID == "" {
					switch recordCreatedObjectType {
					case openAIObjectTypeResponse:
						if id := extractResponseIDFromJSONBytes([]byte(data)); id != "" {
							createdObjectID = id
							h.recordOpenAIObjectRef(r.Context(), recordCreatedObjectType, createdObjectID, p, sel)
						}
					case openAIObjectTypeChatCompletion:
						if id := extractChatCompletionIDFromJSONBytes([]byte(data)); id != "" {
							createdObjectID = id
							h.recordOpenAIObjectRef(r.Context(), recordCreatedObjectType, createdObjectID, p, sel)
						}
					default:
					}
				}
				if firstTokenLatencyMS <= 0 {
					firstTokenLatencyMS = int(time.Since(reqStart).Milliseconds())
					if firstTokenLatencyMS < 0 {
						firstTokenLatencyMS = 0
					}
				}
				// 避免对每个 delta 事件反复 JSON 解析：仅在疑似包含 usage 时尝试。
				if !strings.Contains(data, "usage") && !strings.Contains(data, "input_tokens") && !strings.Contains(data, "prompt_tokens") {
					if responseRouteKey == "" {
						var evt any
						if err := json.Unmarshal([]byte(data), &evt); err == nil {
							responseRouteKey = extractRouteKeyFromStructuredData(evt)
						}
					}
					return
				}
				var evt any
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					return
				}
				if responseRouteKey == "" {
					responseRouteKey = extractRouteKeyFromStructuredData(evt)
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

		doneSSE := obs.TrackSSEConnection()
		defer doneSSE()
		pumpRes, _ := upstream.PumpSSE(r.Context(), cw, resp.Body, h.sseOpts, hooks)
		obs.RecordSSEFirstWriteLatency(pumpRes.FirstWriteLatency)
		obs.RecordSSEBytesStreamed(pumpRes.BytesWritten)
		obs.RecordSSEPumpResult(pumpRes.ErrorClass, pumpRes.SawDone)

		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.quota.Commit(bookCtx, quota.CommitInput{
				UsageEventID:       usageID,
				Model:              model,
				ServiceTier:        serviceTier,
				UpstreamChannelID:  &sel.ChannelID,
				RouteGroup:         optionalString(sel.RouteGroup),
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
			h.rememberCodexLastSuccessRoute(r, sel)
		} else {
			retriable := pumpRes.ErrorClass == "stream_idle_timeout" || pumpRes.ErrorClass == "stream_read_error"
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: retriable, StatusCode: resp.StatusCode, ErrorClass: pumpRes.ErrorClass})
		}
		if pumpRes.ErrorClass != "" && pumpRes.ErrorClass != "client_disconnect" && pumpRes.ErrorClass != "stream_max_duration" {
			h.maybeLogProxyFailure(r.Context(), r, p, &sel, model, resp.StatusCode, pumpRes.ErrorClass, "", time.Since(attemptStart), true)
		}
		h.finalizeUsageEvent(r, usageID, &sel, resp.StatusCode, pumpRes.ErrorClass, "", time.Since(reqStart), firstTokenLatencyMS, true, reqBytes, cw.bytes)
		return proxyAttemptDone, proxyFailureInfo{}
	}

	// 非流式：流式转发以避免超大响应导致 OOM，同时仅缓冲有限前缀用于提取 usage。
	var capBuf limitedPrefixBuffer
	capBuf.maxBytes = upstreamNonStreamExtractMaxBytes

	cw := &countingResponseWriter{ResponseWriter: w}
	h.patchCodexQuotaBestEffort(sel, resp.Header)
	copyResponseHeaders(cw.Header(), resp.Header)
	cw.WriteHeader(resp.StatusCode)

	_, copyErr := io.Copy(cw, io.TeeReader(resp.Body, &capBuf))
	respBytes := cw.bytes
	if copyErr != nil {
		if h.sched != nil {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: false, StatusCode: resp.StatusCode, ErrorClass: "proxy_copy"})
		}
		if usageID != 0 && h.quota != nil {
			bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.quota.Void(bookCtx, usageID)
		}
		h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, model, resp.StatusCode, "proxy_copy", time.Since(attemptStart))
		h.maybeLogProxyFailure(r.Context(), r, p, &sel, model, resp.StatusCode, "proxy_copy", copyErr.Error(), time.Since(attemptStart), false)
		h.finalizeUsageEvent(r, usageID, &sel, resp.StatusCode, "proxy_copy", "", time.Since(reqStart), 0, false, reqBytes, respBytes)
		return proxyAttemptDone, proxyFailureInfo{}
	}

	var (
		inTok, outTok, cachedInTok, cachedOutTok *int64
	)
	if !capBuf.exceeded {
		bodyBytes := capBuf.buf.Bytes()
		if recordCreatedObject {
			switch recordCreatedObjectType {
			case openAIObjectTypeResponse:
				if id := extractResponseIDFromJSONBytes(bodyBytes); id != "" {
					h.recordOpenAIObjectRef(r.Context(), recordCreatedObjectType, id, p, sel)
				}
			case openAIObjectTypeChatCompletion:
				if id := extractChatCompletionIDFromJSONBytes(bodyBytes); id != "" {
					h.recordOpenAIObjectRef(r.Context(), recordCreatedObjectType, id, p, sel)
				}
			default:
			}
		}
		inTok, outTok, cachedInTok, cachedOutTok = extractUsageTokens(bodyBytes)
	}

	if usageID != 0 && h.quota != nil {
		bookCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.quota.Commit(bookCtx, quota.CommitInput{
			UsageEventID:       usageID,
			Model:              model,
			ServiceTier:        serviceTier,
			UpstreamChannelID:  &sel.ChannelID,
			RouteGroup:         optionalString(sel.RouteGroup),
			InputTokens:        inTok,
			CachedInputTokens:  cachedInTok,
			OutputTokens:       outTok,
			CachedOutputTokens: cachedOutTok,
		})
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
		h.sched.Report(sel, scheduler.Result{Success: true})
	}
	h.rememberCodexLastSuccessRoute(r, sel)
	h.finalizeUsageEvent(r, usageID, &sel, resp.StatusCode, "", "", time.Since(reqStart), 0, false, reqBytes, respBytes)
	return proxyAttemptDone, proxyFailureInfo{}
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

func (h *Handler) finalizeUsageEvent(r *http.Request, usageID int64, sel *scheduler.Selection, status int, class string, msg string, latency time.Duration, firstTokenLatencyMS int, stream bool, reqBytes, respBytes int64) {
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
		UsageEventID:        usageID,
		Endpoint:            ep,
		Method:              method,
		StatusCode:          status,
		LatencyMS:           int(latency.Milliseconds()),
		FirstTokenLatencyMS: firstTokenLatencyMS,
		ErrorClass:          classPtr,
		ErrorMessage:        msgPtr,
		UpstreamChannelID:   upstreamChannelID,
		UpstreamEndpointID:  upstreamEndpointID,
		UpstreamCredID:      upstreamCredID,
		IsStream:            stream,
		RequestBytes:        reqBytes,
		ResponseBytes:       respBytes,
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

func isCodexUsageLimitReached(sel scheduler.Selection, statusCode int, body []byte) bool {
	if sel.CredentialType != scheduler.CredentialTypeCodex || len(body) == 0 {
		return false
	}
	// 对齐参考项目（openaiRoutes.js）：
	// - 非流式以 429 作为限流/耗尽信号
	// - 流式/结构化错误以 error.type=usage_limit_reached 识别
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	code, typ := extractUpstreamErrorCodeAndType(body)
	if strings.EqualFold(strings.TrimSpace(typ), "usage_limit_reached") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(code), "usage_limit_reached")
}

func codexUsageLimitCooldownUntil(now time.Time, body []byte) time.Time {
	if resetAt := parseCodexUsageLimitResetAt(now, body); resetAt != nil && resetAt.After(now) {
		return *resetAt
	}
	return now.Add(5 * time.Minute)
}

func codexRateLimitCooldownUntil(now time.Time, body []byte) time.Time {
	if resetAt := parseCodexUsageLimitResetAt(now, body); resetAt != nil && resetAt.After(now) {
		return *resetAt
	}
	return now.Add(30 * time.Second)
}

func parseCodexUsageLimitResetAt(now time.Time, body []byte) *time.Time {
	if len(body) == 0 {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	errObj, ok := parsed["error"].(map[string]any)
	if !ok || errObj == nil {
		return nil
	}
	typ := strings.TrimSpace(stringFromAny(errObj["type"]))
	code := strings.TrimSpace(stringFromAny(errObj["code"]))
	if !strings.EqualFold(typ, "usage_limit_reached") &&
		!strings.EqualFold(code, "usage_limit_reached") &&
		!strings.EqualFold(typ, "rate_limit_exceeded") &&
		!strings.EqualFold(code, "rate_limit_exceeded") {
		return nil
	}
	if ts, ok := int64FromJSONScalar(errObj["resets_at"]); ok && ts > 0 {
		t := time.Unix(ts, 0)
		return &t
	}
	if sec, ok := int64FromJSONScalar(errObj["resets_in_seconds"]); ok && sec > 0 {
		t := now.Add(time.Duration(sec) * time.Second)
		return &t
	}
	return nil
}

func int64FromJSONScalar(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func (h *Handler) setCodexCredentialCooldown(ctx context.Context, sel scheduler.Selection, until time.Time) {
	if h == nil || h.exec == nil {
		return
	}
	if sel.CredentialType != scheduler.CredentialTypeCodex || sel.CredentialID <= 0 || until.IsZero() {
		return
	}
	setter, ok := h.exec.(CodexCooldownSetter)
	if !ok {
		return
	}
	_ = setter.SetCodexOAuthAccountCooldown(ctx, sel.CredentialID, until)
}

func (h *Handler) setCodexCredentialDisabled(ctx context.Context, sel scheduler.Selection) {
	if h == nil || h.exec == nil {
		return
	}
	if sel.CredentialType != scheduler.CredentialTypeCodex || sel.CredentialID <= 0 {
		return
	}
	setter, ok := h.exec.(CodexStatusSetter)
	if !ok {
		return
	}
	_ = setter.SetCodexOAuthAccountStatus(ctx, sel.CredentialID, 0)
}

func (h *Handler) setCodexCredentialQuotaError(ctx context.Context, sel scheduler.Selection, msg string) {
	if h == nil || h.exec == nil {
		return
	}
	if sel.CredentialType != scheduler.CredentialTypeCodex || sel.CredentialID <= 0 {
		return
	}
	setter, ok := h.exec.(CodexQuotaErrorSetter)
	if !ok {
		return
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	_ = setter.SetCodexOAuthAccountQuotaError(ctx, sel.CredentialID, &msg)
}

type codexOAuthErrKind int

const (
	codexOAuthErrNone codexOAuthErrKind = iota
	codexOAuthErrRateLimited
	codexOAuthErrBalanceDepleted
	codexOAuthErrCredentialInvalid
)

type codexOAuthUpstreamErr struct {
	Kind                codexOAuthErrKind
	DisableAccount      bool
	MarkBalanceDepleted bool
}

func (e codexOAuthUpstreamErr) retriable() bool {
	return e.Kind != codexOAuthErrNone
}

func (e codexOAuthUpstreamErr) errorClass() string {
	switch e.Kind {
	case codexOAuthErrRateLimited:
		return "upstream_throttled"
	case codexOAuthErrBalanceDepleted:
		return "upstream_exhausted"
	case codexOAuthErrCredentialInvalid:
		return "upstream_credential_invalid"
	default:
		return ""
	}
}

func (e codexOAuthUpstreamErr) cooldownUntil(now time.Time, body []byte) *time.Time {
	switch e.Kind {
	case codexOAuthErrRateLimited:
		until := codexRateLimitCooldownUntil(now, body)
		return &until
	case codexOAuthErrBalanceDepleted:
		until := codexUsageLimitCooldownUntil(now, body)
		return &until
	default:
		return nil
	}
}

func classifyCodexOAuthUpstreamError(sel scheduler.Selection, statusCode int, body []byte) codexOAuthUpstreamErr {
	if sel.CredentialType != scheduler.CredentialTypeCodex {
		return codexOAuthUpstreamErr{}
	}
	code, typ := extractUpstreamErrorCodeAndType(body)
	code = strings.ToLower(strings.TrimSpace(code))
	typ = strings.ToLower(strings.TrimSpace(typ))
	msg := strings.ToLower(summarizeUpstreamErrorBody(body))

	isUsageLimit := typ == "usage_limit_reached" || code == "usage_limit_reached"
	isUsageLimit = isUsageLimit || typ == "insufficient_quota" || code == "insufficient_quota"
	isUsageLimit = isUsageLimit || typ == "billing_hard_limit_reached" || code == "billing_hard_limit_reached"
	if statusCode == http.StatusPaymentRequired {
		isUsageLimit = true
	}

	if isUsageLimit {
		return codexOAuthUpstreamErr{
			Kind:                codexOAuthErrBalanceDepleted,
			MarkBalanceDepleted: true,
		}
	}

	// rate_limit_exceeded / 429：优先视为限流（短冷却），避免把限流误判为“余额用尽”。
	if typ == "rate_limit_exceeded" || code == "rate_limit_exceeded" || statusCode == http.StatusTooManyRequests {
		return codexOAuthUpstreamErr{Kind: codexOAuthErrRateLimited}
	}

	// 401/403 常见为 token 被撤销/账号被封禁：高置信才禁用，避免误伤。
	if statusCode == http.StatusUnauthorized {
		if typ == "invalid_authentication" || code == "invalid_authentication" ||
			typ == "invalid_api_key" || code == "invalid_api_key" ||
			typ == "invalid_token" || code == "invalid_token" ||
			typ == "token_expired" || code == "token_expired" ||
			(strings.Contains(msg, "invalid") && strings.Contains(msg, "token")) {
			return codexOAuthUpstreamErr{
				Kind:           codexOAuthErrCredentialInvalid,
				DisableAccount: true,
			}
		}
	}
	if statusCode == http.StatusForbidden {
		if typ == "account_deactivated" || code == "account_deactivated" ||
			strings.Contains(msg, "suspended") || strings.Contains(msg, "banned") ||
			strings.Contains(msg, "disabled") || strings.Contains(msg, "deactivated") {
			return codexOAuthUpstreamErr{
				Kind:           codexOAuthErrCredentialInvalid,
				DisableAccount: true,
			}
		}
	}

	return codexOAuthUpstreamErr{}
}

func extractUpstreamErrorCodeAndType(body []byte) (string, string) {
	if len(body) == 0 {
		return "", ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", ""
	}

	var code, typ string
	if s, ok := parsed["code"].(string); ok {
		code = s
	}
	if s, ok := parsed["type"].(string); ok {
		typ = s
	}

	if errObj, ok := parsed["error"].(map[string]any); ok {
		if s, ok := errObj["code"].(string); ok && strings.TrimSpace(code) == "" {
			code = s
		}
		if s, ok := errObj["type"].(string); ok && strings.TrimSpace(typ) == "" {
			typ = s
		}
	}
	return strings.TrimSpace(code), strings.TrimSpace(typ)
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

const (
	upstreamErrorBodyMaxBytes        = 64 << 10
	failoverErrorBodyMaxBytes        = 8 << 10
	upstreamNonStreamExtractMaxBytes = 2 << 20
)

func readPrefixBestEffort(r io.Reader, max int64) []byte {
	if r == nil {
		return nil
	}
	if max <= 0 {
		b, _ := io.ReadAll(r)
		return b
	}
	lr := &io.LimitedReader{R: r, N: max + 1}
	b, _ := io.ReadAll(lr)
	if int64(len(b)) > max {
		return b[:max]
	}
	return b
}

type limitedPrefixBuffer struct {
	buf      bytes.Buffer
	maxBytes int64
	exceeded bool
}

func (b *limitedPrefixBuffer) Write(p []byte) (int, error) {
	if b == nil || len(p) == 0 {
		return len(p), nil
	}
	if b.maxBytes <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	remaining := b.maxBytes - int64(b.buf.Len())
	if remaining <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.exceeded = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
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
	if sessionID := extractSessionIDFromMetadataUserID(payload); sessionID != "" {
		return sessionID
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
	if ok {
		meta, ok := metaAny.(map[string]any)
		if ok {
			for _, key := range []string{
				"prompt_cache_key",
				"session_id",
				"conversation_id",
			} {
				if s := normalizeRouteKey(meta[key]); s != "" {
					return s
				}
			}
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

func shouldEnableStickyRouting(payload map[string]any, r *http.Request, routeKeySource string) bool {
	if payload == nil || r == nil {
		return false
	}
	// derived/missing 路由键不够稳定：避免把“内容哈希”或空值当会话键导致意外粘性。
	if routeKeySource == "derived" || routeKeySource == "missing" {
		return false
	}
	// sticky routing 仅用于“有状态会话”：
	// - Codex-like responses payload（input 数组）
	// - /v1/responses/compact（input 可能是 string，但 compact 结果会影响后续续链）
	// - 已携带续链信号（compaction/item_reference/previous_response_id）
	path := ""
	if r.URL != nil {
		path = strings.TrimSpace(r.URL.Path)
	}
	if _, ok := payload["input"].([]any); !ok && path != "/v1/responses/compact" && !codexHasStatefulContinuationSignals(payload) {
		return false
	}

	// 只在客户端显式提供会话键时启用粘性（或 Codex UA 兜底），避免无意间改变路由行为。
	if strings.TrimSpace(extractSessionIDFromHeaders(r.Header)) != "" {
		return true
	}
	if strings.TrimSpace(stringFromAny(payload["prompt_cache_key"])) != "" {
		return true
	}
	ua := strings.ToLower(strings.TrimSpace(r.Header.Get("User-Agent")))
	return strings.Contains(ua, "codex")
}

type codexStickyBindingPayloadV1 struct {
	Kind            string `json:"kind,omitempty"`
	ChannelID       int64  `json:"channel_id"`
	CredentialKey   string `json:"credential_key"`
	UpdatedAtUnixMS int64  `json:"updated_at_unix_ms,omitempty"`
}

func parseCodexStickyBindingPayload(payload string, now time.Time) (codexLastSuccessRoute, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return codexLastSuccessRoute{}, false
	}
	var parsed codexStickyBindingPayloadV1
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return codexLastSuccessRoute{}, false
	}
	if parsed.ChannelID <= 0 {
		return codexLastSuccessRoute{}, false
	}
	kind := strings.TrimSpace(parsed.Kind)
	if kind != "" && kind != "codex_route_v1" {
		return codexLastSuccessRoute{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	return codexLastSuccessRoute{
		channelID:     parsed.ChannelID,
		credentialKey: strings.TrimSpace(parsed.CredentialKey),
		expiresAt:     now.Add(codexSessionTTL()),
	}, true
}

func codexStickyBindingPayloadJSON(sel scheduler.Selection) (string, bool) {
	if sel.ChannelID <= 0 {
		return "", false
	}
	b, err := json.Marshal(codexStickyBindingPayloadV1{
		Kind:            "codex_route_v1",
		ChannelID:       sel.ChannelID,
		CredentialKey:   strings.TrimSpace(sel.CredentialKey()),
		UpdatedAtUnixMS: time.Now().UnixMilli(),
	})
	if err != nil || len(b) == 0 {
		return "", false
	}
	return string(b), true
}

func (h *Handler) loadCodexStickyBinding(ctx context.Context, userID int64, stickyRouteKeyHash string, now time.Time) (codexLastSuccessRoute, bool) {
	if h == nil || userID <= 0 || strings.TrimSpace(stickyRouteKeyHash) == "" {
		return codexLastSuccessRoute{}, false
	}
	if v, ok := h.getCodexLastSuccessRoute(userID, stickyRouteKeyHash, now); ok {
		return v, true
	}
	if h.sessionBindings == nil {
		return codexLastSuccessRoute{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, ok, err := h.sessionBindings.GetSessionBindingPayload(ctx, userID, stickyRouteKeyHash, now)
	if err != nil || !ok {
		return codexLastSuccessRoute{}, false
	}
	route, parsed := parseCodexStickyBindingPayload(payload, now)
	if !parsed {
		return codexLastSuccessRoute{}, false
	}
	if h.codexRouteCache != nil {
		key := codexStickyBindingKey(userID, stickyRouteKeyHash)
		if key != "" {
			h.codexRouteCache.SetRoute(key, route)
		}
	}
	return route, true
}

func (h *Handler) clearCodexStickyBindingBestEffort(ctx context.Context, userID int64, stickyRouteKeyHash string) {
	if h == nil || userID <= 0 || strings.TrimSpace(stickyRouteKeyHash) == "" {
		return
	}
	if h.sessionBindings != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		_ = h.sessionBindings.DeleteSessionBinding(ctx, userID, stickyRouteKeyHash)
	}
	h.clearCodexLastSuccessRoute(userID, stickyRouteKeyHash)
}

func (h *Handler) upsertCodexStickyBindingBestEffort(ctx context.Context, userID int64, stickyRouteKeyHash string, sel scheduler.Selection) {
	if h == nil || h.sessionBindings == nil || userID <= 0 || strings.TrimSpace(stickyRouteKeyHash) == "" {
		return
	}
	payload, ok := codexStickyBindingPayloadJSON(sel)
	if !ok {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = h.sessionBindings.UpsertSessionBindingPayload(ctx, userID, stickyRouteKeyHash, payload, time.Now().Add(codexSessionTTL()))
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
