# 变更提案: SSE 流式转发器对齐 new-api（断联修复 + token 统计）

## 需求背景

目前数据面 `POST /v1/responses` 的 SSE 流式请求存在“随机断联”，并伴随 token 统计口径不准的问题（尤其在流式路径）。

结合现有实现，主要风险点集中在：
1. **固定超时导致误杀长连接**
   - `middleware.RequestTimeout(limits.max_request_duration)` 默认 2 分钟，会直接对流式请求施加 deadline。
   - `upstream.Executor` 的 `http.Client.Timeout=limits.upstream_request_timeout` 默认 2 分钟，包含 body 读取时间，会在长输出时强制中断上游 SSE。
2. **SSE 转发实现对“大事件行”不友好**
   - `internal/upstream/sse.go` 以 64KB 行缓冲 `ReadString('\n')` 读取上游，遇到单行 `data:` 事件较大时可能触发 `ErrBufferFull` 并返回错误，从而断流。
3. **流式 token 统计缺失**
   - `internal/api/openai/handler.go` 流式分支目前不解析 SSE 事件里的 usage，仅做“按预留兜底结算/统计”，导致用量页面与计费口径偏差。

目标是参考 `QuantumNous/new-api` 的 `StreamScannerHandler` 思路，构建更稳的“流式管道”（大 buffer、ping、idle-timeout、并发写保护、客户端断开感知），并补齐流式 token 统计与可诊断性。

## 变更内容

1. 引入 **StreamPump**（对齐 new-api 的 StreamScanner）
   - SSE 扫描使用可配置的大 buffer（避免 64KB 行限制）。
   - ping 保活（`: ping\n\n` 注释事件），避免中间层/客户端在无数据间隔时误判断开。
   - idle-timeout（按“最后一次收到上游数据”刷新），取代固定 2m 截止。
   - 写互斥（避免 ping 与数据并发写导致的乱序/崩溃）。
2. 重新定义 **超时边界**
   - 非流式请求：保持现有 `max_request_duration` 与 `upstream_request_timeout` 策略。
   - 流式请求：取消“全局 2m deadline”的硬切断，改为以 idle-timeout 与最大流式时长控制资源占用。
3. 补齐 **流式 token 统计**
   - 从 SSE `data:` JSON 事件中尽力提取 usage（input/output/cached tokens），用于 `quota.Commit` 与 `usage_events` 请求明细。
   - 若未拿到 usage，兜底按预留金额结算，并落库标记 `usage_unknown`（仅写分类，不写 body）。
4. 增强 **断联可观测性**
   - 统一错误分类（例如：`client_disconnect` / `upstream_disconnect` / `stream_idle_timeout` / `stream_event_too_large` / `stream_read_error`）。
   - 明确区分“客户端断开”与“上游断开/读取失败”，避免调度器与告警误判。

## 影响范围

- **模块:** `internal/upstream`、`internal/api/openai`、`internal/middleware`、`internal/config`
- **文件:** 预计涉及新增/修改 SSE 转发器、超时中间件、流式 token 解析与测试用例
- **API:** 不新增/不破坏；主要是流式连接生命周期与统计口径的行为修正
- **数据:** 不新增表结构；继续使用 `usage_events` 的请求明细字段

## 核心场景

### 需求: 流式不断联（长连接）
**模块:** upstream / openai handler

#### 场景: 超过 2 分钟仍持续输出
- 预期结果: 不再因固定 2 分钟 deadline 断开；仅在 idle 超过阈值时中断并记录原因。

### 需求: 支持大 SSE 事件行
**模块:** upstream

#### 场景: 单次 data 行 > 64KB
- 预期结果: 不触发 ErrBufferFull；在可配置的 `max_event_bytes` 内正常转发；超限时明确分类为 `stream_event_too_large`。

### 需求: 流式 token 统计准确
**模块:** openai / quota / store

#### 场景: 上游在结束事件返回 usage
- 预期结果: 从 SSE 事件中提取 `input_tokens/output_tokens` 与 `cached_tokens`，用于结算与“请求明细”展示。

### 需求: 断联可诊断
**模块:** openai / upstream

#### 场景: 客户端中途断开
- 预期结果: 记录为 `client_disconnect`，不误记为上游故障；usage_event 能及时 finalize，避免长期停留 reserved。

## 风险评估

- **风险:** 放宽流式超时可能导致长连接占用资源
  - **缓解:** 保留 `max_sse_connections_per_token` 限制；新增 `stream_idle_timeout` 与 `max_stream_duration` 控制上限。
- **风险:** 大 buffer 增加内存占用
  - **缓解:** 采用可配置上限（默认保守），超限明确报错并中断；测试覆盖大事件行。
- **风险:** 流式 usage 提取兼容性不一致
  - **缓解:** 只做“尽力解析”，失败回退到 reserved；不影响基本转发能力。

