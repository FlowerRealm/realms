package openai

import (
	"encoding/json"
	"errors"
	"strings"
)

var errInvalidJSON = errors.New("invalid json")

// sanitizeResponsesPayload 用于解析 /v1/responses 请求体，并做最小的结构校验。
//
// 注意：这里不做字段白名单过滤，也不做 tokens 字段别名/补齐，避免“中转改写导致上游校验失败”。
func sanitizeResponsesPayload(body []byte) (map[string]any, error) {
	out := make(map[string]any)
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, errInvalidJSON
	}

	model, _ := out["model"].(string)
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("model 不能为空")
	}
	if _, ok := out["input"]; !ok || out["input"] == nil {
		return nil, errors.New("input 不能为空")
	}
	return out, nil
}

// sanitizeMessagesPayload 参考 new-api 的 ClaudeRequest：仅保留已写定字段（未知字段静默丢弃），并做 tokens 别名/补齐。
func sanitizeMessagesPayload(body []byte, defaultMaxTokens int) (map[string]any, error) {
	type thinking struct {
		Type         string `json:"type,omitempty"`
		BudgetTokens *int64 `json:"budget_tokens,omitempty"`
	}
	type req struct {
		Model    string `json:"model,omitempty"`
		Prompt   string `json:"prompt,omitempty"`
		System   any    `json:"system,omitempty"`
		Messages any    `json:"messages,omitempty"`

		MaxTokens       *int64 `json:"max_tokens,omitempty"`
		MaxTokensSample *int64 `json:"max_tokens_to_sample,omitempty"`
		MaxOutputTokens *int64 `json:"max_output_tokens,omitempty"`
		MaxCompletion   *int64 `json:"max_completion_tokens,omitempty"`

		StopSequences json.RawMessage `json:"stop_sequences,omitempty"`
		Temperature   json.RawMessage `json:"temperature,omitempty"`
		TopP          json.RawMessage `json:"top_p,omitempty"`
		TopK          json.RawMessage `json:"top_k,omitempty"`
		Stream        bool            `json:"stream,omitempty"`
		Tools         any             `json:"tools,omitempty"`

		ContextManagement json.RawMessage `json:"context_management,omitempty"`
		OutputConfig      json.RawMessage `json:"output_config,omitempty"`
		OutputFormat      json.RawMessage `json:"output_format,omitempty"`
		Container         json.RawMessage `json:"container,omitempty"`
		ToolChoice        any             `json:"tool_choice,omitempty"`
		Thinking          *thinking       `json:"thinking,omitempty"`
		McpServers        json.RawMessage `json:"mcp_servers,omitempty"`
		Metadata          json.RawMessage `json:"metadata,omitempty"`
		ServiceTier       string          `json:"service_tier,omitempty"`
		Store             json.RawMessage `json:"store,omitempty"`
		SafetyIdentifier  string          `json:"safety_identifier,omitempty"`
	}

	var r req
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, errInvalidJSON
	}

	if strings.TrimSpace(r.Model) == "" {
		return nil, errors.New("model 不能为空")
	}
	if r.Messages == nil {
		return nil, errors.New("messages 不能为空")
	}

	// max_output_tokens/max_completion_tokens -> max_tokens
	switch {
	case r.MaxTokens != nil:
		r.MaxOutputTokens = nil
		r.MaxCompletion = nil
	case r.MaxOutputTokens != nil:
		r.MaxTokens = r.MaxOutputTokens
		r.MaxOutputTokens = nil
		r.MaxCompletion = nil
	case r.MaxCompletion != nil:
		r.MaxTokens = r.MaxCompletion
		r.MaxOutputTokens = nil
		r.MaxCompletion = nil
	}

	if r.MaxTokens == nil && defaultMaxTokens > 0 {
		v := int64(defaultMaxTokens)
		r.MaxTokens = &v
	}
	if r.MaxTokens == nil || *r.MaxTokens <= 0 {
		return nil, errors.New("max_tokens 不能为空")
	}

	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errInvalidJSON
	}
	return out, nil
}

