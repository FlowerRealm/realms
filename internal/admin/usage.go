package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

type usageWindowView struct {
	Window       string
	Since        string
	Until        string
	Requests     int64
	Tokens       int64
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
	CacheRatio   string
	RPM          string
	TPM          string
	CommittedUSD string
	ReservedUSD  string
	TotalUSD     string
}

type usageUserView struct {
	UserID       int64
	Email        string
	Role         string
	Status       int
	CommittedUSD string
	ReservedUSD  string
}

type usageEventView struct {
	ID                int64
	Time              string
	UserID            int64
	UserEmail         string
	Endpoint          string
	Method            string
	Model             string
	StatusCode        string
	LatencyMS         string
	InputTokens       string
	OutputTokens      string
	CachedTokens      string
	RequestBytes      string
	ResponseBytes     string
	CostUSD           string
	StateLabel        string
	StateBadgeClass   string
	IsStream          bool
	UpstreamChannelID string
	RequestID         string
	Error             string
	ErrorClass        string
	ErrorMessage      string
}

func (s *Server) Usage(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !isRoot {
		http.Error(w, "无权限", http.StatusForbidden)
		return
	}

	loc, tzName := s.adminTimeLocation(r.Context())

	nowUTC := time.Now().UTC()
	now := nowUTC.In(loc)
	todayStartLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	todayStr := todayStartLocal.Format("2006-01-02")

	q := r.URL.Query()
	startStr := strings.TrimSpace(q.Get("start"))
	endStr := strings.TrimSpace(q.Get("end"))
	limit := 50
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := parseInt64(v)
		if err != nil {
			http.Error(w, "limit 不合法", http.StatusBadRequest)
			return
		}
		limit = int(n)
	}
	if limit < 10 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	if startStr == "" {
		startStr = todayStr
	}
	if endStr == "" {
		endStr = startStr
	}
	sinceLocal, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		http.Error(w, "start 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	endDateLocal, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		http.Error(w, "end 不合法（格式：YYYY-MM-DD）", http.StatusBadRequest)
		return
	}
	if sinceLocal.After(endDateLocal) {
		http.Error(w, "start 不能晚于 end", http.StatusBadRequest)
		return
	}
	if endDateLocal.After(todayStartLocal) {
		endDateLocal = todayStartLocal
		endStr = todayStr
	}
	untilLocal := endDateLocal.AddDate(0, 0, 1)
	if endStr == todayStr {
		untilLocal = now
	}

	since := sinceLocal.UTC()
	until := untilLocal.UTC()

	committed, reserved, err := s.st.SumCommittedAndReservedUSDAllRange(r.Context(), store.UsageSumAllWithReservedRangeInput{
		Since: since,
		Until: until,
		Now:   nowUTC,
	})
	if err != nil {
		http.Error(w, "用量汇总失败", http.StatusInternalServerError)
		return
	}

	stats, err := s.st.GetGlobalUsageStatsRange(r.Context(), since, until)
	if err != nil {
		http.Error(w, "统计失败", http.StatusInternalServerError)
		return
	}

	minutes := until.Sub(since).Minutes()
	if minutes < 1 {
		minutes = 1
	}
	rpm := float64(stats.Requests) / minutes
	tpm := float64(stats.Tokens) / minutes

	view := usageWindowView{
		Window:       "统计区间",
		Since:        sinceLocal.Format("2006-01-02 15:04"),
		Until:        untilLocal.Format("2006-01-02 15:04"),
		Requests:     stats.Requests,
		Tokens:       stats.Tokens,
		InputTokens:  stats.InputTokens,
		OutputTokens: stats.OutputTokens,
		CachedTokens: stats.CachedInputTokens + stats.CachedOutputTokens,
		CacheRatio:   fmt.Sprintf("%.1f%%", stats.CacheRatio*100),
		RPM:          fmt.Sprintf("%.1f", rpm),
		TPM:          fmt.Sprintf("%.1f", tpm),
		CommittedUSD: formatUSDPlain(committed),
		ReservedUSD:  formatUSDPlain(reserved),
		TotalUSD:     formatUSDPlain(committed.Add(reserved)),
	}

	topUsers, err := s.st.ListUsageTopUsers(r.Context(), store.UsageTopUsersInput{
		Since: since,
		Until: until,
		Now:   now,
		Limit: 50,
	})
	if err != nil {
		http.Error(w, "用户用量汇总失败", http.StatusInternalServerError)
		return
	}
	var topViews []usageUserView
	for _, row := range topUsers {
		topViews = append(topViews, usageUserView{
			UserID:       row.UserID,
			Email:        row.Email,
			Role:         row.Role,
			Status:       row.Status,
			CommittedUSD: formatUSDPlain(row.CommittedUSD),
			ReservedUSD:  formatUSDPlain(row.ReservedUSD),
		})
	}

	var beforeID *int64
	if v := strings.TrimSpace(q.Get("before_id")); v != "" {
		id, err := parseInt64(v)
		if err != nil || id <= 0 {
			http.Error(w, "before_id 不合法", http.StatusBadRequest)
			return
		}
		beforeID = &id
	}
	var afterID *int64
	if v := strings.TrimSpace(q.Get("after_id")); v != "" {
		id, err := parseInt64(v)
		if err != nil || id <= 0 {
			http.Error(w, "after_id 不合法", http.StatusBadRequest)
			return
		}
		afterID = &id
	}
	if beforeID != nil && afterID != nil {
		http.Error(w, "before_id 与 after_id 不能同时使用", http.StatusBadRequest)
		return
	}

	events, err := s.st.ListUsageEventsWithUserRange(r.Context(), since, until, limit, beforeID, afterID)
	if err != nil {
		http.Error(w, "查询请求明细失败", http.StatusInternalServerError)
		return
	}
	var eventViews []usageEventView
	for _, row := range events {
		e := row.Event
		endpoint := "-"
		if e.Endpoint != nil && strings.TrimSpace(*e.Endpoint) != "" {
			endpoint = *e.Endpoint
		}
		method := "-"
		if e.Method != nil && strings.TrimSpace(*e.Method) != "" {
			method = *e.Method
		}
		model := "-"
		if e.Model != nil && strings.TrimSpace(*e.Model) != "" {
			model = *e.Model
		}
		statusCode := "-"
		if e.StatusCode > 0 {
			statusCode = fmt.Sprintf("%d", e.StatusCode)
		}
		latencyMS := "-"
		if e.LatencyMS > 0 {
			latencyMS = fmt.Sprintf("%d", e.LatencyMS)
		}
		inTok := "-"
		if e.InputTokens != nil {
			inTok = fmt.Sprintf("%d", *e.InputTokens)
		}
		outTok := "-"
		if e.OutputTokens != nil {
			outTok = fmt.Sprintf("%d", *e.OutputTokens)
		}
		var cached int64
		if e.CachedInputTokens != nil {
			cached += *e.CachedInputTokens
		}
		if e.CachedOutputTokens != nil {
			cached += *e.CachedOutputTokens
		}
		cachedTok := "-"
		if cached > 0 {
			cachedTok = fmt.Sprintf("%d", cached)
		}
		reqBytes := fmt.Sprintf("%d", e.RequestBytes)
		respBytes := fmt.Sprintf("%d", e.ResponseBytes)
		costUSD := decimal.Zero
		switch e.State {
		case store.UsageStateCommitted:
			costUSD = e.CommittedUSD
		case store.UsageStateReserved:
			costUSD = e.ReservedUSD
		}
		cost := formatUSDPlain(costUSD)
		if e.State == store.UsageStateReserved {
			cost += " (预留)"
		}
		stateLabel := e.State
		stateBadge := "bg-secondary-subtle text-secondary border border-secondary-subtle"
		switch e.State {
		case store.UsageStateCommitted:
			stateLabel = "已结算"
			stateBadge = "bg-success-subtle text-success border border-success-subtle"
		case store.UsageStateReserved:
			stateLabel = "预留中"
			stateBadge = "bg-warning-subtle text-warning border border-warning-subtle"
		case store.UsageStateVoid:
			stateLabel = "已作废"
			stateBadge = "bg-secondary-subtle text-secondary border border-secondary-subtle"
		case store.UsageStateExpired:
			stateLabel = "已过期"
			stateBadge = "bg-secondary-subtle text-secondary border border-secondary-subtle"
		}
		upstreamChannelID := "-"
		if e.UpstreamChannelID != nil && *e.UpstreamChannelID > 0 {
			upstreamChannelID = fmt.Sprintf("%d", *e.UpstreamChannelID)
		}
		errClass := ""
		if e.ErrorClass != nil && strings.TrimSpace(*e.ErrorClass) != "" {
			errClass = strings.TrimSpace(*e.ErrorClass)
		}
		errMsg := ""
		if e.ErrorMessage != nil && strings.TrimSpace(*e.ErrorMessage) != "" {
			errMsg = strings.TrimSpace(*e.ErrorMessage)
		}
		if errClass == "client_disconnect" {
			errClass = ""
			errMsg = ""
		}
		errText := ""
		if errClass != "" {
			errText = errClass
		}
		if errMsg != "" {
			if errText == "" {
				errText = errMsg
			} else {
				errText = errText + " (" + errMsg + ")"
			}
		}

		eventViews = append(eventViews, usageEventView{
			ID:                e.ID,
			Time:              formatTimeIn(e.Time, "2006-01-02 15:04:05", loc),
			UserID:            e.UserID,
			UserEmail:         row.UserEmail,
			Endpoint:          endpoint,
			Method:            method,
			Model:             model,
			StatusCode:        statusCode,
			LatencyMS:         latencyMS,
			InputTokens:       inTok,
			OutputTokens:      outTok,
			CachedTokens:      cachedTok,
			RequestBytes:      reqBytes,
			ResponseBytes:     respBytes,
			CostUSD:           cost,
			StateLabel:        stateLabel,
			StateBadgeClass:   stateBadge,
			IsStream:          e.IsStream,
			UpstreamChannelID: upstreamChannelID,
			RequestID:         e.RequestID,
			Error:             errText,
			ErrorClass:        errClass,
			ErrorMessage:      errMsg,
		})
	}
	nextBeforeID := ""
	if len(events) == limit {
		next := events[len(events)-1].Event.ID
		nextBeforeID = fmt.Sprintf("%d", next)
	}
	prevAfterID := ""
	if len(events) > 0 {
		canPrev := beforeID != nil || (afterID != nil && len(events) == limit)
		if canPrev {
			prev := events[0].Event.ID
			prevAfterID = fmt.Sprintf("%d", prev)
		}
	}
	cursorActive := beforeID != nil || afterID != nil

	s.render(w, "admin_usage", s.withFeatures(r.Context(), templateData{
		Title:                  "全站用量 - Realms",
		User:                   u,
		IsRoot:                 isRoot,
		CSRFToken:              csrf,
		AdminTimeZoneEffective: tzName,
		UsageNow:               now.Format("2006-01-02 15:04"),
		UsageStart:             startStr,
		UsageEnd:               endStr,
		UsageWins:              []usageWindowView{view},
		UsageTop:               topViews,
		UsageEvents:            eventViews,
		UsageNextBeforeID:      nextBeforeID,
		UsagePrevAfterID:       prevAfterID,
		UsageCursorActive:      cursorActive,
		UsageLimit:             limit,
	}))
}
