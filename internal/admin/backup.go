package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"realms/internal/store"
)

func (s *Server) Backup(w http.ResponseWriter, r *http.Request) {
	// 导入/导出已合并至“系统设置”页面。
	target := "/admin/settings#backup"
	if strings.TrimSpace(r.URL.RawQuery) != "" {
		target = "/admin/settings?" + r.URL.RawQuery + "#backup"
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) Export(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	out, err := s.st.ExportAdminConfig(r.Context())
	if err != nil {
		http.Error(w, "导出失败", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("realms-export-%s.json", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func (s *Server) Import(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	var reader io.Reader = r.Body
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		f, _, err := r.FormFile("file")
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "请选择要导入的 JSON 文件")
				return
			}
			http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("请选择要导入的 JSON 文件")+"#backup", http.StatusFound)
			return
		}
		defer func() { _ = f.Close() }()
		reader = f
	}

	var payload store.AdminConfigExport
	dec := json.NewDecoder(reader)
	if err := dec.Decode(&payload); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "JSON 解析失败")
			return
		}
		http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape("JSON 解析失败")+"#backup", http.StatusFound)
		return
	}

	rep, err := s.st.ImportAdminConfig(r.Context(), payload)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "导入失败: "+err.Error())
			return
		}
		msg := err.Error()
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		http.Redirect(w, r, "/admin/settings?err="+url.QueryEscape(msg)+"#backup", http.StatusFound)
		return
	}

	if isAjax(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"report": rep,
		})
		return
	}

	http.Redirect(w, r, "/admin/settings?msg="+url.QueryEscape("已导入（upsert）：分组 "+fmt.Sprint(rep.ChannelGroups)+"，成员 "+fmt.Sprint(rep.ChannelGroupMembers)+"，渠道 "+fmt.Sprint(rep.UpstreamChannels)+"，端点 "+fmt.Sprint(rep.UpstreamEndpoints)+"，模型 "+fmt.Sprint(rep.ManagedModels)+"，绑定 "+fmt.Sprint(rep.ChannelModels))+"#backup", http.StatusFound)
}
