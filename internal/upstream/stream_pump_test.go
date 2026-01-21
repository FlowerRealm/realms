package upstream

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestPumpSSE_AllowsLargeDataLine(t *testing.T) {
	t.Parallel()

	large := strings.Repeat("a", 128<<10) // 128KB
	in := "data: {\"usage\":{\"input_tokens\":1,\"output_tokens\":2},\"pad\":\"" + large + "\"}\n\n"

	w := &flushWriter{}
	res, err := PumpSSE(context.Background(), w, io.NopCloser(strings.NewReader(in)), SSEPumpOptions{
		MaxLineBytes:     256 << 10,
		InitialLineBytes: 64 << 10,
	}, SSEPumpHooks{})
	if err != nil {
		t.Fatalf("PumpSSE err: %v (class=%s)", err, res.ErrorClass)
	}
	if got := w.buf.String(); got != in {
		t.Fatalf("unexpected output size=%d want=%d", len(got), len(in))
	}
	if w.flushes < 1 {
		t.Fatalf("expected at least 1 flush, got=%d", w.flushes)
	}
}

func TestPumpSSE_IdleTimeout(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	defer pw.Close()

	w := &flushWriter{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	res, err := PumpSSE(ctx, w, pr, SSEPumpOptions{
		MaxLineBytes:     256 << 10,
		InitialLineBytes: 64 << 10,
		IdleTimeout:      20 * time.Millisecond,
	}, SSEPumpHooks{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if res.ErrorClass != "stream_idle_timeout" {
		t.Fatalf("unexpected error class: %q", res.ErrorClass)
	}
}

func TestPumpSSE_CanceledContext(t *testing.T) {
	t.Parallel()

	var input bytes.Buffer
	input.WriteString("data: a\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := &flushWriter{}
	res, err := PumpSSE(ctx, w, io.NopCloser(bytes.NewReader(input.Bytes())), SSEPumpOptions{
		MaxLineBytes:     64 << 10,
		InitialLineBytes: 64 << 10,
	}, SSEPumpHooks{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if res.ErrorClass != "client_disconnect" {
		t.Fatalf("unexpected error class: %q", res.ErrorClass)
	}
}

func TestPumpSSE_OnDataAggregatesMultiLineDataEvent(t *testing.T) {
	t.Parallel()

	in := strings.Join([]string{
		"data: {\"usage\":{\"input_tokens\":1,",
		"data: \"output_tokens\":2}}",
		"",
		"",
	}, "\n")

	var got []string
	w := &flushWriter{}
	res, err := PumpSSE(context.Background(), w, io.NopCloser(strings.NewReader(in)), SSEPumpOptions{
		MaxLineBytes:     64 << 10,
		InitialLineBytes: 64 << 10,
	}, SSEPumpHooks{
		OnData: func(data string) {
			got = append(got, data)
		},
	})
	if err != nil {
		t.Fatalf("PumpSSE err: %v (class=%s)", err, res.ErrorClass)
	}
	if got := w.buf.String(); got != in {
		t.Fatalf("unexpected output size=%d want=%d", len(got), len(in))
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 OnData call, got=%d", len(got))
	}
	if want := "{\"usage\":{\"input_tokens\":1,\n\"output_tokens\":2}}"; got[0] != want {
		t.Fatalf("unexpected OnData payload: %q", got[0])
	}
}

func TestPumpSSE_OnDataSeparatesEvents(t *testing.T) {
	t.Parallel()

	in := strings.Join([]string{
		"data: a",
		"",
		"data: b",
		"",
		"",
	}, "\n")

	var got []string
	w := &flushWriter{}
	res, err := PumpSSE(context.Background(), w, io.NopCloser(strings.NewReader(in)), SSEPumpOptions{
		MaxLineBytes:     64 << 10,
		InitialLineBytes: 64 << 10,
	}, SSEPumpHooks{
		OnData: func(data string) {
			got = append(got, data)
		},
	})
	if err != nil {
		t.Fatalf("PumpSSE err: %v (class=%s)", err, res.ErrorClass)
	}
	if got := w.buf.String(); got != in {
		t.Fatalf("unexpected output size=%d want=%d", len(got), len(in))
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 OnData calls, got=%d (%v)", len(got), got)
	}
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected OnData payloads: %v", got)
	}
}
