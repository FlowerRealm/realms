package channeltest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"realms/internal/modelcheck"
	"realms/internal/scheduler"
)

const (
	probePrompt          = "Reply with exactly: OK"
	probeReadLimit int64 = 1 << 20
)

type executor interface {
	Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)
}

type Result struct {
	ForwardedModel        string
	UpstreamResponseModel string
	SuccessPath           string
	UsedFallback          bool
}

type Prober interface {
	Probe(ctx context.Context, sel scheduler.Selection, model string) (Result, error)
}

type Service struct {
	exec executor
}

func New(exec executor) *Service {
	if exec == nil {
		return nil
	}
	return &Service{exec: exec}
}

func (s *Service) Probe(ctx context.Context, sel scheduler.Selection, model string) (Result, error) {
	if s == nil || s.exec == nil {
		return Result{}, fmt.Errorf("channel test probe not configured")
	}
	model = strings.TrimSpace(model)
	out := Result{ForwardedModel: model}

	switch sel.CredentialType {
	case scheduler.CredentialTypeOpenAI:
		responsesBody, err := json.Marshal(map[string]any{
			"model":             model,
			"input":             probePrompt,
			"max_output_tokens": 1,
		})
		if err != nil {
			return out, err
		}
		attempt, err := s.doProbe(ctx, sel, "/v1/responses", responsesBody)
		if err == nil {
			out.UpstreamResponseModel = attempt.UpstreamResponseModel
			out.SuccessPath = attempt.SuccessPath
			return out, nil
		}
		if !shouldFallbackOpenAIResponses(err) {
			return out, err
		}

		chatBody, chatErr := json.Marshal(map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "user", "content": probePrompt},
			},
			"max_tokens": 1,
		})
		if chatErr != nil {
			return out, chatErr
		}
		fallback, fallbackErr := s.doProbe(ctx, sel, "/v1/chat/completions", chatBody)
		if fallbackErr != nil {
			return out, fmt.Errorf("responses probe failed: %w; chat probe failed: %v", err, fallbackErr)
		}
		out.UpstreamResponseModel = fallback.UpstreamResponseModel
		out.SuccessPath = fallback.SuccessPath
		out.UsedFallback = true
		return out, nil

	case scheduler.CredentialTypeAnthropic:
		body, err := json.Marshal(map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "user", "content": probePrompt},
			},
			"max_tokens": 1,
		})
		if err != nil {
			return out, err
		}
		attempt, err := s.doProbe(ctx, sel, "/v1/messages", body)
		if err != nil {
			return out, err
		}
		out.UpstreamResponseModel = attempt.UpstreamResponseModel
		out.SuccessPath = attempt.SuccessPath
		return out, nil

	case scheduler.CredentialTypeCodex:
		body, err := json.Marshal(map[string]any{
			"model":             model,
			"input":             probePrompt,
			"max_output_tokens": 1,
		})
		if err != nil {
			return out, err
		}
		attempt, err := s.doProbe(ctx, sel, "/v1/responses", body)
		if err != nil {
			return out, err
		}
		out.UpstreamResponseModel = attempt.UpstreamResponseModel
		out.SuccessPath = attempt.SuccessPath
		return out, nil
	default:
		return out, fmt.Errorf("unsupported credential type: %s", sel.CredentialType)
	}
}

type probeAttempt struct {
	SuccessPath           string
	UpstreamResponseModel string
}

type probeHTTPError struct {
	Path       string
	StatusCode int
	Message    string
}

func (e *probeHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s returned HTTP %d: %s", e.Path, e.StatusCode, e.Message)
}

func shouldFallbackOpenAIResponses(err error) bool {
	var httpErr *probeHTTPError
	if !errors.As(err, &httpErr) || httpErr == nil {
		return false
	}
	switch httpErr.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	}

	msg := strings.ToLower(strings.TrimSpace(httpErr.Message))
	if msg == "" {
		return false
	}
	for _, marker := range []string{"unsupported", "not supported", "does not support"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func (s *Service) doProbe(ctx context.Context, sel scheduler.Selection, path string, body []byte) (probeAttempt, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://channel-test.local"+path, bytes.NewReader(body))
	if err != nil {
		return probeAttempt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.exec.Do(ctx, sel, req, body)
	if err != nil {
		return probeAttempt{}, err
	}
	if resp == nil {
		return probeAttempt{}, fmt.Errorf("%s returned nil response", path)
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, probeReadLimit))
	if readErr != nil {
		return probeAttempt{}, readErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return probeAttempt{}, &probeHTTPError{
			Path:       path,
			StatusCode: resp.StatusCode,
			Message:    summarizeProbeError(bodyBytes),
		}
	}

	out := probeAttempt{SuccessPath: path}
	if model := modelcheck.ExtractResponseModelBytes(bodyBytes); model != nil {
		out.UpstreamResponseModel = *model
	}
	return out, nil
}

func summarizeProbeError(body []byte) string {
	if len(body) == 0 {
		return "empty response"
	}
	for _, path := range []string{"error.message", "message", "error"} {
		if v := strings.TrimSpace(gjson.GetBytes(body, path).String()); v != "" {
			return truncateForProbe(v)
		}
	}
	return truncateForProbe(strings.TrimSpace(string(body)))
}

func truncateForProbe(v string) string {
	if len(v) <= 240 {
		return v
	}
	return v[:240] + "..."
}
