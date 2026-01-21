package admin

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type subscriptionPlanView struct {
	ID           int64
	Name         string
	GroupName    string
	PriceCNY     string
	DurationDays int
	Status       int
	Limit5H      string
	Limit1D      string
	Limit7D      string
	Limit30D     string
}

func toSubscriptionPlanView(p store.SubscriptionPlan) subscriptionPlanView {
	v := subscriptionPlanView{
		ID:           p.ID,
		Name:         p.Name,
		GroupName:    p.GroupName,
		PriceCNY:     formatCNYPlain(p.PriceCNY),
		DurationDays: p.DurationDays,
		Status:       p.Status,
	}
	if p.Limit5HUSD.GreaterThan(decimal.Zero) {
		v.Limit5H = formatUSDPlain(p.Limit5HUSD)
	}
	if p.Limit1DUSD.GreaterThan(decimal.Zero) {
		v.Limit1D = formatUSDPlain(p.Limit1DUSD)
	}
	if p.Limit7DUSD.GreaterThan(decimal.Zero) {
		v.Limit7D = formatUSDPlain(p.Limit7DUSD)
	}
	if p.Limit30DUSD.GreaterThan(decimal.Zero) {
		v.Limit30D = formatUSDPlain(p.Limit30DUSD)
	}
	return v
}

func newSubscriptionPlanCode() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "sp_" + hex.EncodeToString(buf[:]), nil
}

func (s *Server) Subscriptions(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelGroups, err := s.st.ListChannelGroups(r.Context())
	if err != nil {
		http.Error(w, "查询分组失败", http.StatusInternalServerError)
		return
	}

	plans, err := s.st.ListAllSubscriptionPlans(r.Context())
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}
	views := make([]subscriptionPlanView, 0, len(plans))
	for _, p := range plans {
		views = append(views, toSubscriptionPlanView(p))
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_subscriptions", s.withFeatures(r.Context(), templateData{
		Title:             "订阅套餐 - Realms",
		Error:             errMsg,
		Notice:            notice,
		User:              u,
		IsRoot:            isRoot,
		CSRFToken:         csrf,
		ChannelGroups:     channelGroups,
		SubscriptionPlans: views,
	}))
}

func (s *Server) CreateSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	code := strings.TrimSpace(r.FormValue("code"))
	if code == "" {
		gen, err := newSubscriptionPlanCode()
		if err != nil {
			if isAjax(r) {
				ajaxError(w, http.StatusInternalServerError, "生成套餐标识失败")
				return
			}
			http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("生成套餐标识失败"), http.StatusFound)
			return
		}
		code = gen
	}
	name := strings.TrimSpace(r.FormValue("name"))
	groupName, err := normalizeSingleGroup(r.FormValue("group_name"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "组不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("组不合法"), http.StatusFound)
		return
	}
	if err := validateChannelGroupsSelectable(r.Context(), s.st, groupName); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	priceCNY, err := parseCNY(r.FormValue("price_cny"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "价格不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("价格不合法"), http.StatusFound)
		return
	}

	durationDays, err := parseInt(r.FormValue("duration_days"))
	if err != nil || durationDays <= 0 {
		durationDays = 30
	}
	status, err := parseInt(r.FormValue("status"))
	if err != nil {
		status = 1
	}
	if status != 0 && status != 1 {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "status 不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("status 不合法"), http.StatusFound)
		return
	}

	if name == "" {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "name 不能为空")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("name 不能为空"), http.StatusFound)
		return
	}

	limit5H, err := parseOptionalUSD(r.FormValue("limit_5h"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "5h 限额不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("5h 限额不合法"), http.StatusFound)
		return
	}
	limit1D, err := parseOptionalUSD(r.FormValue("limit_1d"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "1d 限额不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("1d 限额不合法"), http.StatusFound)
		return
	}
	limit7D, err := parseOptionalUSD(r.FormValue("limit_7d"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "7d 限额不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("7d 限额不合法"), http.StatusFound)
		return
	}
	limit30D, err := parseOptionalUSD(r.FormValue("limit_30d"))
	if err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusBadRequest, "30d 限额不合法")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("30d 限额不合法"), http.StatusFound)
		return
	}

	if _, err := s.st.CreateSubscriptionPlan(r.Context(), store.SubscriptionPlanCreate{
		Code:              code,
		Name:              name,
		GroupName:         groupName,
		PriceCNY:          priceCNY,
		Limit5HUSD:        limit5H,
		Limit1DUSD:        limit1D,
		Limit7DUSD:        limit7D,
		Limit30DUSD:       limit30D,
		DurationDays:      durationDays,
		Status:            status,
	}); err != nil {
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "创建失败")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("创建失败"), http.StatusFound)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已创建")
		return
	}
	http.Redirect(w, r, "/admin/subscriptions?msg="+url.QueryEscape("已创建"), http.StatusFound)
}

