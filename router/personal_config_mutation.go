package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"realms/internal/personalconfig"
)

func beginPersonalConfigMutation(c *gin.Context, opts Options) (*personalconfig.Mutation, bool) {
	if opts.PersonalConfig == nil || !opts.PersonalConfig.Enabled() {
		return nil, true
	}
	m, err := opts.PersonalConfig.BeginMutation(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "personal config 同步失败"})
		return nil, false
	}
	return m, true
}

