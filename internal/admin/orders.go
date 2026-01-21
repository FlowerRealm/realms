// orders.go 提供订阅订单的管理后台页面与操作入口。
package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"realms/internal/store"
)

type subscriptionOrderView struct {
	ID           int64
	UserEmail    string
	PlanName     string
	GroupName    string
	AmountCNY    string
	Status       int
	StatusText   string
	CreatedAt    string
	PaidAt       string
	ApprovedAt   string
	Subscription string
}

func toSubscriptionOrderView(row store.SubscriptionOrderWithUserAndPlan, loc *time.Location) subscriptionOrderView {
	statusText := "未知"
	switch row.Order.Status {
	case store.SubscriptionOrderStatusPending:
		statusText = "待支付"
	case store.SubscriptionOrderStatusActive:
		statusText = "已生效"
	case store.SubscriptionOrderStatusCanceled:
		statusText = "已取消"
	}
	v := subscriptionOrderView{
		ID:         row.Order.ID,
		UserEmail:  row.UserEmail,
		PlanName:   row.Plan.Name,
		GroupName:  strings.TrimSpace(row.Plan.GroupName),
		AmountCNY:  formatCNYPlain(row.Order.AmountCNY),
		Status:     row.Order.Status,
		StatusText: statusText,
		CreatedAt:  formatTimeIn(row.Order.CreatedAt, "2006-01-02 15:04:05", loc),
	}
	if row.Order.PaidAt != nil {
		v.PaidAt = formatTimeIn(*row.Order.PaidAt, "2006-01-02 15:04:05", loc)
	}
	if row.Order.ApprovedAt != nil {
		v.ApprovedAt = formatTimeIn(*row.Order.ApprovedAt, "2006-01-02 15:04:05", loc)
	}
	if row.Order.SubscriptionID != nil {
		v.Subscription = fmt.Sprintf("%d", *row.Order.SubscriptionID)
	}
	return v
}

func (s *Server) Orders(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	loc, _ := s.adminTimeLocation(r.Context())
	rows, err := s.st.ListRecentSubscriptionOrders(r.Context(), 200)
	if err != nil {
		http.Error(w, "查询订单失败", http.StatusInternalServerError)
		return
	}
	views := make([]subscriptionOrderView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toSubscriptionOrderView(row, loc))
	}

	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	notice := strings.TrimSpace(r.URL.Query().Get("msg"))
	if len(notice) > 200 {
		notice = notice[:200] + "..."
	}

	s.render(w, "admin_orders", s.withFeatures(r.Context(), templateData{
		Title:              "订单 - Realms",
		Error:              errMsg,
		Notice:             notice,
		User:               u,
		IsRoot:             isRoot,
		CSRFToken:          csrf,
		SubscriptionOrders: views,
	}))
}

func (s *Server) ApproveSubscriptionOrder(w http.ResponseWriter, r *http.Request) {
	u, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	orderID, err := parseInt64(strings.TrimSpace(r.PathValue("order_id")))
	if err != nil || orderID <= 0 {
		http.Redirect(w, r, "/admin/orders?err="+url.QueryEscape("参数错误"), http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/orders?err="+url.QueryEscape("表单解析失败"), http.StatusFound)
		return
	}
	subID, err := s.st.ApproveSubscriptionOrderAndDelete(r.Context(), orderID, u.ID, time.Now())
	if err != nil {
		http.Redirect(w, r, "/admin/orders?err="+url.QueryEscape("批准失败："+err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/admin/orders?msg="+url.QueryEscape(fmt.Sprintf("订单 #%d 已批准并生效（subscription_id=%d）。", orderID, subID)), http.StatusFound)
}

func (s *Server) RejectSubscriptionOrder(w http.ResponseWriter, r *http.Request) {
	u, _, _, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	orderID, err := parseInt64(strings.TrimSpace(r.PathValue("order_id")))
	if err != nil || orderID <= 0 {
		http.Redirect(w, r, "/admin/orders?err="+url.QueryEscape("参数错误"), http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/orders?err="+url.QueryEscape("表单解析失败"), http.StatusFound)
		return
	}
	if err := s.st.RejectSubscriptionOrderAndDelete(r.Context(), orderID, u.ID); err != nil {
		http.Redirect(w, r, "/admin/orders?err="+url.QueryEscape("拒绝失败："+err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/admin/orders?msg="+url.QueryEscape(fmt.Sprintf("订单 #%d 已拒绝。", orderID)), http.StatusFound)
}
