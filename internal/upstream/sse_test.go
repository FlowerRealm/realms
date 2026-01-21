package upstream

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

type flushWriter struct {
	h       http.Header
	buf     bytes.Buffer
	flushes int
	status  int
}

func (w *flushWriter) Header() http.Header {
	if w.h == nil {
		w.h = make(http.Header)
	}
	return w.h
}

func (w *flushWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *flushWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *flushWriter) Flush() {
	w.flushes++
}

func TestRelaySSE_FlushOnEventBoundary(t *testing.T) {
	in := "data: a\n\n" +
		"event: message\n" +
		"data: b\n\n"
	w := &flushWriter{}
	if err := RelaySSE(context.Background(), w, strings.NewReader(in)); err != nil {
		t.Fatalf("RelaySSE err: %v", err)
	}
	if got := w.buf.String(); got != in {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, in)
	}
	if w.flushes < 2 {
		t.Fatalf("expected at least 2 flushes, got=%d", w.flushes)
	}
}
