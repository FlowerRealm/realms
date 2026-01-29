// Package upstream 提供上游流式（SSE）转发工具：大行 buffer、ping 保活、idle 超时与安全的并发写入。
package upstream

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SSEPumpOptions struct {
	// MaxLineBytes 控制单行最大长度（用于 bufio.Scanner）。
	// - > 0：启用最大行长度限制；超过该值会触发 bufio.ErrTooLong
	// - <= 0：不限制行长度（以 bufio.Reader 读取，避免 Scanner 限制）
	MaxLineBytes int
	// InitialLineBytes 控制行读取的初始 buffer（避免频繁扩容）。
	InitialLineBytes int

	// PingInterval 为 0 表示禁用 ping；否则周期性向下游写入 SSE 注释行保持连接活跃。
	PingInterval time.Duration
	// IdleTimeout 为 0 表示禁用 idle 超时；否则当上游在该时长内无任何输出时终止转发。
	IdleTimeout time.Duration
}

type SSEPumpHooks struct {
	// OnData 在遇到 `data:` 行时触发（已剥离 `data:` 前缀并 trim 空白）。
	OnData func(data string)
	// TransformData 用于按“事件边界（空行）”对 data payload 做变换：
	// - 入参 data 为聚合后的 event payload（多行 data: 以 "\n" 拼接）
	// - 返回非空 slice 表示用返回值替换该事件（每个元素会被写为一个独立的 data: 事件）
	// - 返回 nil/空 slice 表示不替换（透传原始事件）
	TransformData func(data string) ([]string, error)
}

type SSEPumpResult struct {
	ErrorClass string
}

var (
	errSSEIdleTimeout  = errors.New("sse idle timeout")
	errSSEEventTooLong = errors.New("sse event too large")
)

