package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"realms/internal/store"
)

type adminChannelKeyUsageView struct {
	ID           int64
	Name         string
	Key          string
	Requests     int64
	Success      int64
	Failure      int64
	InputTokens  int64
	OutputTokens int64
	LastSeen     string
	Balance      string
	BalanceHint  string
}

func parseChannelUsageWindow(raw string) (time.Duration, string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		v = "1h"
	}
	switch v {
	case "5m":
		return 5 * time.Minute, v, nil
	case "1h":
		return time.Hour, v, nil
	case "24h":
		return 24 * time.Hour, v, nil
	default:
		return 0, "", fmt.Errorf("window 不合法")
	}
}

func maskKeyHint(hintPtr *string) string {
	if hintPtr == nil {
		return "-"
	}
	hint := strings.TrimSpace(*hintPtr)
	if hint == "" {
		return "-"
	}
	if len(hint) > 4 {
		return "..." + hint[len(hint)-4:]
	}
	return hint
}

func (s *Server) ChannelDetail(w http.ResponseWriter, r *http.Request) {
	u, csrf, isRoot, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	channelID, err := parseInt64(strings.TrimSpace(r.PathValue("channel_id")))
	if err != nil || channelID <= 0 {
		http.Error(w, "channel_id 不合法", http.StatusBadRequest)
		return
	}

	dur, window, err := parseChannelUsageWindow(r.URL.Query().Get("window"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := s.st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "channel 不存在", http.StatusNotFound)
			return
		}
		http.Error(w, "查询失败", http.StatusInternalServerError)
		return
	}

	ep, err := s.st.GetUpstreamEndpointByChannelID(ctx, channelID)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		ep, err = s.st.SetUpstreamEndpointBaseURL(ctx, channelID, defaultEndpointBaseURL(ch.Type))
	}
	if err != nil {
		http.Error(w, "查询 endpoint 失败", http.StatusInternalServerError)
		return
	}

	loc, tzName := s.adminTimeLocation(ctx)
	nowUTC := time.Now().UTC()
	since := nowUTC.Add(-dur)
	until := nowUTC

	rawStats, err := s.st.GetUsageStatsByCredentialForChannelRange(ctx, channelID, since, until)
	if err != nil {
		http.Error(w, "查询用量失败", http.StatusInternalServerError)
		return
	}
	statsByID := make(map[int64]store.CredentialUsageStats, len(rawStats))
	for _, st := range rawStats {
		statsByID[st.CredentialID] = st
	}

	var rows []adminChannelKeyUsageView
	switch ch.Type {
	case store.UpstreamTypeCodexOAuth:
		accs, err := s.st.ListCodexOAuthAccountsByEndpoint(ctx, ep.ID)
		if err != nil {
			http.Error(w, "查询账号失败", http.StatusInternalServerError)
			return
		}
		rows = make([]adminChannelKeyUsageView, 0, len(accs))
		for _, a := range accs {
			name := "-"
			if a.Email != nil && strings.TrimSpace(*a.Email) != "" {
				name = strings.TrimSpace(*a.Email)
			}
			key := strings.TrimSpace(a.AccountID)
			if key == "" {
				key = "-"
			}
			v := adminChannelKeyUsageView{
				ID:   a.ID,
				Name: name,
				Key:  key,

				LastSeen: "-",
				Balance:  "-",
			}
			if st, ok := statsByID[a.ID]; ok {
				v.Requests = st.Requests
				v.Success = st.Success
				v.Failure = st.Failure
				v.InputTokens = st.InputTokens
				v.OutputTokens = st.OutputTokens
				if !st.LastSeenAt.IsZero() {
					v.LastSeen = formatTimeIn(st.LastSeenAt, "2006-01-02 15:04:05", loc)
				}
			}
			if a.BalanceTotalAvailableUSD != nil {
				v.Balance = formatUSDPlain(*a.BalanceTotalAvailableUSD) + " USD"
			}
			var hints []string
			if a.BalanceUpdatedAt != nil {
				hints = append(hints, "更新："+formatTimeIn(*a.BalanceUpdatedAt, "2006-01-02 15:04:05", loc))
			}
			if a.BalanceError != nil && strings.TrimSpace(*a.BalanceError) != "" {
				hints = append(hints, "错误："+strings.TrimSpace(*a.BalanceError))
				if v.Balance == "-" {
					v.Balance = "查询失败"
				}
			}
			if len(hints) > 0 {
				v.BalanceHint = strings.Join(hints, "\n")
			}
			rows = append(rows, v)
		}
	case store.UpstreamTypeAnthropic:
		creds, err := s.st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			http.Error(w, "查询密钥失败", http.StatusInternalServerError)
			return
		}
		rows = make([]adminChannelKeyUsageView, 0, len(creds))
		for _, c := range creds {
			name := "未命名"
			if c.Name != nil && strings.TrimSpace(*c.Name) != "" {
				name = strings.TrimSpace(*c.Name)
			}
			v := adminChannelKeyUsageView{
				ID:   c.ID,
				Name: name,
				Key:  maskKeyHint(c.APIKeyHint),

				LastSeen: "-",
				Balance:  "暂不支持",
			}
			if st, ok := statsByID[c.ID]; ok {
				v.Requests = st.Requests
				v.Success = st.Success
				v.Failure = st.Failure
				v.InputTokens = st.InputTokens
				v.OutputTokens = st.OutputTokens
				if !st.LastSeenAt.IsZero() {
					v.LastSeen = formatTimeIn(st.LastSeenAt, "2006-01-02 15:04:05", loc)
				}
			}
			rows = append(rows, v)
		}
	default:
		creds, err := s.st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
		if err != nil {
			http.Error(w, "查询密钥失败", http.StatusInternalServerError)
			return
		}
		rows = make([]adminChannelKeyUsageView, 0, len(creds))
		for _, c := range creds {
			name := "未命名"
			if c.Name != nil && strings.TrimSpace(*c.Name) != "" {
				name = strings.TrimSpace(*c.Name)
			}
			v := adminChannelKeyUsageView{
				ID:   c.ID,
				Name: name,
				Key:  maskKeyHint(c.APIKeyHint),

				LastSeen: "-",
				Balance:  "暂不支持",
			}
			if st, ok := statsByID[c.ID]; ok {
				v.Requests = st.Requests
				v.Success = st.Success
				v.Failure = st.Failure
				v.InputTokens = st.InputTokens
				v.OutputTokens = st.OutputTokens
				if !st.LastSeenAt.IsZero() {
					v.LastSeen = formatTimeIn(st.LastSeenAt, "2006-01-02 15:04:05", loc)
				}
			}
			rows = append(rows, v)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Requests != rows[j].Requests {
			return rows[i].Requests > rows[j].Requests
		}
		if rows[i].LastSeen != rows[j].LastSeen {
			return rows[i].LastSeen > rows[j].LastSeen
		}
		return rows[i].ID > rows[j].ID
	})

	s.render(w, "admin_channel_detail", s.withFeatures(ctx, templateData{
		Title:                  fmt.Sprintf("渠道详情 - %s - Realms", ch.Name),
		User:                   u,
		IsRoot:                 isRoot,
		CSRFToken:              csrf,
		AdminTimeZoneEffective: tzName,
		Channel:                ch,
		Endpoint:               ep,
		ChannelUsageWindow:     window,
		ChannelUsageSince:      formatTimeIn(since, "2006-01-02 15:04:05", loc),
		ChannelUsageUntil:      formatTimeIn(until, "2006-01-02 15:04:05", loc),
		ChannelKeyUsage:        rows,
	}))
}
