package admin

import (
	"fmt"
	"net/http"
	"net/url"

	"realms/internal/store"
)

func (s *Server) UpdateOpenAICredentialLimits(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	credentialID, err := parseInt64(r.PathValue("credential_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	cred, err := s.st.GetOpenAICompatibleCredentialByID(r.Context(), credentialID)
	if err != nil {
		http.Error(w, "credential 不存在", http.StatusNotFound)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, cred.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	returnTo := safeAdminReturnTo(r.FormValue("return_to"), fmt.Sprintf("/admin/channels/%d/endpoints#keys", ep.ChannelID))

	limitSessions, err := parseOptionalLimitInt(r.FormValue("limit_sessions"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_sessions 不合法"), http.StatusFound)
		return
	}
	limitRPM, err := parseOptionalLimitInt(r.FormValue("limit_rpm"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_rpm 不合法"), http.StatusFound)
		return
	}
	limitTPM, err := parseOptionalLimitInt(r.FormValue("limit_tpm"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_tpm 不合法"), http.StatusFound)
		return
	}

	if err := s.st.UpdateOpenAICompatibleCredentialLimits(r.Context(), cred.ID, limitSessions, limitRPM, limitTPM); err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) UpdateAnthropicCredentialLimits(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	credentialID, err := parseInt64(r.PathValue("credential_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	cred, err := s.st.GetAnthropicCredentialByID(r.Context(), credentialID)
	if err != nil {
		http.Error(w, "credential 不存在", http.StatusNotFound)
		return
	}
	ep, ch, err := s.loadEndpointAndChannel(r, cred.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}
	if ch.Type != store.UpstreamTypeAnthropic {
		http.Error(w, "credential 不匹配 channel 类型", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	returnTo := safeAdminReturnTo(r.FormValue("return_to"), fmt.Sprintf("/admin/channels/%d/endpoints#keys", ep.ChannelID))

	limitSessions, err := parseOptionalLimitInt(r.FormValue("limit_sessions"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_sessions 不合法"), http.StatusFound)
		return
	}
	limitRPM, err := parseOptionalLimitInt(r.FormValue("limit_rpm"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_rpm 不合法"), http.StatusFound)
		return
	}
	limitTPM, err := parseOptionalLimitInt(r.FormValue("limit_tpm"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_tpm 不合法"), http.StatusFound)
		return
	}

	if err := s.st.UpdateAnthropicCredentialLimits(r.Context(), cred.ID, limitSessions, limitRPM, limitTPM); err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) UpdateCodexAccountLimits(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	accountID, err := parseInt64(r.PathValue("account_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	acc, err := s.st.GetCodexOAuthAccountByID(r.Context(), accountID)
	if err != nil {
		http.Error(w, "account 不存在", http.StatusNotFound)
		return
	}
	ep, _, err := s.loadEndpointAndChannel(r, acc.EndpointID)
	if err != nil {
		http.Error(w, "endpoint 不存在", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	returnTo := safeAdminReturnTo(r.FormValue("return_to"), fmt.Sprintf("/admin/channels/%d/endpoints#accounts", ep.ChannelID))

	limitSessions, err := parseOptionalLimitInt(r.FormValue("limit_sessions"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_sessions 不合法"), http.StatusFound)
		return
	}
	limitRPM, err := parseOptionalLimitInt(r.FormValue("limit_rpm"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_rpm 不合法"), http.StatusFound)
		return
	}
	limitTPM, err := parseOptionalLimitInt(r.FormValue("limit_tpm"))
	if err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("limit_tpm 不合法"), http.StatusFound)
		return
	}

	if err := s.st.UpdateCodexOAuthAccountLimits(r.Context(), acc.ID, limitSessions, limitRPM, limitTPM); err != nil {
		http.Redirect(w, r, returnTo+"?err="+url.QueryEscape("保存失败"), http.StatusFound)
		return
	}
	http.Redirect(w, r, returnTo+"?msg="+url.QueryEscape("已保存"), http.StatusFound)
}
