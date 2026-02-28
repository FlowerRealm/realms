package upstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Sub2APIClient struct {
	baseURL    string
	gatewayKey string
	timeout    time.Duration
	client     *http.Client
}

func NewSub2APIClient(baseURL string, gatewayKey string, timeout time.Duration) *Sub2APIClient {
	return &Sub2APIClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		gatewayKey: strings.TrimSpace(gatewayKey),
		timeout:    timeout,
		client: &http.Client{
			Timeout:       0,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Sub2APIClient) Configured() bool {
	return c != nil && strings.TrimSpace(c.baseURL) != "" && strings.TrimSpace(c.gatewayKey) != ""
}

func (c *Sub2APIClient) Timeout() time.Duration {
	if c == nil {
		return 0
	}
	return c.timeout
}

func (c *Sub2APIClient) ForwardResponsesCompact(ctx context.Context, downstream *http.Request, body []byte, traceID string) (*http.Response, error) {
	return c.forward(ctx, downstream, "/v1/responses/compact", body, traceID)
}

func (c *Sub2APIClient) forward(ctx context.Context, downstream *http.Request, path string, body []byte, traceID string) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("sub2api client is nil")
	}
	if !c.Configured() {
		return nil, errors.New("sub2api is not configured")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sub2api path is empty")
	}

	base, err := url.Parse(c.baseURL)
	if err != nil || base == nil || base.Host == "" {
		return nil, fmt.Errorf("invalid sub2api base url: %w", err)
	}

	full := c.baseURL + path
	reqCtx := ctx
	cancel := func() {}
	if c.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, full, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Important: do NOT forward all client headers upstream (Host/Connection/Proxy-* can break routing,
	// and Authorization must never be leaked). Only forward a safe allowlist.
	allow := []string{
		"accept-language",
		"content-type",
		"user-agent",
		// codex sticky-session helpers
		"conversation_id",
		"session_id",
		"originator",
	}
	if downstream != nil {
		for _, name := range allow {
			if v := strings.TrimSpace(downstream.Header.Get(name)); v != "" {
				req.Header.Set(name, v)
			}
		}
	}

	req.Header.Set("Authorization", "Bearer "+c.gatewayKey)
	if strings.TrimSpace(req.Header.Get("Content-Type")) == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Align with su8-codes compact route: do not forward Accept; keep upstream defaults.
	req.Header.Del("Accept")

	if strings.TrimSpace(traceID) != "" {
		req.Header.Set("X-Gateway-Trace-Id", strings.TrimSpace(traceID))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
