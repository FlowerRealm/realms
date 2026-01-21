// Package upstream 实现 SSE 的逐事件转发与 flush，避免“读完再写回”的假流式体验。
package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

func RelaySSE(ctx context.Context, w http.ResponseWriter, upstreamBody io.Reader) error {
	rc, ok := upstreamBody.(io.ReadCloser)
	if !ok {
		// 兼容历史签名：在无法主动 Close 的情况下，仅做最小透传（无 idle/ping）。
		return relaySSENoClose(ctx, w, upstreamBody)
	}
	_, err := PumpSSE(ctx, w, rc, SSEPumpOptions{
		// RelaySSE 作为兼容入口，使用保守默认值；上层可直接调用 PumpSSE 自定义参数。
		MaxLineBytes:     4 << 20,
		InitialLineBytes: 64 << 10,
		PingInterval:     0,
		IdleTimeout:      0,
	}, SSEPumpHooks{})
	return err
}

func relaySSENoClose(ctx context.Context, w http.ResponseWriter, upstreamBody io.Reader) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("ResponseWriter 不支持 Flush")
	}

	buf := make([]byte, 32<<10)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := upstreamBody.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			// 无法可靠识别事件边界时，按固定频率 flush，避免积压太久。
			flusher.Flush()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				flusher.Flush()
				return nil
			}
			return err
		}
		// 避免 tight loop；理论上 Read 会阻塞，但某些 Reader 可能返回 (0, nil)。
		time.Sleep(5 * time.Millisecond)
	}
}
