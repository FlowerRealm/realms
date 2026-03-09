package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type contextBoundReadCloser struct {
	ctx  context.Context
	data []byte
}

func (r *contextBoundReadCloser) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *contextBoundReadCloser) Close() error {
	return nil
}

func TestCompactGatewayClientForwardResponsesCompact_KeepsTimeoutContextAliveUntilBodyClosed(t *testing.T) {
	t.Parallel()

	var reqCtx context.Context
	client := NewCompactGatewayClient("http://compact-gateway.test", "gwk_test", time.Second)
	client.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		reqCtx = req.Context()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: &contextBoundReadCloser{
				ctx:  req.Context(),
				data: []byte(`{"ok":true}`),
			},
		}, nil
	})

	downstream := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", nil)
	resp, err := client.ForwardResponsesCompact(context.Background(), downstream, []byte(`{"input":"hi"}`), "trace-1", CompactGatewayRequestOptions{})
	if err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	if reqCtx == nil {
		t.Fatal("expected request context to be captured")
	}
	if err := reqCtx.Err(); err != nil {
		t.Fatalf("request context canceled before body read: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if err := reqCtx.Err(); err != nil {
		t.Fatalf("request context canceled before body close: %v", err)
	}

	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body failed: %v", err)
	}
	if err := reqCtx.Err(); err == nil {
		t.Fatal("expected request context canceled after body close")
	}
}
