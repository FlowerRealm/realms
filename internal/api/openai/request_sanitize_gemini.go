package openai

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/tidwall/gjson"
)

func sanitizeGeminiBody(body []byte, path string) ([]byte, error) {
	switch {
	case strings.Contains(path, ":embedContent"):
		return sanitizeGeminiEmbeddingBody(body)
	case strings.Contains(path, ":batchEmbedContents"):
		return sanitizeGeminiBatchEmbeddingBody(body)
	default:
		return sanitizeGeminiChatBody(body)
	}
}

func sanitizeGeminiChatBody(body []byte) ([]byte, error) {
	type req struct {
		Requests json.RawMessage `json:"requests,omitempty"` // batch
		Contents json.RawMessage `json:"contents,omitempty"`

		SafetySettings   json.RawMessage `json:"safetySettings,omitempty"`
		GenerationConfig json.RawMessage `json:"generationConfig,omitempty"`
		Tools            json.RawMessage `json:"tools,omitempty"`
		ToolConfig       json.RawMessage `json:"toolConfig,omitempty"`

		SystemInstruction      json.RawMessage `json:"systemInstruction,omitempty"`
		SystemInstructionSnake json.RawMessage `json:"system_instruction,omitempty"`

		CachedContent string `json:"cachedContent,omitempty"`
	}

	var r req
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, errInvalidJSON
	}

	if len(r.SystemInstruction) == 0 && len(r.SystemInstructionSnake) > 0 {
		r.SystemInstruction = r.SystemInstructionSnake
		r.SystemInstructionSnake = nil
	}

	contentsLen := int(gjson.GetBytes(r.Contents, "#").Int())
	requestsLen := int(gjson.GetBytes(r.Requests, "#").Int())
	if contentsLen == 0 && requestsLen == 0 {
		return nil, errors.New("contents 不能为空")
	}

	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func sanitizeGeminiEmbeddingBody(body []byte) ([]byte, error) {
	type req struct {
		Model string `json:"model,omitempty"`

		Content json.RawMessage `json:"content,omitempty"`

		TaskType             string `json:"taskType,omitempty"`
		Title                string `json:"title,omitempty"`
		OutputDimensionality int    `json:"outputDimensionality,omitempty"`
	}
	var r req
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, errInvalidJSON
	}
	return json.Marshal(r)
}

func sanitizeGeminiBatchEmbeddingBody(body []byte) ([]byte, error) {
	type req struct {
		Requests json.RawMessage `json:"requests,omitempty"`
	}
	var r req
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, errInvalidJSON
	}
	return json.Marshal(r)
}
