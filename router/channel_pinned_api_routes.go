package router

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func pinnedChannelInfoHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Admin == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"available": false}})
			return
		}
		info := opts.Admin.PinnedChannelInfoForAPI(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": info})
	}
}

func pinChannelHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Admin == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "admin 未初始化"})
			return
		}

		channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("channel_id")), 10, 64)
		if err != nil || channelID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel_id 不合法"})
			return
		}

		msg, err := opts.Admin.PinChannelForAPI(c.Request.Context(), channelID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
	}
}
