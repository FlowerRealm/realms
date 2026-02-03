# 技术设计: SSE StreamPump（对齐 new-api）与流式 token 统计

## 技术方案

### 核心技术
- Go 标准库 `net/http`（上游请求与下游响应）
- `bufio.Scanner`（可配置大 buffer 的逐行扫描）
- `time.Ticker`（idle-timeout 与 ping）
- `sync.Mutex` + `sync.WaitGroup`（并发写保护与资源回收）

### 实现要点

#### 1) 新增 StreamPump：按行扫描 + 写互斥 + ping + idle-timeout
- 在 `internal/upstream` 引入一个“流式泵”实现（命名可选：`stream_pump.go`），对齐 new-api 的核心行为：
  - Scanner：`scanner.Buffer(initial, max)`，避免 64KB 单行限制
  - 写入：所有 `ResponseWriter.Write` 包裹在互斥锁内（ping 与数据共享写通道）
  - flush：在 SSE 事件边界（空行）或关键写入点 flush
  - idle-timeout：每收到一行上游数据就重置计时器，超过阈值视为上游卡死并断开
  - ping：可配置间隔写入 `: ping\n\n`，用于保持连接活跃（尤其在长时间无增量但连接应保持时）
- 保持兼容：保留现有 `upstream.RelaySSE(ctx, w, body)` 入口（可作为默认参数的 wrapper），避免影响其它调用点。

#### 2) 流式与非流式的超时策略拆分
- 现状问题：`RequestTimeout(2m)` 与 `http.Client.Timeout(2m)` 都会对 SSE 造成“硬中断”。
- 计划调整：
  - `internal/middleware` 提供“流式感知”的 timeout 中间件（建议按路由路径判断：`POST /v1/responses`、`POST /v1/chat/completions` 视为可流式）。
    - 流式：跳过 `context.WithTimeout`（由客户端断开 + StreamPump idle-timeout 控制）
    - 非流式：保持现有行为
  - `internal/upstream/executor.go` 取消全局 `http.Client.Timeout`（或改为仅用于非流式的 client），依赖 request context 控制时长；SSE 由 StreamPump idle-timeout 控制读取阶段。

#### 3) 流式 token 统计：从 SSE 事件提取 usage
- 在 `internal/api/openai/handler.go` 的 SSE 分支：
  - 通过 StreamPump 的 hook（对齐 new-api 的 `dataHandler`）拦截 `data:` JSON 事件，尽力提取 usage tokens：
    - `usage.input_tokens` / `usage.output_tokens`
    - `usage.input_tokens_details.cached_tokens`（或 prompt_tokens_details 等兼容字段）
  - 在流结束（`[DONE]` 或 EOF）后，用解析到的 tokens 调用 `quota.Commit`；未解析到 usage 时 tokens 置空，沿用现有“按预留兜底结算”逻辑。
  - 将“是否解析到 usage / 解析失败原因”映射为 `FinalizeUsageEvent.error_class`（不写响应 body）。
- 同时补强非流式 `extractUsageTokens` 的兼容性（如遇到上游把 usage 放在嵌套字段，增加兜底路径），避免同类口径问题在非流式仍存在。

#### 4) 错误分类与调度器反馈
- StreamPump 需要把错误映射到稳定的分类（例如：`client_disconnect` / `upstream_disconnect` / `stream_idle_timeout` / `stream_event_too_large` / `stream_read_error`）。
- 在 SSE 已开始写回后不能 failover，但仍应：
  - 向 scheduler 记录失败（便于后续选择更健康的上游）
  - finalize usage_event（避免 reserved 悬挂）
  - 保持对用户侧的“流式语义”（尽量及时 flush，并在断流后尽快结束连接）

## 架构决策 ADR

### ADR-001: 用 StreamPump 替换现有 RelaySSE 读取模型
**上下文:** 当前 `ReadString('\n') + 64KB buffer` 在大事件行下会触发 ErrBufferFull；固定 2m timeout 误杀长连接。  
**决策:** 参考 new-api，引入 StreamPump（scanner 大 buffer + ping + idle-timeout + 写互斥），并将超时策略拆分为“非流式 deadline / 流式 idle-timeout”。  
**理由:** 以最小依赖对齐成熟实现，优先解决断联根因，同时提升可观测性与统计口径。  
**替代方案:** 继续使用 `bufio.Reader` 并手写动态扩容读取 → 拒绝原因: 复杂度高、易引入边界 bug；Scanner 的 buffer 机制足够且可控。  
**影响:** 行为变化集中在 SSE 生命周期与结算口径；需要补充单元测试覆盖断流/大行/idle-timeout。  

## 安全与性能
- **安全:** 不记录请求/响应 body；只记录 error_class 与状态摘要；避免在日志/DB 中落明文凭据。
- **性能:** ping 与扫描为常数开销；max_event_bytes 默认保守，避免异常上游导致内存爆炸；idle-timeout 防止卡死连接占用资源。

## 测试与部署
- **测试:**
  - upstream StreamPump：大事件行、flush 行为、idle-timeout、client cancel 场景
  - openai handler：流式 usage 提取与 quota.Commit 参数断言（可通过 mock Doer / fake upstream SSE）
- **部署:**
  - 新增配置项提供默认值（不改配置也能更稳）
  - 对线上流式连接建议监控：SSE 连接数、断流 error_class 分布、TTFT/耗时与 499/502 类状态

