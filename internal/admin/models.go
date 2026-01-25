package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"realms/internal/store"
)

type managedModelView struct {
	ID                  int64
	PublicID            string
	OwnedBy             string
	InputUSDPer1M       string
	OutputUSDPer1M      string
	CacheInputUSDPer1M  string
	CacheOutputUSDPer1M string
	Status              int
	CreatedAt           string
}

func toManagedModelView(m store.ManagedModel, loc *time.Location) managedModelView {
	v := managedModelView{
		ID:                  m.ID,
		PublicID:            m.PublicID,
		InputUSDPer1M:       formatUSDPlain(m.InputUSDPer1M),
		OutputUSDPer1M:      formatUSDPlain(m.OutputUSDPer1M),
		CacheInputUSDPer1M:  formatUSDPlain(m.CacheInputUSDPer1M),
		CacheOutputUSDPer1M: formatUSDPlain(m.CacheOutputUSDPer1M),
		Status:              m.Status,
		CreatedAt:           formatTimeIn(m.CreatedAt, "2006-01-02 15:04", loc),
	}
	if m.OwnedBy != nil {
		v.OwnedBy = *m.OwnedBy
	}
	return v
}

func (s *Server) Models(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	loc, _ := s.adminTimeLocation(r.Context())
	ms, err := s.st.ListManagedModels(r.Context())
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	views := make([]managedModelView, 0, len(ms))
	for _, m := range ms {
		views = append(views, toManagedModelView(m, loc))
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_models", s.withFeatures(r.Context(), templateData{
		Title:         "模型管理 - Realms",
		Error:         errMsg,
		Notice:        notice,
		User:          u,
		IsRoot:        isRoot,
		CSRFToken:     csrf,
		ManagedModels: views,
	}))
}

func (s *Server) CreateModel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	publicID := strings.TrimSpace(r.FormValue("public_id"))
	ownedByRaw := strings.TrimSpace(r.FormValue("owned_by"))
	inUSDRaw := strings.TrimSpace(r.FormValue("input_usd_per_1m"))
	outUSDRaw := strings.TrimSpace(r.FormValue("output_usd_per_1m"))
	cacheInUSDRaw := strings.TrimSpace(r.FormValue("cache_input_usd_per_1m"))
	cacheOutUSDRaw := strings.TrimSpace(r.FormValue("cache_output_usd_per_1m"))
	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		status = 1
	}

	if publicID == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "public_id 不能为空")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("public_id 不能为空"), http.StatusFound)
		return
	}
	if status != 0 && status != 1 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "status 不合法")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("status 不合法"), http.StatusFound)
		return
	}

	if inUSDRaw == "" || outUSDRaw == "" || cacheInUSDRaw == "" || cacheOutUSDRaw == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "定价不能为空")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("定价不能为空"), http.StatusFound)
		return
	}
	inV, err := parseUSD(inUSDRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "input_price 不合法")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("input_price 不合法"), http.StatusFound)
		return
	}
	outV, err := parseUSD(outUSDRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "output_price 不合法")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("output_price 不合法"), http.StatusFound)
		return
	}
	cacheInV, err := parseUSD(cacheInUSDRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "cache_input_price 不合法")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("cache_input_price 不合法"), http.StatusFound)
		return
	}
	cacheOutV, err := parseUSD(cacheOutUSDRaw)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "cache_output_price 不合法")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("cache_output_price 不合法"), http.StatusFound)
		return
	}

	var ownedBy *string
	if ownedByRaw != "" {
		v := ownedByRaw
		ownedBy = &v
	}

	if _, err := s.st.CreateManagedModel(r.Context(), store.ManagedModelCreate{
		PublicID:            publicID,
		OwnedBy:             ownedBy,
		InputUSDPer1M:       inV,
		OutputUSDPer1M:      outV,
		CacheInputUSDPer1M:  cacheInV,
		CacheOutputUSDPer1M: cacheOutV,
		Status:              status,
	}); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "创建失败")
			return
		}
		http.Redirect(w, r, "/admin/models?err="+url.QueryEscape("创建失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/models?msg="+url.QueryEscape("已创建"), http.StatusFound)
}

func (s *Server) Model(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	loc, _ := s.adminTimeLocation(r.Context())
	modelID, err := parseInt64(r.PathValue("model_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	m, err := s.st.GetManagedModelByID(r.Context(), modelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "模型不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	s.render(w, "admin_model_edit", s.withFeatures(r.Context(), templateData{
		Title:        "编辑模型 - Realms",
		Notice:       notice,
		User:         u,
		IsRoot:       isRoot,
		CSRFToken:    csrf,
		ManagedModel: toManagedModelView(m, loc),
	}))
}

func (s *Server) UpdateModel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	modelID, err := parseInt64(r.PathValue("model_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	publicID := strings.TrimSpace(r.FormValue("public_id"))
	ownedByRaw := strings.TrimSpace(r.FormValue("owned_by"))
	inUSDRaw := strings.TrimSpace(r.FormValue("input_usd_per_1m"))
	outUSDRaw := strings.TrimSpace(r.FormValue("output_usd_per_1m"))
	cacheInUSDRaw := strings.TrimSpace(r.FormValue("cache_input_usd_per_1m"))
	cacheOutUSDRaw := strings.TrimSpace(r.FormValue("cache_output_usd_per_1m"))
	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}

	if publicID == "" {
		http.Error(w, "public_id 不能为空", http.StatusBadRequest)
		return
	}
	if status != 0 && status != 1 {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}

	if inUSDRaw == "" || outUSDRaw == "" || cacheInUSDRaw == "" || cacheOutUSDRaw == "" {
		http.Error(w, "定价不能为空", http.StatusBadRequest)
		return
	}
	inV, err := parseUSD(inUSDRaw)
	if err != nil {
		http.Error(w, "input_price 不合法", http.StatusBadRequest)
		return
	}
	outV, err := parseUSD(outUSDRaw)
	if err != nil {
		http.Error(w, "output_price 不合法", http.StatusBadRequest)
		return
	}
	cacheInV, err := parseUSD(cacheInUSDRaw)
	if err != nil {
		http.Error(w, "cache_input_price 不合法", http.StatusBadRequest)
		return
	}
	cacheOutV, err := parseUSD(cacheOutUSDRaw)
	if err != nil {
		http.Error(w, "cache_output_price 不合法", http.StatusBadRequest)
		return
	}

	var ownedBy *string
	if ownedByRaw != "" {
		v := ownedByRaw
		ownedBy = &v
	}

	if err := s.st.UpdateManagedModel(r.Context(), store.ManagedModelUpdate{
		ID:                  modelID,
		PublicID:            publicID,
		OwnedBy:             ownedBy,
		InputUSDPer1M:       inV,
		OutputUSDPer1M:      outV,
		CacheInputUSDPer1M:  cacheInV,
		CacheOutputUSDPer1M: cacheOutV,
		Status:              status,
	}); err != nil {
		http.Error(w, "更新失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/models/"+fmt.Sprintf("%d", modelID)+"?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) DeleteModel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	modelID, err := parseInt64(r.PathValue("model_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := s.st.DeleteManagedModel(r.Context(), modelID); err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已删除")
		return
	}
	http.Redirect(w, r, "/admin/models?msg="+url.QueryEscape("已删除"), http.StatusFound)
}
