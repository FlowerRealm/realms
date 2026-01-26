package web

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"realms/internal/auth"
)

type usageEventDetailAPIResponse struct {
	EventID               int64  `json:"event_id"`
	Available             bool   `json:"available"`
	DownstreamRequestBody string `json:"downstream_request_body,omitempty"`
	UpstreamRequestBody   string `json:"upstream_request_body,omitempty"`
	UpstreamResponseBody  string `json:"upstream_response_body,omitempty"`
}

func (s *Server) UsageEventDetailAPI(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	idStr := strings.TrimSpace(r.PathValue("event_id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "event_id 不合法", http.StatusBadRequest)
		return
	}

	ev, err := s.store.GetUsageEvent(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	if ev.UserID != p.UserID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	detail, err := s.store.GetUsageEventDetail(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, usageEventDetailAPIResponse{EventID: id, Available: false})
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	resp := usageEventDetailAPIResponse{
		EventID:   id,
		Available: true,
	}
	if detail.DownstreamRequestBody != nil {
		resp.DownstreamRequestBody = *detail.DownstreamRequestBody
	}
	if detail.UpstreamRequestBody != nil {
		resp.UpstreamRequestBody = *detail.UpstreamRequestBody
	}
	if detail.UpstreamResponseBody != nil {
		resp.UpstreamResponseBody = *detail.UpstreamResponseBody
	}
	writeJSON(w, resp)
}
