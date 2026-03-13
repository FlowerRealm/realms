package modelcheck

import (
	"strings"

	"github.com/tidwall/gjson"
)

type Status string

const (
	StatusOK       Status = "ok"
	StatusMismatch Status = "mismatch"
	StatusUnknown  Status = "unknown"
)

func Optional(raw string) *string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	return &v
}

func Normalize(raw *string) *string {
	if raw == nil {
		return nil
	}
	return Optional(*raw)
}

func extractResponseModelString(raw string) *string {
	model := strings.TrimSpace(gjson.Get(raw, "model").String())
	if model == "" {
		model = strings.TrimSpace(gjson.Get(raw, "response.model").String())
	}
	if model == "" {
		model = strings.TrimSpace(gjson.Get(raw, "message.model").String())
	}
	return Optional(model)
}

func extractResponseModelLines(body string) *string {
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(line[len("data:"):])
		}
		if line == "" || line == "[DONE]" {
			continue
		}
		if model := extractResponseModelString(line); model != nil {
			return model
		}
	}
	return nil
}

func ExtractResponseModelBytes(body []byte) *string {
	if len(body) == 0 {
		return nil
	}
	if model := extractResponseModelString(string(body)); model != nil {
		return model
	}
	return extractResponseModelLines(string(body))
}

func ExtractResponseModelString(body string) *string {
	if model := extractResponseModelString(body); model != nil {
		return model
	}
	return extractResponseModelLines(body)
}

func Mismatch(forwardedModel *string, upstreamResponseModel *string) bool {
	forwarded := Normalize(forwardedModel)
	upstream := Normalize(upstreamResponseModel)
	if forwarded == nil || upstream == nil {
		return false
	}
	return *forwarded != *upstream
}

func StatusFrom(forwardedModel *string, upstreamResponseModel *string) Status {
	forwarded := Normalize(forwardedModel)
	upstream := Normalize(upstreamResponseModel)
	if forwarded == nil || upstream == nil {
		return StatusUnknown
	}
	if *forwarded == *upstream {
		return StatusOK
	}
	return StatusMismatch
}
