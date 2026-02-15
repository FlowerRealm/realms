package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/sjson"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/obs"
	"realms/internal/scheduler"
	"realms/internal/upstream"
)

func responseIDFromPath(path string) string {
	const prefix = "/v1/responses/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return ""
	}
	id := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		id = rest[:i]
	}
	id = strings.TrimSpace(id)
	if !isLikelyResponseID(id) {
		return ""
	}
	return id
}

func chatCompletionIDFromPath(path string) string {
	const prefix = "/v1/chat/completions/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return ""
	}
	id := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		id = rest[:i]
	}
	id = strings.TrimSpace(id)
	if !isLikelyChatCompletionID(id) {
		return ""
	}
	return id
}

func modelIDFromPath(path string) string {
	const prefix = "/v1/models/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return ""
	}
	id := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		id = rest[:i]
	}
	return strings.TrimSpace(id)
}

func wantsEventStream(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	switch v := strings.TrimSpace(r.URL.Query().Get("stream")); strings.ToLower(v) {
	case "1", "true":
		return true
	default:
		return false
	}
}

func (h *Handler) proxyFixedSelection(w http.ResponseWriter, r *http.Request, p auth.Principal, sel scheduler.Selection, body []byte) int {
	attemptStart := time.Now()

	resp, err := h.exec.Do(r.Context(), sel, r, body)
	if err != nil {
		if h.sched != nil {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "network"})
		}
		h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, nil, 0, "network", time.Since(attemptStart))
		http.Error(w, "上游不可用", http.StatusBadGateway)
		return http.StatusBadGateway
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	downstreamStatus := resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		downstreamStatus = resetStatusCode(resp.StatusCode, sel.StatusCodeMapping)
	}

	if wantsEventStream(r) || isSSE {
		var routeKey string
		cw := &countingResponseWriter{ResponseWriter: w}
		copyResponseHeaders(cw.Header(), resp.Header)
		cw.Header().Set("X-Accel-Buffering", "no")
		if isSSE {
			cw.Header().Set("Content-Type", "text/event-stream")
		}
		cw.WriteHeader(downstreamStatus)

		doneSSE := obs.TrackSSEConnection()
		defer doneSSE()
		pumpRes, _ := upstream.PumpSSE(r.Context(), cw, resp.Body, h.sseOpts, upstream.SSEPumpHooks{
			OnData: func(data string) {
				if routeKey != "" || !strings.Contains(data, "session") && !strings.Contains(data, "conversation") && !strings.Contains(data, "previous_response_id") {
					return
				}
				var evt any
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					return
				}
				routeKey = extractRouteKeyFromStructuredData(evt)
			},
		})
		obs.RecordSSEFirstWriteLatency(pumpRes.FirstWriteLatency)
		obs.RecordSSEBytesStreamed(pumpRes.BytesWritten)
		obs.RecordSSEPumpResult(pumpRes.ErrorClass, pumpRes.SawDone)
		h.touchBindingFromRouteKey(p.UserID, sel, routeKey)

		if h.sched != nil {
			if pumpRes.ErrorClass == "" || pumpRes.ErrorClass == "client_disconnect" || pumpRes.ErrorClass == "stream_max_duration" {
				h.sched.Report(sel, scheduler.Result{Success: resp.StatusCode >= 200 && resp.StatusCode < 300})
			} else {
				h.sched.Report(sel, scheduler.Result{Success: false, Retriable: false, StatusCode: resp.StatusCode, ErrorClass: pumpRes.ErrorClass})
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, nil, resp.StatusCode, "upstream_status", time.Since(attemptStart))
		} else if pumpRes.ErrorClass != "" && pumpRes.ErrorClass != "client_disconnect" && pumpRes.ErrorClass != "stream_max_duration" {
			h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, nil, resp.StatusCode, pumpRes.ErrorClass, time.Since(attemptStart))
		}
		return downstreamStatus
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		if h.sched != nil {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "read_upstream"})
		}
		h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, nil, 0, "read_upstream", time.Since(attemptStart))
		http.Error(w, "读取上游响应失败", http.StatusBadGateway)
		return http.StatusBadGateway
	}

	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(downstreamStatus)
	_, _ = w.Write(bodyBytes)

	if h.sched != nil {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			h.sched.Report(sel, scheduler.Result{Success: true})
		} else {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: false, StatusCode: resp.StatusCode, ErrorClass: "upstream_status"})
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.auditUpstreamError(r.Context(), r.URL.Path, p, &sel, nil, resp.StatusCode, "upstream_status", time.Since(attemptStart))
	}
	return downstreamStatus
}

