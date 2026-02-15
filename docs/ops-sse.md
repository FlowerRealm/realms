# SSE / 长连接运维指南（10k–100k connections）

本项目把 “请求（HTTP）” 与 “连接（SSE）” 作为不同口径分别度量（见 `.omx/plans/million-scale-master-plan.md:4`）。

## 1) 关键指标（必须同时看）

### 连接口径（SSE）
- `active_sse_connections`、`total_sse_connections`：`/debug/vars`（`internal/obs/expvar.go:13`）
- `sse_pump_results_total`、`sse_first_write_*`、`sse_bytes_streamed_total`：`/debug/vars`（`internal/obs/sse_expvar.go:1`）

### 请求口径（HTTP）
- QPS、p50/p99 latency、status 分布：建议由反代/LB（或 APM）采集

## 2) 反向代理（Nginx）要点

SSE 容易被代理缓冲/超时误伤，建议（示例片段，按你的部署调整）：

```
location /v1/responses {
  proxy_http_version 1.1;
  proxy_set_header Connection "";
  proxy_buffering off;
  proxy_cache off;
  proxy_read_timeout 3600s;
  proxy_send_timeout 3600s;
}
```

常见故障模式：
- 首包慢：`proxy_buffering` 打开导致事件被缓冲
- 早断：读超时太短或上游 idle timeout/网络抖动

## 3) 机器资源上限（100k 连接是系统工程）

100k SSE 连接通常会先撞到这些限制（需要系统/容器/反代一起调）：
- FD：`ulimit -n`（每连接至少 1 FD；代理层也消耗）
- 内存：每连接缓冲 + goroutine 栈（要看 RSS 曲线）
- goroutines：`/debug/pprof/goroutine?debug=1`（需开启 debug routes）

建议在压测环境把这些观测固定下来：
- RSS、FD、goroutines、CPU、`active_sse_connections`
- `sse_pump_results_total` 中的错误类占比（`client_disconnect`、`stream_idle_timeout`、`stream_read_error` 等）

## 4) 可复现压测（建议从 1k → 10k → 50k）

### Soak（连接稳定性）
- 使用工具：`cmd/realms-load-sse/main.go:1`
- 脚本：`scripts/soak-sse.sh:1`

示例：
```
REALMS_SOAK_CONNS=1000 REALMS_SOAK_DURATION=60s REALMS_SOAK_RAMP=10s bash scripts/soak-sse.sh
```

### 读指标（debug vars）
开启 debug routes 并满足 guard（见 `.env.example:109`）后：
```
curl -fsS -H "X-Realms-Debug-Token: <token>" http://127.0.0.1:19090/debug/vars | jq .
```