func PumpSSE(ctx context.Context, w http.ResponseWriter, upstreamBody io.ReadCloser, opts SSEPumpOptions, hooks SSEPumpHooks) (SSEPumpResult, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return SSEPumpResult{ErrorClass: "no_flusher"}, errors.New("ResponseWriter 不支持 Flush")
	}
	if upstreamBody == nil {
		return SSEPumpResult{ErrorClass: "upstream_body_nil"}, errors.New("上游响应体为空")
	}

	maxLine := opts.MaxLineBytes
	initialLine := opts.InitialLineBytes
	if initialLine <= 0 {
		initialLine = 64 << 10
	}
	if maxLine > 0 && initialLine > maxLine {
		initialLine = minInt(64<<10, maxLine)
	}

	var (
		stopOnce sync.Once
		stopCh   = make(chan struct{})
		wg       sync.WaitGroup
		writeMu  sync.Mutex
	)
	stop := func() {
		stopOnce.Do(func() {
			close(stopCh)
			_ = upstreamBody.Close()
		})
	}

	lineCh := make(chan string, 32)
	errCh := make(chan error, 1) // 仅发送一次：scanner.Err()（nil 表示正常结束）

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(lineCh)

		if maxLine > 0 {
			sc := bufio.NewScanner(upstreamBody)
			sc.Buffer(make([]byte, 0, initialLine), maxLine)
			sc.Split(bufio.ScanLines)

			for sc.Scan() {
				line := sc.Text()
				select {
				case lineCh <- line:
				case <-stopCh:
					select {
					case errCh <- errors.New("stopped"):
					default:
					}
					return
				case <-ctx.Done():
					select {
					case errCh <- ctx.Err():
					default:
					}
					return
				}
			}
			select {
			case errCh <- sc.Err():
			default:
			}
			return
		}

		br := bufio.NewReaderSize(upstreamBody, initialLine)
		for {
			b, err := br.ReadBytes('\n')
			if len(b) > 0 {
				// ScanLines 的行为：返回值不包含 '\n'，但会保留 '\r'（上游可能是 CRLF）。
				if b[len(b)-1] == '\n' {
					b = b[:len(b)-1]
				}
				line := string(b)
				select {
				case lineCh <- line:
				case <-stopCh:
					select {
					case errCh <- errors.New("stopped"):
					default:
					}
					return
				case <-ctx.Done():
					select {
					case errCh <- ctx.Err():
					default:
					}
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					select {
					case errCh <- nil:
					default:
					}
					return
				}
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}()

	if opts.PingInterval > 0 {
		pingInterval := opts.PingInterval
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(pingInterval)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					writeMu.Lock()
					_, werr := io.WriteString(w, ": ping\n\n")
					if werr == nil {
						flusher.Flush()
					}
					writeMu.Unlock()
					if werr != nil {
						stop()
						return
					}
				case <-stopCh:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	var idleTimer *time.Timer
	if opts.IdleTimeout > 0 {
		idleTimer = time.NewTimer(opts.IdleTimeout)
		defer idleTimer.Stop()
	}
	resetIdle := func() {
		if idleTimer == nil {
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(opts.IdleTimeout)
	}

	var (
		res      SSEPumpResult
		retError error
	)

	// SSE 事件允许多行 data:，规范要求将多行按 "\n" 连接后视为一个事件 payload。
	// 这里按事件边界（空行）聚合，避免上游把 JSON 拆成多行导致下游解析/计费统计丢失。
	var (
		eventLines     []string
		eventData      strings.Builder
		hasData        bool
		hasNonDataLine bool
	)

	isTransformSafeLine := func(line string) bool {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return true
		}
		if strings.HasPrefix(trimmed, "data:") {
			return true
		}
		if strings.HasPrefix(trimmed, ":") {
			return true
		}
		return false
	}

	resetEventBuf := func() {
		eventLines = eventLines[:0]
		eventData.Reset()
		hasData = false
		hasNonDataLine = false
	}

	flushEvent := func() error {
		// 先对齐旧行为：仅在有聚合 data 时触发 OnData。
		agg := ""
		if hasData {
			agg = eventData.String()
		}

		// 可变换事件：仅对“纯 data/注释行事件”生效，避免丢失 event:/id:/retry: 等字段。
		if agg != "" && hooks.TransformData != nil && !hasNonDataLine {
			outs, err := hooks.TransformData(agg)
			if err == nil && len(outs) > 0 {
				writeMu.Lock()
				defer writeMu.Unlock()
				for _, out := range outs {
					if hooks.OnData != nil {
						hooks.OnData(out)
					}
					if _, werr := io.WriteString(w, "data: "+out+"\n\n"); werr != nil {
						return werr
					}
					flusher.Flush()
				}
				resetEventBuf()
				return nil
			}
		}

		if agg != "" && hooks.OnData != nil {
			hooks.OnData(agg)
		}

		writeMu.Lock()
		defer writeMu.Unlock()
		for _, l := range eventLines {
			if _, werr := io.WriteString(w, l+"\n"); werr != nil {
				return werr
			}
		}
		flusher.Flush()
		resetEventBuf()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				res.ErrorClass = "stream_max_duration"
			} else {
				res.ErrorClass = "client_disconnect"
			}
			retError = ctx.Err()
			stop()
			wg.Wait()
			return res, retError
		case line, ok := <-lineCh:
			if !ok {
				err := <-errCh
				if err == nil {
					if len(eventLines) > 0 {
						if werr := flushEvent(); werr != nil {
							res.ErrorClass = "client_disconnect"
							stop()
							wg.Wait()
							return res, werr
						}
					}
					stop()
					wg.Wait()
					return res, nil
				}
				if errors.Is(err, bufio.ErrTooLong) {
					res.ErrorClass = "stream_event_too_large"
					stop()
					wg.Wait()
					return res, errSSEEventTooLong
				}
				if ctx.Err() != nil {
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						res.ErrorClass = "stream_max_duration"
					} else {
						res.ErrorClass = "client_disconnect"
					}
					stop()
					wg.Wait()
					return res, ctx.Err()
				}
				if errors.Is(err, errSSEIdleTimeout) || errors.Is(err, errSSEEventTooLong) {
					stop()
					wg.Wait()
					return res, err
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					// ctx.Done 分支已处理，这里兜底。
					res.ErrorClass = "client_disconnect"
					stop()
					wg.Wait()
					return res, err
				}
				if strings.TrimSpace(err.Error()) == "stopped" {
					stop()
					wg.Wait()
					return res, nil
				}
				res.ErrorClass = "stream_read_error"
				stop()
				wg.Wait()
				return res, fmt.Errorf("读取上游流式响应失败: %w", err)
			}
			resetIdle()

			data := strings.TrimSuffix(line, "\r")
			eventLines = append(eventLines, line)
			if !isTransformSafeLine(data) {
				hasNonDataLine = true
			}
			if v := parseSSEDataLine(data); v != "" && v != "[DONE]" {
				if hasData {
					eventData.WriteByte('\n')
				}
				eventData.WriteString(v)
				hasData = true
			}
			if strings.TrimSpace(data) == "" {
				if werr := flushEvent(); werr != nil {
					res.ErrorClass = "client_disconnect"
					stop()
					wg.Wait()
					return res, werr
				}
			}
		case <-idleTimerC(idleTimer):
			res.ErrorClass = "stream_idle_timeout"
			stop()
			wg.Wait()
			return res, errSSEIdleTimeout
		case <-stopCh:
			wg.Wait()
			return res, nil
		}
	}
}

func parseSSEDataLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if !strings.HasPrefix(line, "data:") {
		return ""
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	return data
}

func idleTimerC(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