func (h *Handler) ResponseRetrieve(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := responseIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeResponse, id)
	if !ok {
		writeNotFound(w)
		return
	}
	if sel.CredentialType == scheduler.CredentialTypeCodex {
		http.Error(w, "codex_oauth 上游暂不支持该端点", http.StatusNotImplemented)
		return
	}
	_ = h.proxyFixedSelection(w, r, p, sel, nil)
}

func (h *Handler) ResponseDelete(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := responseIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeResponse, id)
	if !ok {
		writeNotFound(w)
		return
	}
	if sel.CredentialType == scheduler.CredentialTypeCodex {
		http.Error(w, "codex_oauth 上游暂不支持该端点", http.StatusNotImplemented)
		return
	}
	status := h.proxyFixedSelection(w, r, p, sel, nil)
	if h.refs != nil && (status == http.StatusNotFound || (status >= 200 && status < 300)) {
		_ = h.refs.DeleteOpenAIObjectRef(r.Context(), openAIObjectTypeResponse, id)
	}
}

func (h *Handler) ResponseCancel(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := responseIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeResponse, id)
	if !ok {
		writeNotFound(w)
		return
	}
	if sel.CredentialType == scheduler.CredentialTypeCodex {
		http.Error(w, "codex_oauth 上游暂不支持该端点", http.StatusNotImplemented)
		return
	}
	body := middleware.CachedBody(r.Context())
	_ = h.proxyFixedSelection(w, r, p, sel, body)
}

func (h *Handler) ResponseInputItems(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := responseIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeResponse, id)
	if !ok {
		writeNotFound(w)
		return
	}
	if sel.CredentialType == scheduler.CredentialTypeCodex {
		http.Error(w, "codex_oauth 上游暂不支持该端点", http.StatusNotImplemented)
		return
	}
	_ = h.proxyFixedSelection(w, r, p, sel, nil)
}

func (h *Handler) ChatCompletionsList(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	if h.refs == nil {
		writeNotFound(w)
		return
	}

	ctx := r.Context()
	refs, err := h.refs.ListOpenAIObjectRefsByUser(ctx, p.UserID, openAIObjectTypeChatCompletion, 500)
	if err != nil {
		http.Error(w, "查询对象归属失败", http.StatusBadGateway)
		return
	}
	if len(refs) == 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
		return
	}

	allowIDs := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.ObjectID) == "" {
			continue
		}
		allowIDs[ref.ObjectID] = struct{}{}
	}

	var sel scheduler.Selection
	if err := json.Unmarshal([]byte(refs[0].SelectionJSON), &sel); err != nil {
		http.Error(w, "解析对象归属失败", http.StatusBadGateway)
		return
	}

	upReq := r.Clone(ctx)
	q := upReq.URL.Query()
	for k := range q {
		if strings.HasPrefix(k, "metadata[") {
			q.Del(k)
		}
	}
	if ownerTag := realmsOwnerTagForUser(p.UserID); ownerTag != "" {
		q.Set("metadata["+realmsOwnerMetadataKey+"]", ownerTag)
	}
	upReq.URL.RawQuery = q.Encode()

	attemptStart := time.Now()
	resp, err := h.exec.Do(ctx, sel, upReq, nil)
	if err != nil {
		if h.sched != nil {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "network"})
		}
		h.auditUpstreamError(ctx, r.URL.Path, p, &sel, nil, 0, "network", time.Since(attemptStart))
		http.Error(w, "上游不可用", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		if h.sched != nil {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: true, ErrorClass: "read_upstream"})
		}
		h.auditUpstreamError(ctx, r.URL.Path, p, &sel, nil, 0, "read_upstream", time.Since(attemptStart))
		http.Error(w, "读取上游响应失败", http.StatusBadGateway)
		return
	}

	downstreamStatus := resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		downstreamStatus = resetStatusCode(resp.StatusCode, sel.StatusCodeMapping)
	}
	if h.sched != nil {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			h.sched.Report(sel, scheduler.Result{Success: true})
		} else {
			h.sched.Report(sel, scheduler.Result{Success: false, Retriable: false, StatusCode: resp.StatusCode, ErrorClass: "upstream_status"})
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.auditUpstreamError(ctx, r.URL.Path, p, &sel, nil, resp.StatusCode, "upstream_status", time.Since(attemptStart))
		copyResponseHeaders(w.Header(), resp.Header)
		w.Header().Del("Content-Length")
		w.WriteHeader(downstreamStatus)
		_, _ = w.Write(bodyBytes)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err == nil {
		if data, ok := payload["data"].([]any); ok {
			filtered := make([]any, 0, len(data))
			for _, item := range data {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				id, _ := m["id"].(string)
				if _, ok := allowIDs[id]; !ok {
					continue
				}
				filtered = append(filtered, item)
			}
			payload["data"] = filtered
			if len(filtered) > 0 {
				if first, ok := filtered[0].(map[string]any); ok {
					if id, _ := first["id"].(string); strings.TrimSpace(id) != "" {
						payload["first_id"] = id
					}
				}
				if last, ok := filtered[len(filtered)-1].(map[string]any); ok {
					if id, _ := last["id"].(string); strings.TrimSpace(id) != "" {
						payload["last_id"] = id
					}
				}
			} else {
				payload["first_id"] = nil
				payload["last_id"] = nil
				payload["has_more"] = false
			}

			if out, err := json.Marshal(payload); err == nil {
				bodyBytes = out
			}
		}
	}

	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(downstreamStatus)
	_, _ = w.Write(bodyBytes)
}

