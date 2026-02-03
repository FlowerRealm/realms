# 调研：new-api 的 API 端口通信与转发实现

> 目标：解释 `QuantumNous/new-api` 如何在端口上接收请求，并把请求转发到上游（HTTP / SSE / WebSocket），供本项目后续实现 Codex API 中转参考。  
> 调研对象：`https://github.com/QuantumNous/new-api`  
> 本次调研基于提交：`6169f46cc67c7548a71965064d43483bd2646a13`

---

## 1. 监听端口与服务启动（入站）

**核心入口：** `main.go`

- 使用 `gin.New()` 创建 HTTP 服务（基于 Go 标准库 `net/http`）。
- 端口选择优先级：
  1) 环境变量 `PORT`  
  2) 命令行参数 `--port`（默认 `3000`，定义在 `common/init.go`）
- 最终通过 `server.Run(":"+port)` 启动监听（等价于 `http.ListenAndServe`）。

**辅助端口：**
- `ENABLE_PPROF=true` 时，会额外监听 `0.0.0.0:8005`（`net/http/pprof`），用于性能分析。

**重要备注：**
- `main.go` 中明确注释：对 Gin 启用 gzip 中间件会导致 SSE（流式）不可用，因此默认不启用响应 gzip 压缩。

---

## 2. 路由分层：对外 API 如何落到 Relay

**路由组合：** `router/main.go` → `SetRouter()`

其中与“端口通信/转发”关系最密切的是：`router/relay-router.go` → `SetRelayRouter()`

### 2.1 OpenAI 兼容 HTTP 路由（/v1）

`/v1` 下典型路由（节选）：
- `POST /v1/chat/completions`
- `POST /v1/completions`
- `POST /v1/embeddings`
- `POST /v1/images/generations`
- `POST /v1/audio/transcriptions`

这些路由统一进入 `controller.Relay(c, relayFormat)`，由 relayFormat 决定解析与转发策略（OpenAI / Claude / Gemini 等）。

### 2.2 WebSocket 路由（OpenAI Realtime）

在 `router/relay-router.go` 中：
- `GET /v1/realtime` 进入 `controller.Relay(c, types.RelayFormatOpenAIRealtime)`
- `controller/relay.go` 内部会对该路径执行 WebSocket Upgrade（基于 `gorilla/websocket`）

---

## 3. 入站请求“读 body / 复用 / 限制大小”的关键实现

这一块决定了你能否在 “验证请求 → 转发请求” 的链路中安全地多次读取 body。

### 3.1 解压与体积上限（防 zip bomb）

`middleware/gzip.go` → `DecompressRequestMiddleware()`：
- 支持 `Content-Encoding: gzip` 与 `Content-Encoding: br`（Brotli）
- 对 **解压后的请求体** 使用 `http.MaxBytesReader` 强制上限
- 同时也对未压缩的请求体应用上限，避免超大请求导致内存分配暴涨

### 3.2 body 缓存与可重复读取

`common/gin.go`：
- `GetRequestBody(c)`：读取并缓存到 `gin.Context`（key：`key_request_body`），并提供统一的大小限制与错误映射
- `UnmarshalBodyReusable(c, v)`：在解析 JSON / form / multipart 后，**将 `c.Request.Body` 重置**，保证后续仍可读取

这类“body 可复用”设计对于中转项目非常关键：你通常需要先解析/校验一次，再把原始 body 转发给上游（或在 retry 时再次读取）。

---

## 4. 出站请求构造与上游通信（HTTP）

**核心位置：** `relay/channel/api_request.go`

### 4.1 构造请求

`DoApiRequest()`（以及 `DoFormRequest()`）主要做：
- 通过 adaptor 计算上游 URL：`a.GetRequestURL(info)`
- `http.NewRequest(method, fullRequestURL, body)`
- 处理 header 覆盖（支持 `{api_key}` 变量替换）：`processHeaderOverride()`
- 调用 adaptor 写入鉴权/上游所需头：`a.SetupRequestHeader(...)`

### 4.2 选择 http.Client 与代理

`doRequest()`：
- 若 channel 配置了 `Proxy`：使用 `service.NewProxyHttpClient(proxyURL)`
- 否则使用全局 `service.GetHttpClient()`（由 `service.InitHttpClient()` 初始化）

