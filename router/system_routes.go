package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func setSystemRoutes(r *gin.Engine, opts Options) {
	r.GET("/healthz", wrapHTTPFunc(opts.Healthz))

	r.GET("/api/meta", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"mode": "business",
			},
		})
	})

	r.GET("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))
	r.HEAD("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))

	r.GET("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
	r.HEAD("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
}
