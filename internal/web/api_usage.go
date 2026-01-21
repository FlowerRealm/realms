package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/auth"
	"realms/internal/store"
)

type usageWindowAPI struct {
	Window             string          `json:"window"`
	Since              time.Time       `json:"since"`
	Until              time.Time       `json:"until"`
	Requests           int64           `json:"requests"`
	Tokens             int64           `json:"tokens"`
	InputTokens        int64           `json:"input_tokens"`
	OutputTokens       int64           `json:"output_tokens"`
	CachedInputTokens  int64           `json:"cached_input_tokens"`
	CachedOutputTokens int64           `json:"cached_output_tokens"`
	CacheRatio         float64         `json:"cache_ratio"`
	CommittedUSD       decimal.Decimal `json:"committed_usd"`
	ReservedUSD        decimal.Decimal `json:"reserved_usd"`
	LimitUSD           decimal.Decimal `json:"limit_usd"`
	RemainingUSD       decimal.Decimal `json:"remaining_usd"`
}

type subscriptionAPI struct {
	Active   bool      `json:"active"`
	PlanName string    `json:"plan_name,omitempty"`
	StartAt  time.Time `json:"start_at,omitempty"`
	EndAt    time.Time `json:"end_at,omitempty"`
}

type usageWindowsAPIResponse struct {
	Now          time.Time        `json:"now"`
	Subscription subscriptionAPI  `json:"subscription"`
	Windows      []usageWindowAPI `json:"windows"`
}

