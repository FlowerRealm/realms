package admin

import (
	"context"
	"fmt"
	"strings"
)

type PinnedChannelInfo struct {
	Available       bool   `json:"available"`
	PinnedActive    bool   `json:"pinned_active"`
	PinnedChannelID int64  `json:"pinned_channel_id"`
	PinnedChannel   string `json:"pinned_channel"`
	PinnedNote      string `json:"pinned_note,omitempty"`
}

func (s *Server) PinnedChannelInfoForAPI(ctx context.Context) PinnedChannelInfo {
	out := PinnedChannelInfo{Available: s != nil && s.sched != nil}
	if s == nil || s.sched == nil {
		return out
	}

	id, movedAt, reason, ok := s.sched.PinnedChannelInfo()
	if !ok {
		return out
	}
	out.PinnedActive = true
	out.PinnedChannelID = id

	name := ""
	if s.st != nil && id > 0 {
		if ch, err := s.st.GetUpstreamChannelByID(ctx, id); err == nil {
			name = strings.TrimSpace(ch.Name)
		}
	}
	if name != "" {
		out.PinnedChannel = fmt.Sprintf("%s (#%d)", name, id)
	} else {
		out.PinnedChannel = fmt.Sprintf("渠道 #%d", id)
	}

	reasonText := ""
	switch strings.TrimSpace(reason) {
	case "manual":
		reasonText = "手动设置"
	case "ban":
		reasonText = "因封禁轮转"
	case "invalid":
		reasonText = "指针无效修正"
	default:
		reasonText = strings.TrimSpace(reason)
	}
	movedAtText := ""
	if !movedAt.IsZero() {
		loc, _ := s.adminTimeLocation(ctx)
		movedAtText = formatTimeIn(movedAt, "2006-01-02 15:04:05", loc)
	}
	switch {
	case movedAtText != "" && reasonText != "":
		out.PinnedNote = "更新时间：" + movedAtText + "；原因：" + reasonText
	case movedAtText != "":
		out.PinnedNote = "更新时间：" + movedAtText
	case reasonText != "":
		out.PinnedNote = "原因：" + reasonText
	}
	return out
}

func (s *Server) PinChannelForAPI(ctx context.Context, channelID int64) (string, error) {
	if s == nil || s.st == nil {
		return "", fmt.Errorf("server 未初始化")
	}
	if s.sched == nil {
		return "", fmt.Errorf("调度器未初始化")
	}
	if channelID <= 0 {
		return "", fmt.Errorf("channel_id 不合法")
	}

	ch, err := s.st.GetUpstreamChannelByID(ctx, channelID)
	if err != nil || ch.ID <= 0 {
		return "", fmt.Errorf("channel 不存在")
	}

	if err := s.sched.RefreshPinnedRing(ctx, s.st); err != nil {
		return "", fmt.Errorf("构建渠道指针失败：" + err.Error())
	}
	s.sched.PinChannel(ch.ID)
	s.sched.ClearChannelBan(ch.ID)
	return fmt.Sprintf("已将当前渠道指针指向 %s (#%d)", ch.Name, ch.ID), nil
}