`service/http_client.go` 的关键点：
- 自定义 `http.Transport`，设置连接池参数（`MaxIdleConns` / `MaxIdleConnsPerHost`），并开启 `ForceAttemptHTTP2`
- 支持环境变量代理：`HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`
- 支持 per-channel 代理（`http/https/socks5/socks5h`）
- `CheckRedirect` 内有 SSRF 防护：对重定向后的 URL 做域名/IP/端口校验（配置由 `system_setting` 驱动）

---

## 5. 流式转发（SSE）：端到端保持“流式语义”

这一块是很多中转项目最容易出问题的地方：只要中间层处理不当，客户端就会感觉“流式变成一次性返回”。

### 5.1 何时认定为流式

在 OpenAI 转发链路中（例如 `relay/compatible_handler.go`）：
- 请求中 `stream=true` 会进入流式逻辑
- 或者上游响应 `Content-Type: text/event-stream` 也会被识别为流式

### 5.2 SSE Header、Flush 与 Ping 保活

`relay/helper/common.go`：
- `SetEventStreamHeaders(c)`：设置 `Content-Type: text/event-stream`、`Transfer-Encoding: chunked` 等
- `FlushWriter(c)`：通过 `http.Flusher` 主动 flush，确保 chunk 尽快到达客户端
- `PingData(c)`：写入 `: PING\n\n`（注释行），用于保持连接活跃

### 5.3 上游 SSE 读取与转发

`relay/helper/stream_scanner.go` → `StreamScannerHandler()`：
- 用 `bufio.Scanner` 按行扫描 `resp.Body`
- 只消费 `data:` 开头的行（兼容 `[DONE]`）
- 通过 `dataHandler(data)` 将内容写回客户端（通常是透传，也可能做格式化/重写）
- 用互斥锁 `writeMutex` 避免 ping 与数据写入并发导致写乱序/崩溃
- 通过 `streamingTimeout` 防止上游长时间无数据时挂死
- 监听 `c.Request.Context().Done()`，客户端断开立即退出

**结论：**
SSE 中转的“关键三件事”是：**正确设置 Header**、**及时 Flush**、**避免并发写导致的乱序/阻塞**。

---

## 6. WebSocket（OpenAI Realtime）：Upgrade + Dial + 双向转发

WebSocket 这条链路在 `controller/relay.go` 与 `relay/websocket.go` 中。

### 6.1 Upgrade（客户端 → new-api）

`controller/relay.go`：
- 对 `RelayFormatOpenAIRealtime` 先执行 `upgrader.Upgrade(...)`
- `websocket.Upgrader` 的 `Subprotocols` 包含 `realtime`
- `CheckOrigin` 直接返回 `true`（允许跨域）——这对公网部署并不安全，需要在自建网关时谨慎处理

### 6.2 Dial（new-api → 上游）

`relay/channel/api_request.go`：
- `DoWssRequest()` 使用 `websocket.DefaultDialer.Dial(fullRequestURL, headers)`

### 6.3 转发（双向拷贝）

`relay/websocket.go`：
- 通过 adaptor 的 `DoResponse(...)` 承担双向读写（客户端 WS ↔ 上游 WS）
- 该层同时挂接了计费/统计逻辑（这与纯中转的最小实现可解耦）

---

## 7. 对 Codex API 中转项目的可复用落地建议（最小实现）

下面是把上述结论迁移到“Codex API 中转”时，最该优先实现的一小撮能力（别一上来就学全套计费/用户系统，那是自找麻烦）。

### 7.1 必做（最小可用）
- **端口监听**：`PORT` 环境变量 + `--port` 参数
- **HTTP 转发**：构造上游请求、透传必要 header、统一的超时/连接池
- **body 可复用**：缓存 request body，允许 “校验 → 转发 → 重试” 多次读取
- **SSE 流式**：正确 header + flush + ping（可选）+ 上游 `data:` 行扫描转发
- **请求体上限**：对压缩与解压后都限制大小

### 7.2 可选（确认确实需要再做）
- **WebSocket 代理**：如果 Codex/上游确实要求 WS（例如 Realtime），再实现 Upgrade + Dial + 双向转发
- **SSRF 防护**：当上游 URL 可配置或可被用户输入时才必须做“强防护”

### 7.3 不要踩的坑
- **对 SSE 做 gzip 响应压缩**：会破坏流式语义（new-api 已明确踩过并回避）
- **不限制解压后的 body**：zip bomb 会把网关打爆
- **WS 直接 `CheckOrigin=true`**：公网部署风险极高，至少要做域名白名单/鉴权