func (s *Server) SubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelGroups, err := s.st.ListChannelGroups(r.Context())
	if err != nil {
		http.Error(w, "查询分组失败", http.StatusInternalServerError)
		return
	}

	planID, err := parseInt64(r.PathValue("plan_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	p, err := s.st.GetSubscriptionPlanByID(r.Context(), planID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "套餐不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}

	s.render(w, "admin_subscription_edit", s.withFeatures(r.Context(), templateData{
		Title:            "编辑套餐 - Realms",
		Error:            errMsg,
		Notice:           notice,
		User:             u,
		IsRoot:           isRoot,
		CSRFToken:        csrf,
		ChannelGroups:    channelGroups,
		SubscriptionPlan: toSubscriptionPlanView(p),
	}))
}

func (s *Server) UpdateSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	planID, err := parseInt64(r.PathValue("plan_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}

	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	groupName, err := normalizeSingleGroup(r.FormValue("group_name"))
	if err != nil {
		http.Error(w, "组不合法", http.StatusBadRequest)
		return
	}
	if err := validateChannelGroupsSelectable(r.Context(), s.st, groupName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "name 不能为空", http.StatusBadRequest)
		return
	}
	if code == "" {
		p, err := s.st.GetSubscriptionPlanByID(r.Context(), planID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "套餐不存在", http.StatusNotFound)
				return
			}
			http.Error(w, "查询失败", http.StatusInternalServerError)
			return
		}
		code = p.Code
	}

	priceCNY, err := parseCNY(r.FormValue("price_cny"))
	if err != nil {
		http.Error(w, "价格不合法", http.StatusBadRequest)
		return
	}

	durationDays, err := parseInt(r.FormValue("duration_days"))
	if err != nil || durationDays <= 0 {
		http.Error(w, "duration_days 不合法", http.StatusBadRequest)
		return
	}

	status, err := parseInt(r.FormValue("status"))
	if err != nil || (status != 0 && status != 1) {
		http.Error(w, "status 不合法", http.StatusBadRequest)
		return
	}

	limit5H, err := parseOptionalUSD(r.FormValue("limit_5h"))
	if err != nil {
		http.Error(w, "5h 限额不合法", http.StatusBadRequest)
		return
	}
	limit1D, err := parseOptionalUSD(r.FormValue("limit_1d"))
	if err != nil {
		http.Error(w, "1d 限额不合法", http.StatusBadRequest)
		return
	}
	limit7D, err := parseOptionalUSD(r.FormValue("limit_7d"))
	if err != nil {
		http.Error(w, "7d 限额不合法", http.StatusBadRequest)
		return
	}
	limit30D, err := parseOptionalUSD(r.FormValue("limit_30d"))
	if err != nil {
		http.Error(w, "30d 限额不合法", http.StatusBadRequest)
		return
	}

	if err := s.st.UpdateSubscriptionPlan(r.Context(), store.SubscriptionPlanUpdate{
		ID:                planID,
		Code:              code,
		Name:              name,
		GroupName:         groupName,
		PriceCNY:          priceCNY,
		Limit5HUSD:        limit5H,
		Limit1DUSD:        limit1D,
		Limit7DUSD:        limit7D,
		Limit30DUSD:       limit30D,
		DurationDays:      durationDays,
		Status:            status,
	}); err != nil {
		http.Error(w, "更新失败", http.StatusInternalServerError)
		return
	}
	if isAjax(r) {
		ajaxOK(w, "已保存")
		return
	}
	http.Redirect(w, r, "/admin/subscriptions/"+fmt.Sprintf("%d", planID)+"?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

func (s *Server) DeleteSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	planID, err := parseInt64(r.PathValue("plan_id"))
	if err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}

	sum, err := s.st.DeleteSubscriptionPlan(r.Context(), planID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "套餐不存在", http.StatusNotFound)
			return
		}
		if isAjax(r) {
			ajaxError(w, http.StatusInternalServerError, "删除失败")
			return
		}
		http.Redirect(w, r, "/admin/subscriptions?err="+url.QueryEscape("删除失败"), http.StatusFound)
		return
	}

	msg := "已删除"
	if sum.OrdersDeleted > 0 || sum.SubscriptionsDeleted > 0 || sum.UsageEventsUnbound > 0 {
		msg += "（已清理"
		if sum.SubscriptionsDeleted > 0 {
			msg += " subscriptions=" + strconv.FormatInt(sum.SubscriptionsDeleted, 10)
		}
		if sum.OrdersDeleted > 0 {
			if sum.SubscriptionsDeleted > 0 {
				msg += ","
			}
			msg += " orders=" + strconv.FormatInt(sum.OrdersDeleted, 10)
		}
		if sum.UsageEventsUnbound > 0 {
			if sum.SubscriptionsDeleted > 0 || sum.OrdersDeleted > 0 {
				msg += ","
			}
			msg += " usage_events=" + strconv.FormatInt(sum.UsageEventsUnbound, 10)
		}
		msg += "）"
	}

	if isAjax(r) {
		ajaxOK(w, msg)
		return
	}
	http.Redirect(w, r, "/admin/subscriptions?msg="+url.QueryEscape(msg), http.StatusFound)
}