// sanitizeChatCompletionsPayload 参考 new-api 的 GeneralOpenAIRequest：仅保留已写定字段（未知字段静默丢弃），并做补齐。
func sanitizeChatCompletionsPayload(body []byte, defaultMaxTokens int) (map[string]any, error) {
	type streamOptions struct {
		IncludeUsage bool `json:"include_usage,omitempty"`
	}
	type responseFormat struct {
		Type       string          `json:"type,omitempty"`
		JsonSchema json.RawMessage `json:"json_schema,omitempty"`
	}
	type webSearchOptions struct {
		SearchContextSize string          `json:"search_context_size,omitempty"`
		UserLocation      json.RawMessage `json:"user_location,omitempty"`
	}
	type message struct {
		Role       string          `json:"role"`
		Content    any             `json:"content"`
		Name       *string         `json:"name,omitempty"`
		Prefix     *bool           `json:"prefix,omitempty"`
		ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
	}
	type req struct {
		Model    string    `json:"model,omitempty"`
		Messages []message `json:"messages,omitempty"`

		Prompt any `json:"prompt,omitempty"`
		Prefix any `json:"prefix,omitempty"`
		Suffix any `json:"suffix,omitempty"`

		Stream        bool           `json:"stream,omitempty"`
		StreamOptions *streamOptions `json:"stream_options,omitempty"`

		MaxTokens           *int64          `json:"max_tokens,omitempty"`
		MaxOutputTokens     *int64          `json:"max_output_tokens,omitempty"`
		MaxCompletionTokens *int64          `json:"max_completion_tokens,omitempty"`
		ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
		Verbosity           json.RawMessage `json:"verbosity,omitempty"`

		Temperature *float64 `json:"temperature,omitempty"`
		TopP        float64  `json:"top_p,omitempty"`
		TopK        int      `json:"top_k,omitempty"`
		Stop        any      `json:"stop,omitempty"`
		N           int      `json:"n,omitempty"`

		Input       any    `json:"input,omitempty"`
		Instruction string `json:"instruction,omitempty"`
		Size        string `json:"size,omitempty"`

		Functions        json.RawMessage `json:"functions,omitempty"`
		FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
		PresencePenalty  float64         `json:"presence_penalty,omitempty"`

		ResponseFormat *responseFormat `json:"response_format,omitempty"`
		EncodingFormat json.RawMessage `json:"encoding_format,omitempty"`
		Seed           float64         `json:"seed,omitempty"`

		ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
		Tools             json.RawMessage `json:"tools,omitempty"`
		ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`

		User        string `json:"user,omitempty"`
		LogProbs    bool   `json:"logprobs,omitempty"`
		TopLogProbs int    `json:"top_logprobs,omitempty"`

		Dimensions int             `json:"dimensions,omitempty"`
		Modalities json.RawMessage `json:"modalities,omitempty"`
		Audio      json.RawMessage `json:"audio,omitempty"`

		SafetyIdentifier string          `json:"safety_identifier,omitempty"`
		Store            json.RawMessage `json:"store,omitempty"`

		PromptCacheKey       string          `json:"prompt_cache_key,omitempty"`
		PromptCacheRetention json.RawMessage `json:"prompt_cache_retention,omitempty"`

		LogitBias   json.RawMessage `json:"logit_bias,omitempty"`
		Metadata    json.RawMessage `json:"metadata,omitempty"`
		Prediction  json.RawMessage `json:"prediction,omitempty"`
		Usage       json.RawMessage `json:"usage,omitempty"`
		Reasoning   json.RawMessage `json:"reasoning,omitempty"`
		ExtraBody   json.RawMessage `json:"extra_body,omitempty"`        // gemini
		SearchParms json.RawMessage `json:"search_parameters,omitempty"` // xai

		WebSearchOptions       *webSearchOptions `json:"web_search_options,omitempty"`
		VlHighResolutionImages json.RawMessage   `json:"vl_high_resolution_images,omitempty"`
		EnableThinking         json.RawMessage   `json:"enable_thinking,omitempty"`
		ChatTemplateKwargs     json.RawMessage   `json:"chat_template_kwargs,omitempty"`
		EnableSearch           json.RawMessage   `json:"enable_search,omitempty"`
		Think                  json.RawMessage   `json:"think,omitempty"`
		WebSearch              json.RawMessage   `json:"web_search,omitempty"`
		THINKING               json.RawMessage   `json:"thinking,omitempty"`
		SearchDomainFilter     json.RawMessage   `json:"search_domain_filter,omitempty"`
		SearchRecencyFilter    string            `json:"search_recency_filter,omitempty"`
		ReturnImages           bool              `json:"return_images,omitempty"`
		ReturnRelatedQuestions bool              `json:"return_related_questions,omitempty"`
		SearchMode             string            `json:"search_mode,omitempty"`
	}

	var r req
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, errInvalidJSON
	}

	if strings.TrimSpace(r.Model) == "" {
		return nil, errors.New("model 不能为空")
	}
	if len(r.Messages) == 0 && r.Prefix == nil && r.Suffix == nil {
		return nil, errors.New("messages 不能为空")
	}

	if r.WebSearchOptions != nil {
		switch strings.TrimSpace(r.WebSearchOptions.SearchContextSize) {
		case "":
			r.WebSearchOptions.SearchContextSize = "medium"
		case "high", "medium", "low":
		default:
			return nil, errors.New("search_context_size 非法")
		}
	}

	// max_output_tokens/max_completion_tokens -> max_tokens（Chat Completions 语义）
	switch {
	case r.MaxCompletionTokens != nil:
		r.MaxTokens = nil
		r.MaxOutputTokens = nil
	case r.MaxTokens != nil:
		r.MaxCompletionTokens = nil
		r.MaxOutputTokens = nil
	case r.MaxOutputTokens != nil:
		r.MaxTokens = r.MaxOutputTokens
		r.MaxCompletionTokens = nil
		r.MaxOutputTokens = nil
	}

	// 参考 new-api：仅流式场景保留 stream_options，并默认补齐 include_usage=true。
	//（new-api 里该行为由 FORCE_STREAM_OPTION 控制，默认开启）
	if !r.Stream {
		r.StreamOptions = nil
	} else {
		r.StreamOptions = &streamOptions{IncludeUsage: true}
	}

	// tokens 缺省补齐：优先 max_completion_tokens，其次 max_tokens。
	if r.MaxCompletionTokens == nil && r.MaxTokens == nil && defaultMaxTokens > 0 {
		v := int64(defaultMaxTokens)
		r.MaxTokens = &v
	}

	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errInvalidJSON
	}
	return out, nil
}
