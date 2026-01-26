package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/middleware"
	"realms/internal/store"
)

func normalizeAdminNote(raw string, maxRunes int) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if maxRunes > 0 {
		r := []rune(s)
		if len(r) > maxRunes {
			s = string(r[:maxRunes])
		}
	}
	return s
}

func (s *Server) AddUserBalance(w http.ResponseWriter, r *http.Request) {
	actor, _, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}

	userID, err := parseInt64(r.PathValue("user_id"))
	if err != nil || userID <= 0 {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	target, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		http.Error(w, "用户不存在", http.StatusNotFound)
		return
	}

	amountUSD, err := parseUSD(r.FormValue("amount_usd"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	if amountUSD.LessThanOrEqual(decimal.Zero) {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "金额必须大于 0")
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("金额必须大于 0"), http.StatusFound)
		return
	}

	note := normalizeAdminNote(r.FormValue("note"), 200)

	start := time.Now()
	newBal, err := s.st.AddUserBalanceUSD(ctx, target.ID, amountUSD)
	latency := time.Since(start)
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape("入账失败："+err.Error()), http.StatusFound)
		return
	}

	deltaStr := formatUSDPlain(amountUSD)
	newBalStr := formatUSDPlain(newBal)
	auditMsg := fmt.Sprintf("target_user_id=%d delta_usd=%s new_balance_usd=%s", target.ID, deltaStr, newBalStr)
	if note != "" {
		auditMsg += " note=" + note
	}
	auditMsgPtr := &auditMsg

	actorID := actor.ID
	action := "admin.user_balance.add"
	endpoint := r.URL.Path
	_ = s.st.InsertAuditEvent(ctx, store.AuditEventInput{
		RequestID:    middleware.GetRequestID(ctx),
		ActorType:    "session",
		UserID:       &actorID,
		TokenID:      nil,
		Action:       action,
		Endpoint:     endpoint,
		Model:        nil,
		StatusCode:   http.StatusOK,
		LatencyMS:    int(latency.Milliseconds()),
		ErrorClass:   nil,
		ErrorMessage: auditMsgPtr,
	})

	msg := fmt.Sprintf("已为 %s 增加 %s USD（当前余额 %s USD）", target.Email, deltaStr, newBalStr)
	if isAjax(r) {
		ajaxOK(w, msg)
		return
	}
	http.Redirect(w, r, "/admin/users?msg="+url.QueryEscape(msg), http.StatusFound)
}