func (s *Server) APIUsageWindows(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayStr := todayStart.Format("2006-01-02")

	q := r.URL.Query()
	startStr := strings.TrimSpace(q.Get("start"))
	endStr := strings.TrimSpace(q.Get("end"))
	if startStr == "" {
		startStr = todayStr
	}
	if endStr == "" {
		endStr = startStr
	}
	since, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		http.Error(w, "start 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	endDate, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		http.Error(w, "end 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	if since.After(endDate) {
		http.Error(w, "start 不能晚于 end", http.StatusBadRequest)
		return
	}
	if endDate.After(todayStart) {
		endDate = todayStart
		endStr = todayStr
	}
	until := endDate.Add(24 * time.Hour)
	if endStr == todayStr {
		until = now
	}

	subs, err := s.store.ListActiveSubscriptionsWithPlans(r.Context(), p.UserID, now)
	if err != nil {
		http.Error(w, "订阅查询失败", http.StatusInternalServerError)
		return
	}
	var primarySub *store.SubscriptionWithPlan
	if len(subs) > 0 {
		primarySub = &subs[0]
	}

	var resp usageWindowsAPIResponse
	resp.Now = now
	if primarySub != nil {
		resp.Subscription = subscriptionAPI{
			Active:   true,
			PlanName: primarySub.Plan.Name,
			StartAt:  primarySub.Subscription.StartAt,
			EndAt:    primarySub.Subscription.EndAt,
		}
	}

	committed, reserved, err := s.store.SumCommittedAndReservedUSDRange(r.Context(), store.UsageSumWithReservedRangeInput{
		UserID: p.UserID,
		Since:  since,
		Until:  until,
		Now:    now,
	})
	if err != nil {
		http.Error(w, "用量汇总失败", http.StatusInternalServerError)
		return
	}

	tokenStats, err := s.store.GetUsageTokenStatsByUserRange(r.Context(), p.UserID, since, until)
	if err != nil {
		http.Error(w, "Token 统计失败", http.StatusInternalServerError)
		return
	}

	resp.Windows = append(resp.Windows, usageWindowAPI{
		Window:             "range",
		Since:              since,
		Until:              until,
		Requests:           tokenStats.Requests,
		Tokens:             tokenStats.Tokens,
		InputTokens:        tokenStats.InputTokens,
		OutputTokens:       tokenStats.OutputTokens,
		CachedInputTokens:  tokenStats.CachedInputTokens,
		CachedOutputTokens: tokenStats.CachedOutputTokens,
		CacheRatio:         tokenStats.CacheRatio,
		CommittedUSD:       committed,
		ReservedUSD:        reserved,
		LimitUSD:           decimal.Zero,
		RemainingUSD:       decimal.Zero,
	})

	writeJSON(w, resp)
}

type usageEventAPI struct {
	ID                 int64           `json:"id"`
	Time               time.Time       `json:"time"`
	RequestID          string          `json:"request_id"`
	Endpoint           *string         `json:"endpoint,omitempty"`
	Method             *string         `json:"method,omitempty"`
	TokenID            int64           `json:"token_id"`
	UpstreamChannelID  *int64          `json:"upstream_channel_id,omitempty"`
	UpstreamEndpointID *int64          `json:"upstream_endpoint_id,omitempty"`
	UpstreamCredID     *int64          `json:"upstream_credential_id,omitempty"`
	State              string          `json:"state"`
	Model              *string         `json:"model,omitempty"`
	InputTokens        *int64          `json:"input_tokens,omitempty"`
	CachedInputTokens  *int64          `json:"cached_input_tokens,omitempty"`
	OutputTokens       *int64          `json:"output_tokens,omitempty"`
	CachedOutputTokens *int64          `json:"cached_output_tokens,omitempty"`
	ReservedUSD        decimal.Decimal `json:"reserved_usd"`
	CommittedUSD       decimal.Decimal `json:"committed_usd"`
	ReserveExpiresAt   time.Time       `json:"reserve_expires_at"`
	StatusCode         int             `json:"status_code"`
	LatencyMS          int             `json:"latency_ms"`
	ErrorClass         *string         `json:"error_class,omitempty"`
	ErrorMessage       *string         `json:"error_message,omitempty"`
	IsStream           bool            `json:"is_stream"`
	RequestBytes       int64           `json:"request_bytes"`
	ResponseBytes      int64           `json:"response_bytes"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type usageEventsAPIResponse struct {
	Events       []usageEventAPI `json:"events"`
	NextBeforeID *int64          `json:"next_before_id,omitempty"`
}

func (s *Server) APIUsageEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "limit 不合法", http.StatusBadRequest)
			return
		}
		limit = n
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var beforeID *int64
	if v := q.Get("before_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "before_id 不合法", http.StatusBadRequest)
			return
		}
		beforeID = &id
	}

	startStr := strings.TrimSpace(q.Get("start"))
	endStr := strings.TrimSpace(q.Get("end"))
	useRange := startStr != "" || endStr != ""
	var since time.Time
	var until time.Time
	if useRange {
		now := time.Now().UTC()
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		todayStr := todayStart.Format("2006-01-02")

		if startStr == "" {
			startStr = todayStr
		}
		if endStr == "" {
			endStr = startStr
		}
		var err error
		since, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			http.Error(w, "start 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
			return
		}
		endDate, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			http.Error(w, "end 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
			return
		}
		if since.After(endDate) {
			http.Error(w, "start 不能晚于 end", http.StatusBadRequest)
			return
		}
		if endDate.After(todayStart) {
			endDate = todayStart
			endStr = todayStr
		}
		until = endDate.Add(24 * time.Hour)
		if endStr == todayStr {
			until = now
		}
	}

	var events []store.UsageEvent
	var err error
	if useRange {
		events, err = s.store.ListUsageEventsByUserRange(r.Context(), p.UserID, since, until, limit, beforeID, nil)
	} else {
		events, err = s.store.ListUsageEventsByUser(r.Context(), p.UserID, limit, beforeID)
	}
	if err != nil {
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	resp := usageEventsAPIResponse{
		Events: make([]usageEventAPI, 0, len(events)),
	}
	for _, e := range events {
		errClass := e.ErrorClass
		errMsg := e.ErrorMessage
		if errClass != nil && strings.TrimSpace(*errClass) == "client_disconnect" {
			errClass = nil
			errMsg = nil
		}

		resp.Events = append(resp.Events, usageEventAPI{
			ID:                 e.ID,
			Time:               e.Time,
			RequestID:          e.RequestID,
			Endpoint:           e.Endpoint,
			Method:             e.Method,
			TokenID:            e.TokenID,
			UpstreamChannelID:  e.UpstreamChannelID,
			UpstreamEndpointID: e.UpstreamEndpointID,
			UpstreamCredID:     e.UpstreamCredID,
			State:              e.State,
			Model:              e.Model,
			InputTokens:        e.InputTokens,
			CachedInputTokens:  e.CachedInputTokens,
			OutputTokens:       e.OutputTokens,
			CachedOutputTokens: e.CachedOutputTokens,
			ReservedUSD:        e.ReservedUSD,
			CommittedUSD:       e.CommittedUSD,
			ReserveExpiresAt:   e.ReserveExpiresAt,
			StatusCode:         e.StatusCode,
			LatencyMS:          e.LatencyMS,
			ErrorClass:         errClass,
			ErrorMessage:       errMsg,
			IsStream:           e.IsStream,
			RequestBytes:       e.RequestBytes,
			ResponseBytes:      e.ResponseBytes,
			CreatedAt:          e.CreatedAt,
			UpdatedAt:          e.UpdatedAt,
		})
	}
	if len(events) > 0 {
		next := events[len(events)-1].ID
		resp.NextBeforeID = &next
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
