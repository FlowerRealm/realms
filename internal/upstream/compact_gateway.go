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

type CompactGatewayClient struct {
	baseURL    string
	gatewayKey string
	timeout    time.Duration
	client     *http.Client
}

const (
	CompactGatewayTargetGroupHeader  = "X-Realms-Target-Group"
	CompactGatewayRouteKeyHashHeader = "X-Realms-Route-Key-Hash"
	CompactGatewayErrorClassHeader   = "X-Realms-Gateway-Error-Class"
)

type CompactGatewayRequestOptions struct {
	TargetGroup  string
	RouteKeyHash string
}

func NewCompactGatewayClient(baseURL string, gatewayKey string, timeout time.Duration) *CompactGatewayClient {
	return &CompactGatewayClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		gatewayKey: strings.TrimSpace(gatewayKey),
		timeout:    timeout,
		client: &http.Client{
			Timeout:       0,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *CompactGatewayClient) Configured() bool {
	return c != nil && strings.TrimSpace(c.baseURL) != "" && strings.TrimSpace(c.gatewayKey) != ""
}

func (c *CompactGatewayClient) Timeout() time.Duration {
	if c == nil {
		return 0
	}
	return c.timeout
}

func (c *CompactGatewayClient) ForwardResponsesCompact(ctx context.Context, downstream *http.Request, body []byte, traceID string, opts CompactGatewayRequestOptions) (*http.Response, error) {
	return c.forward(ctx, downstream, "/v1/responses/compact", body, traceID, opts)
}

func (c *CompactGatewayClient) forward(ctx context.Context, downstream *http.Request, path string, body []byte, traceID string, opts CompactGatewayRequestOptions) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("compact gateway client is nil")
	}
	if !c.Configured() {
		return nil, errors.New("compact gateway is not configured")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("compact gateway path is empty")
	}

	base, err := url.Parse(c.baseURL)
	if err != nil || base == nil || base.Host == "" {
		return nil, fmt.Errorf("invalid compact gateway base url: %w", err)
	}

	full := c.baseURL + path
	reqCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, full, bytes.NewReader(body))
	if err != nil {
		if cancel != nil {
			cancel()
		}
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
	if v := strings.TrimSpace(opts.TargetGroup); v != "" {
		req.Header.Set(CompactGatewayTargetGroupHeader, v)
	}
	if v := strings.TrimSpace(opts.RouteKeyHash); v != "" {
		req.Header.Set(CompactGatewayRouteKeyHashHeader, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if cancel != nil && resp.Body != nil {
		resp.Body = cancelOnClose(resp.Body, cancel)
	}
	return resp, nil
}
