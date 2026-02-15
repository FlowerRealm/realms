// realms-load-sse 是面向 1k-100k SSE/长连接的简单 soak 工具（curl 不适合大并发长连接）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type stats struct {
	started   int64
	ok        int64
	badStatus int64
	errors    int64
	bytes     int64
	firstN    int64
	firstSum  int64
	earlyEOF  int64
}

func main() {
	var (
		baseURL  = flag.String("base-url", "http://127.0.0.1:19090", "Realms base URL")
		token    = flag.String("token", "sk_playwright_e2e_user_token", "Bearer token")
		model    = flag.String("model", "gpt-5.2", "model")
		input    = flag.String("input", "hello", "input")
		conns    = flag.Int("conns", 100, "concurrent SSE connections")
		duration = flag.Duration("duration", 30*time.Second, "soak duration")
		ramp     = flag.Duration("ramp", 0, "ramp-up duration (0 = burst)")
	)
	flag.Parse()

	if *conns <= 0 {
		fmt.Fprintln(os.Stderr, "conns must be > 0")
		os.Exit(2)
	}
	if *duration <= 0 {
		fmt.Fprintln(os.Stderr, "duration must be > 0")
		os.Exit(2)
	}
	u := strings.TrimRight(strings.TrimSpace(*baseURL), "/") + "/v1/responses"

	payload := map[string]any{
		"model":  strings.TrimSpace(*model),
		"input":  *input,
		"stream": true,
		// required by /v1/messages but harmless here; keep payload minimal for compatibility.
	}
	raw, _ := json.Marshal(payload)

	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DisableCompression:  true,
		MaxIdleConns:        *conns,
		MaxIdleConnsPerHost: *conns,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{Transport: tr, Timeout: 0}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var s stats
	var wg sync.WaitGroup

	fmt.Printf("[sse-soak] url=%s conns=%d duration=%s ramp=%s\n", u, *conns, duration.String(), ramp.String())

	startedAt := time.Now()
	for i := 0; i < *conns; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if *ramp > 0 && *conns > 1 {
				delay := time.Duration(int64(*ramp) * int64(i) / int64(*conns-1))
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}

			atomic.AddInt64(&s.started, 1)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
			if err != nil {
				atomic.AddInt64(&s.errors, 1)
				return
			}
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Content-Type", "application/json")

			t0 := time.Now()
			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&s.errors, 1)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				atomic.AddInt64(&s.badStatus, 1)
				_, _ = io.CopyN(io.Discard, resp.Body, 1024)
				return
			}
			atomic.AddInt64(&s.ok, 1)

			buf := make([]byte, 32<<10)
			n, err := resp.Body.Read(buf)
			if n > 0 {
				atomic.AddInt64(&s.bytes, int64(n))
				atomic.AddInt64(&s.firstN, 1)
				atomic.AddInt64(&s.firstSum, int64(time.Since(t0)))
			}
			if err != nil {
				if errorsIsEOF(err) && ctx.Err() == nil {
					atomic.AddInt64(&s.earlyEOF, 1)
				}
				return
			}
			if _, err := io.CopyBuffer(io.Discard, resp.Body, buf); err != nil {
				if errorsIsEOF(err) && ctx.Err() == nil {
					atomic.AddInt64(&s.earlyEOF, 1)
				}
				return
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startedAt)

	firstAvg := time.Duration(0)
	if n := atomic.LoadInt64(&s.firstN); n > 0 {
		firstAvg = time.Duration(atomic.LoadInt64(&s.firstSum) / n)
	}
	fmt.Printf("[sse-soak] elapsed=%s started=%d ok=%d bad_status=%d errors=%d early_eof=%d bytes=%d first_byte_avg=%s\n",
		elapsed.String(),
		atomic.LoadInt64(&s.started),
		atomic.LoadInt64(&s.ok),
		atomic.LoadInt64(&s.badStatus),
		atomic.LoadInt64(&s.errors),
		atomic.LoadInt64(&s.earlyEOF),
		atomic.LoadInt64(&s.bytes),
		firstAvg.String(),
	)
}

func errorsIsEOF(err error) bool {
	return err == io.EOF || err == context.Canceled || strings.Contains(err.Error(), "use of closed network connection")
}

