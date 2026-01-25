package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"realms/internal/icons"
	"realms/internal/modellibrary"
)

type modelLibraryLookupResponse struct {
	OK     bool   `json:"ok"`
	Notice string `json:"notice,omitempty"`
	Error  string `json:"error,omitempty"`

	OwnedBy             string `json:"owned_by,omitempty"`
	InputUSDPer1M       string `json:"input_usd_per_1m,omitempty"`
	OutputUSDPer1M      string `json:"output_usd_per_1m,omitempty"`
	CacheInputUSDPer1M  string `json:"cache_input_usd_per_1m,omitempty"`
	CacheOutputUSDPer1M string `json:"cache_output_usd_per_1m,omitempty"`
	Source              string `json:"source,omitempty"`
	IconURL             string `json:"icon_url,omitempty"`
}

func (s *Server) ModelLibraryLookup(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		ajaxError(w, http.StatusBadRequest, "表单解析失败")
		return
	}

	modelID := strings.TrimSpace(r.FormValue("model_id"))
	if modelID == "" {
		ajaxError(w, http.StatusBadRequest, "model_id 不能为空")
		return
	}

	res, err := s.modelsDev.Lookup(r.Context(), modelID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, modellibrary.ErrModelNotFound) {
			status = http.StatusNotFound
		}
		ajaxError(w, status, err.Error())
		return
	}

	resp := modelLibraryLookupResponse{
		OK:                  true,
		Notice:              "已从模型库填充（models.dev）",
		Source:              res.Source,
		OwnedBy:             res.OwnedBy,
		InputUSDPer1M:       formatUSDPlain(res.InputUSDPer1M),
		OutputUSDPer1M:      formatUSDPlain(res.OutputUSDPer1M),
		CacheInputUSDPer1M:  formatUSDPlain(res.CacheInputUSDPer1M),
		CacheOutputUSDPer1M: formatUSDPlain(res.CacheOutputUSDPer1M),
		IconURL:             icons.ModelIconURL(modelID, res.OwnedBy),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
