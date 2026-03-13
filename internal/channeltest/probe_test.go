package channeltest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"realms/internal/scheduler"
)

type fakeExecutor struct {
	do func(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error)
}

func (f fakeExecutor) Do(ctx context.Context, sel scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
	return f.do(ctx, sel, downstream, body)
}

func TestProbeOpenAIResponses(t *testing.T) {
	svc := New(fakeExecutor{
		do: func(_ context.Context, _ scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
			if downstream.URL.Path != "/v1/responses" {
				t.Fatalf("unexpected path: %s", downstream.URL.Path)
			}
			if !strings.Contains(string(body), `"max_output_tokens":1`) {
				t.Fatalf("unexpected body: %s", string(body))
			}
			return jsonResponse(http.StatusOK, `{"model":"gpt-5-mini"}`), nil
		},
	})

	got, err := svc.Probe(context.Background(), scheduler.Selection{CredentialType: scheduler.CredentialTypeOpenAI}, "gpt-5-mini")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if got.SuccessPath != "/v1/responses" {
		t.Fatalf("SuccessPath = %q", got.SuccessPath)
	}
	if got.UsedFallback {
		t.Fatal("expected no fallback")
	}
	if got.UpstreamResponseModel != "gpt-5-mini" {
		t.Fatalf("UpstreamResponseModel = %q", got.UpstreamResponseModel)
	}
}

func TestProbeOpenAIFallbackToChatCompletions(t *testing.T) {
	var calls []string
	svc := New(fakeExecutor{
		do: func(_ context.Context, _ scheduler.Selection, downstream *http.Request, body []byte) (*http.Response, error) {
			calls = append(calls, downstream.URL.Path)
			switch downstream.URL.Path {
			case "/v1/responses":
				return jsonResponse(http.StatusNotFound, `{"error":{"message":"responses unsupported"}}`), nil
			case "/v1/chat/completions":
				if !strings.Contains(string(body), `"max_tokens":1`) {
					t.Fatalf("unexpected fallback body: %s", string(body))
				}
				return jsonResponse(http.StatusOK, `{"model":"gpt-4.1-mini"}`), nil
			default:
				t.Fatalf("unexpected path: %s", downstream.URL.Path)
				return nil, nil
			}
		},
	})

	got, err := svc.Probe(context.Background(), scheduler.Selection{CredentialType: scheduler.CredentialTypeOpenAI}, "gpt-4.1-mini")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if strings.Join(calls, ",") != "/v1/responses,/v1/chat/completions" {
		t.Fatalf("unexpected calls: %v", calls)
	}
	if got.SuccessPath != "/v1/chat/completions" {
		t.Fatalf("SuccessPath = %q", got.SuccessPath)
	}
	if !got.UsedFallback {
		t.Fatal("expected fallback")
	}
	if got.UpstreamResponseModel != "gpt-4.1-mini" {
		t.Fatalf("UpstreamResponseModel = %q", got.UpstreamResponseModel)
	}
}

func TestProbeCodexSSEModel(t *testing.T) {
	svc := New(fakeExecutor{
		do: func(_ context.Context, _ scheduler.Selection, downstream *http.Request, _ []byte) (*http.Response, error) {
			if downstream.URL.Path != "/v1/responses" {
				t.Fatalf("unexpected path: %s", downstream.URL.Path)
			}
			body := strings.Join([]string{
				"event: response.created",
				"data: {\"response\":{\"model\":\"gpt-5-codex\"}}",
				"",
				"event: response.completed",
				"data: {\"message\":{\"model\":\"gpt-5-codex\"}}",
				"",
			}, "\n")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	})

	got, err := svc.Probe(context.Background(), scheduler.Selection{CredentialType: scheduler.CredentialTypeCodex}, "gpt-5-codex")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if got.SuccessPath != "/v1/responses" {
		t.Fatalf("SuccessPath = %q", got.SuccessPath)
	}
	if got.UpstreamResponseModel != "gpt-5-codex" {
		t.Fatalf("UpstreamResponseModel = %q", got.UpstreamResponseModel)
	}
}

func TestProbeErrorIncludesBodyMessage(t *testing.T) {
	svc := New(fakeExecutor{
		do: func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{"error":{"message":"bad request"}}`), nil
		},
	})

	_, err := svc.Probe(context.Background(), scheduler.Selection{CredentialType: scheduler.CredentialTypeAnthropic}, "claude-sonnet-4")
	if err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    &http.Request{Method: http.MethodPost},
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
	}
}
