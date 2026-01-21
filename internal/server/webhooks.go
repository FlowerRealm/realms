package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"realms/internal/store"
)

type subscriptionOrderPaidWebhookRequest struct {
	// PaidAt 可选；未提供则使用当前时间。
	PaidAt *time.Time `json:"paid_at,omitempty"`
}

type subscriptionOrderPaidWebhookResponse struct {
	OK             bool   `json:"ok"`
	Processed      bool   `json:"processed"`
	SubscriptionID *int64 `json:"subscription_id,omitempty"`
}

func (a *App) handleSubscriptionOrderPaidWebhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(a.cfg.Security.SubscriptionOrderWebhookSecret)
	if secret == "" {
		http.NotFound(w, r)
		return
	}

	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authz, bearerPrefix) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authz, bearerPrefix))
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}

	orderIDRaw := strings.TrimSpace(r.PathValue("order_id"))
	orderID, err := strconv.ParseInt(orderIDRaw, 10, 64)
	if err != nil || orderID <= 0 {
		http.Error(w, "order_id 不合法", http.StatusBadRequest)
		return
	}

	var req subscriptionOrderPaidWebhookRequest
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "JSON 不合法", http.StatusBadRequest)
			return
		}
	}

	paidAt := time.Now()
	if req.PaidAt != nil && !req.PaidAt.IsZero() {
		paidAt = *req.PaidAt
	}

	subID, processed, err := a.store.MarkSubscriptionOrderPaidAndActivateAndDelete(r.Context(), orderID, paidAt)
	if err != nil {
		if !errors.Is(err, store.ErrOrderCanceled) {
			http.Error(w, "处理失败", http.StatusInternalServerError)
			return
		}
		processed = true
		subID = 0
	}

	resp := subscriptionOrderPaidWebhookResponse{
		OK:        true,
		Processed: processed,
	}
	if processed && subID > 0 {
		id := subID
		resp.SubscriptionID = &id
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
}