func (h *Handler) ChatCompletionRetrieve(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := chatCompletionIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeChatCompletion, id)
	if !ok {
		writeNotFound(w)
		return
	}
	_ = h.proxyFixedSelection(w, r, p, sel, nil)
}

func (h *Handler) ChatCompletionUpdate(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := chatCompletionIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeChatCompletion, id)
	if !ok {
		writeNotFound(w)
		return
	}
	body := middleware.CachedBody(r.Context())
	if len(body) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}
	ownerTag := realmsOwnerTagForUser(p.UserID)
	if ownerTag != "" {
		if patched, err := sjson.SetBytes(body, "metadata."+realmsOwnerMetadataKey, ownerTag); err == nil {
			body = patched
		}
	}
	_ = h.proxyFixedSelection(w, r, p, sel, body)
}

func (h *Handler) ChatCompletionDelete(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := chatCompletionIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeChatCompletion, id)
	if !ok {
		writeNotFound(w)
		return
	}
	status := h.proxyFixedSelection(w, r, p, sel, nil)
	if h.refs != nil && (status == http.StatusNotFound || (status >= 200 && status < 300)) {
		_ = h.refs.DeleteOpenAIObjectRef(r.Context(), openAIObjectTypeChatCompletion, id)
	}
}

func (h *Handler) ChatCompletionMessages(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := chatCompletionIDFromPath(r.URL.Path)
	if id == "" {
		writeNotFound(w)
		return
	}
	sel, ok := h.ownedSelection(r.Context(), p, openAIObjectTypeChatCompletion, id)
	if !ok {
		writeNotFound(w)
		return
	}
	_ = h.proxyFixedSelection(w, r, p, sel, nil)
}

func (h *Handler) ModelRetrieve(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ActorType != auth.ActorTypeToken || p.TokenID == nil {
		http.Error(w, "未鉴权", http.StatusUnauthorized)
		return
	}
	id := modelIDFromPath(r.URL.Path)
	if strings.TrimSpace(id) == "" {
		writeNotFound(w)
		return
	}
	if h.models == nil {
		http.Error(w, "服务未配置模型目录", http.StatusBadGateway)
		return
	}
	m, err := h.models.GetEnabledManagedModelByPublicID(r.Context(), id)
	if err != nil {
		writeNotFound(w)
		return
	}
	ags := allowGroupsFromPrincipal(p)
	if _, ok := ags.Set[managedModelGroupName(m)]; !ok {
		writeNotFound(w)
		return
	}
	// 与 Models() 保持一致：无绑定则视为不可用。
	if bindings, err := h.models.ListEnabledChannelModelBindingsByPublicID(r.Context(), id); err != nil || len(bindings) == 0 {
		writeNotFound(w)
		return
	}

	ownedBy := "realms"
	if m.OwnedBy != nil && strings.TrimSpace(*m.OwnedBy) != "" {
		ownedBy = *m.OwnedBy
	}
	out := struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}{
		ID:      id,
		Object:  "model",
		Created: 0,
		OwnedBy: ownedBy,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}
