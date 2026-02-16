package router

import "github.com/gin-gonic/gin"

func setSystemRoutes(r *gin.Engine, opts Options) {
	r.GET("/healthz", wrapHTTPFunc(opts.Healthz))

	r.GET("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))
	r.HEAD("/assets/realms_icon.svg", wrapHTTPFunc(opts.RealmsIconSVG))

	r.GET("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
	r.HEAD("/favicon.ico", wrapHTTPFunc(opts.FaviconICO))
}
