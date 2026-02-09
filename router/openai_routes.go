package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"realms/internal/middleware"
	"realms/internal/store"
)

func setOpenAIRoutes(r *gin.Engine, opts Options) {
	selfMode := opts.SelfMode

	apiChain := func(h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.TokenAuth(opts.Store),
			middleware.BodyCache(0),
		))
	}
	apiFeatureChain := func(featureKey string, h http.Handler) gin.HandlerFunc {
		return wrapHTTP(middleware.Chain(h,
			middleware.RequestID,
			middleware.AccessLog,
			middleware.FeatureGateEffective(opts.Store, selfMode, featureKey),
			middleware.TokenAuth(opts.Store),
			middleware.BodyCache(0),
		))
	}

	if opts.OpenAI != nil {
		r.POST("/v1/responses", apiChain(http.HandlerFunc(opts.OpenAI.Responses)))
		r.GET("/v1/responses/:response_id", apiChain(http.HandlerFunc(opts.OpenAI.ResponseRetrieve)))
		r.DELETE("/v1/responses/:response_id", apiChain(http.HandlerFunc(opts.OpenAI.ResponseDelete)))
		r.POST("/v1/responses/:response_id/cancel", apiChain(http.HandlerFunc(opts.OpenAI.ResponseCancel)))
		r.GET("/v1/responses/:response_id/input_items", apiChain(http.HandlerFunc(opts.OpenAI.ResponseInputItems)))
		r.POST("/v1/responses/compact", apiChain(http.HandlerFunc(opts.OpenAI.Responses)))
		r.POST("/v1/responses/input_tokens", apiChain(http.HandlerFunc(opts.OpenAI.Responses)))

		r.POST("/v1/chat/completions", apiChain(http.HandlerFunc(opts.OpenAI.ChatCompletions)))
		r.GET("/v1/chat/completions", apiChain(http.HandlerFunc(opts.OpenAI.ChatCompletionsList)))
		r.GET("/v1/chat/completions/:completion_id", apiChain(http.HandlerFunc(opts.OpenAI.ChatCompletionRetrieve)))
		r.POST("/v1/chat/completions/:completion_id", apiChain(http.HandlerFunc(opts.OpenAI.ChatCompletionUpdate)))
		r.DELETE("/v1/chat/completions/:completion_id", apiChain(http.HandlerFunc(opts.OpenAI.ChatCompletionDelete)))
		r.GET("/v1/chat/completions/:completion_id/messages", apiChain(http.HandlerFunc(opts.OpenAI.ChatCompletionMessages)))

		r.POST("/v1/messages", apiChain(http.HandlerFunc(opts.OpenAI.Messages)))
		r.GET("/v1/models", apiFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.OpenAI.Models)))
		r.GET("/v1/models/:model", apiFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.OpenAI.ModelRetrieve)))

		r.GET("/v1beta/models", apiFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.OpenAI.GeminiModels)))
		r.GET("/v1beta/openai/models", apiFeatureChain(store.SettingFeatureDisableModels, http.HandlerFunc(opts.OpenAI.Models)))
		r.POST("/v1beta/models/*path", apiChain(http.HandlerFunc(opts.OpenAI.GeminiProxy)))
	}
}
