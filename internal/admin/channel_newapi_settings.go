package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"realms/internal/store"
)

func (s *Server) UpdateChannelNewAPIMeta(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
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

	openAIOrganization := strings.TrimSpace(r.FormValue("openai_organization"))
	testModel := strings.TrimSpace(r.FormValue("test_model"))
	tag := strings.TrimSpace(r.FormValue("tag"))
	remark := strings.TrimSpace(r.FormValue("remark"))
	weightRaw := strings.TrimSpace(r.FormValue("weight"))
	autoBan := strings.TrimSpace(r.FormValue("auto_ban")) != "0"

	weight := 0
	if weightRaw != "" {
		v, err := strconv.Atoi(weightRaw)
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "weight 不合法")
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("weight 不合法"), http.StatusFound)
			return
		}
		weight = v
	}

	if err := s.st.UpdateUpstreamChannelNewAPIMeta(
		r.Context(),
		channelID,
		&openAIOrganization,
		&testModel,
		&tag,
		&remark,
		weight,
		autoBan,
	); err != nil {
		status := http.StatusInternalServerError
		msg := "保存失败"
		if strings.Contains(err.Error(), "weight") {
			status = http.StatusBadRequest
			msg = err.Error()
		}
		if isAjax(r) {
			ajaxError(w, status, msg)
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape(msg), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "渠道属性已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("渠道属性已保存"), http.StatusFound)
}

func (s *Server) UpdateChannelNewAPISetting(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelID, err := parseInt64(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
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

	proxyRaw := strings.TrimSpace(r.FormValue("proxy"))
	if proxyRaw != "" && !strings.EqualFold(proxyRaw, "direct") && !strings.EqualFold(proxyRaw, "none") {
		u, err := url.Parse(proxyRaw)
		if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "proxy 不合法")
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("proxy 不合法"), http.StatusFound)
			return
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			if isAjax(r) {
				ajaxError(w, http.StatusBadRequest, "proxy scheme 不支持")
				return
			}
			http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("proxy scheme 不支持"), http.StatusFound)
			return
		}
	}

	setting := store.UpstreamChannelSetting{
		ForceFormat:            strings.TrimSpace(r.FormValue("force_format")) == "1",
		ThinkingToContent:      strings.TrimSpace(r.FormValue("thinking_to_content")) == "1",
		PassThroughBodyEnabled: strings.TrimSpace(r.FormValue("pass_through_body_enabled")) == "1",
		Proxy:                  proxyRaw,
		SystemPrompt:           r.FormValue("system_prompt"),
		SystemPromptOverride:   strings.TrimSpace(r.FormValue("system_prompt_override")) == "1",
	}

	if err := s.st.UpdateUpstreamChannelNewAPISetting(r.Context(), channelID, setting); err != nil {
		status := http.StatusInternalServerError
		msg := "保存失败"
		if isAjax(r) {
			ajaxError(w, status, msg)
			return
		}
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape(msg), http.StatusFound)
		return
	}

	if isAjax(r) {
		ajaxOK(w, "渠道额外设置已保存")
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("渠道额外设置已保存"), http.StatusFound)
}
