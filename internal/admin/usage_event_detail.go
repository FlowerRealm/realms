package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type usageEventDetailAPIResponse struct {
	EventID              int64  `json:"event_id"`
	Available            bool   `json:"available"`
	UpstreamRequestBody  string `json:"upstream_request_body,omitempty"`
	UpstreamResponseBody string `json:"upstream_response_body,omitempty"`
}

func (s *Server) UsageEventDetailAPI(w http.ResponseWriter, r *http.Request) {
	_, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}

	idStr := strings.TrimSpace(r.PathValue("event_id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "event_id 不合法", http.StatusBadRequest)
		return
	}

	detail, err := s.st.GetUsageEventDetail(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(usageEventDetailAPIResponse{EventID: id, Available: false})
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	resp := usageEventDetailAPIResponse{
		EventID:   id,
		Available: true,
	}
	if detail.UpstreamRequestBody != nil {
		resp.UpstreamRequestBody = *detail.UpstreamRequestBody
	}
	if detail.UpstreamResponseBody != nil {
		resp.UpstreamResponseBody = *detail.UpstreamResponseBody
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
}
