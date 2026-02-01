package router

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type pinnedChannelInfo struct {
	Available       bool   `json:"available"`
	PinnedActive    bool   `json:"pinned_active"`
	PinnedChannelID int64  `json:"pinned_channel_id"`
	PinnedChannel   string `json:"pinned_channel"`
	PinnedNote      string `json:"pinned_note,omitempty"`
}

func pinnedChannelInfoHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		info := pinnedChannelInfo{Available: opts.Sched != nil}
		if opts.Sched == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
			return
		}

		id, movedAt, reason, ok := opts.Sched.PinnedChannelInfo()
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
			return
		}
		info.PinnedActive = true
		info.PinnedChannelID = id

		name := ""
		if opts.Store != nil && id > 0 {
			if ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), id); err == nil {
				name = strings.TrimSpace(ch.Name)
			}
		}
		if name != "" {
			info.PinnedChannel = fmt.Sprintf("%s (#%d)", name, id)
		} else {
			info.PinnedChannel = fmt.Sprintf("渠道 #%d", id)
		}

		info.PinnedNote = pinnedNote(c.Request.Context(), opts, movedAt, reason)

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
	}
}

func pinChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		if opts.Sched == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "调度器未初始化"})
			return
		}

		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}

		ch, err := opts.Store.GetUpstreamChannelByID(c.Request.Context(), channelID)
		if err != nil || ch.ID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel 不存在"})
			return
		}

		if err := opts.Sched.RefreshPinnedRing(c.Request.Context(), opts.Store); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "构建渠道指针失败：" + err.Error()})
			return
		}
		opts.Sched.PinChannel(ch.ID)
		opts.Sched.ClearChannelBan(ch.ID)

		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("已将当前渠道指针指向 %s (#%d)", ch.Name, ch.ID)})
	}
}

func pinnedNote(ctx context.Context, opts Options, movedAt time.Time, reason string) string {
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
		loc, _ := adminTimeLocation(ctx, opts)
		movedAtText = formatTimeIn(movedAt, "2006-01-02 15:04:05", loc)
	}

	switch {
	case movedAtText != "" && reasonText != "":
		return "更新时间：" + movedAtText + "；原因：" + reasonText
	case movedAtText != "":
		return "更新时间：" + movedAtText
	case reasonText != "":
		return "原因：" + reasonText
	default:
		return ""
	}
}
