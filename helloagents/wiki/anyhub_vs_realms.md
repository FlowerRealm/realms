# AnyHub vs Realms：协议拓展、路由与故障（对比与建议）

> 对比目的：回答“AnyHub（nightwhite/anyhub）与本地 Realms 的优缺点”，并聚焦 **协议拓展 / 路由 / 故障（failover & streaming）**。
>
> 对比基线：
> - AnyHub：`7ea3e2d2a41a077ebd44c124d707ba7d3970912c`（拉取到 `./.tmp/anyhub_20260125110009/`）
> - Realms：`bb8514e4675b3ec7e940cff4290ac3903423ff7b`（工作区）

## 1) 定位与技术栈

### AnyHub
- 定位：自托管 LLM API gateway + Web console（同域同时支持 OpenAI/Anthropic/Gemini 风格北向接口）。
- 技术栈：Next.js + Bun(Elysia) + PostgreSQL(Drizzle) + Redis(可选)。
- 优势：控制台和网关逻辑同仓，配置/观测/策略体系比较“产品化”。
- 代价：运行时组件更多（Node + Bun + DB + 可选 Redis），运维复杂度更高。

### Realms
- 定位：偏“数据面网关 + 管理后台”的一体化服务（Go）。
- 技术栈：Go 单体服务（附 Web/管理后台），DB 侧以迁移/表结构驱动能力演进。
- 优势：单二进制、资源占用与部署链路更简单；数据面调度/计费/审计强耦合，口径一致性更好。
- 代价：如果要覆盖更多“外部生态协议”（如 Gemini、更多 OpenAI 旧接口），需要持续补齐适配层。

## 2) 协议拓展（Protocol Extension）

### AnyHub：内部协议 + 适配器/注册表
- AnyHub 以“内部请求（internal request）”为中心：将不同北向协议解析成统一内部结构，再按 providerType 做转换/转发。
- 典型例子：`/v1/chat/completions` 可转换为 OpenAI Responses 上游请求，并把 Responses 返回转换回 Chat 形态：
  - `src/gateway/protocol/registry.ts`：
    - `transformOpenAIChatToOpenAIResponsesRequest`
    - `transformOpenAIResponsesNonStreamToOpenAIChat`
    - `transformOpenAIResponsesStreamChunkToOpenAIChatSse`
- 好处：新增一种北向协议或新增一种 providerType 时，有明确的“转换点”与“禁止隐式路由”的安全边界。

### Realms：以 `/v1/responses` 为“统一计费与调度口径”
- Realms 选择 **Responses 作为北向/内部口径的锚点**，将 `/v1/chat/completions` 视为兼容入口：
  - `internal/api/openai/chat_completions.go`：Chat → Responses（转发）→ Chat（回写）
- 好处：
  - 与现有模型目录/分组路由/计费与用量口径天然复用，避免“同一请求在不同端点口径不一致”。
  - Codex OAuth 等只支持 `/v1/responses` 的上游类型，可以在兼容端点下复用。
- 取舍：
  - Chat 生态的部分字段（尤其 multimodal/工具调用的细节）在“仅做最小映射”时会丢失或语义变化，需要明确告知与逐步补齐。

### 关键差异点（建议视目标选择）
- 如果目标是“多协议、多 providerType 同时高质量支持”：AnyHub 的 registry/adapter 思路更直接。
- 如果目标是“计费/调度口径绝对一致、减少协议分叉”：Realms 的“以 Responses 为 SSOT”更稳。

## 3) 路由（Routing）

### AnyHub：Model Route → Provider Candidate（优先级/权重/成本/健康）
- 核心：按“public model”配置多条 route，route 绑定 provider，提供 `priority + weight`；并叠加预算压力、健康惩罚、成本倍率等形成候选集。
- 参考实现：
  - `src/gateway/scheduler.ts`：`getProviderCandidates`（priority/weight、budget pressure、health penalty、timeouts）
  - `src/routes/v1-chat.ts`：`decisionChain` 记录每次 skip/fail 的原因，便于观测与诊断。

### Realms：Channel Group Tree + 约束（组/类型/ID）+ 粘性
- 核心：以渠道（channel）为原子，上层通过“分组树”组织；支持 routeKeyHash 的会话粘性与 affinity。
- 参考实现：
  - `internal/scheduler/scheduler.go`：binding/affinity、channel ban、credential cooling、RPM/TPM
  - `internal/scheduler/group_router.go`：组树展开、max_attempts、按约束过滤候选
  - `internal/api/openai/chat_completions.go`：对 `/v1/chat/completions` 增加 `AllowChannelTypes` 约束，避免跨协议隐式映射。

### 优缺点小结
- AnyHub 路由更“显式可配”（model → route → provider），适合多 provider、多策略组合与可观测性要求高的场景。
- Realms 路由更“数据面一致性优先”（channel/credential 的调度、粘性、限额与 failover 一体），适合以可用性与计费一致为主的网关。

## 4) 故障与流式（Failover & Streaming）

### AnyHub：尝试链 + 首字节超时 + 规则化错误处理
- 典型能力：
  - 流式请求支持 **first byte timeout**：在拿到首包前若超时/失败可切换上游（避免“卡住不出字”导致长时间占用连接）。
  - 通过 `decisionChain` + request log 记录每次尝试的 outcome/reason。
- 参考实现：
  - `src/gateway/streaming-attempts.ts`：`openaiSseAttempt/openaiResponsesSseAttempt`（首字节超时、失败分类、可重试判定）
  - `src/gateway/upstream-error-rules.ts`：错误规则（按状态码/响应体做归一）

### Realms：非流式可 failover；流式写回后禁止 failover；流式超时按路径识别
- 核心原则：**写回（开始输出）后禁止 failover**，避免客户端看到“拼接的多段输出”或语义撕裂。
- 相关实现：
  - `internal/api/openai/chat_completions.go`：
    - 非流式：网络错误/可重试状态码 → 继续 failover
    - 流式：开始写回后仅做 SSE pump（含 idle/max-duration 分类），并将结果回填调度状态与用量口径
  - `internal/middleware/stream_timeout.go`：`/v1/chat/completions` 纳入 stream-capable path，避免 SSE 被非流式超时误杀。
- 与 AnyHub 的差异：
  - Realms 当前更偏“服务端流式边界正确性”，而 AnyHub 更偏“首包前更积极的 failover 体验”。

## 5) 建议：如果要把 Realms 对齐到 AnyHub 的“协议/路由/故障体验”

按投入产出排序：
1. **补齐流式首字节超时（TTFB）**：仅在未写回前启用，可显著提升“卡住不出字”的可用性体验（对齐 AnyHub 的 `firstByteTimeoutMs`）。✅ Realms 已实现：`limits.stream_first_byte_timeout` / `REALMS_LIMITS_STREAM_FIRST_BYTE_TIMEOUT`（见 `internal/api/openai/handler.go`、`internal/api/openai/stream_first_byte.go`）。
2. **按上游/渠道暴露可配超时**：request timeout / stream idle timeout / first byte timeout 分离配置，便于针对不同 provider 调参。
3. **更“显式”的协议-渠道映射矩阵**：像 AnyHub 一样把“哪些 providerType 支持哪些北向端点”固化为显式约束（Realms 已在 `/v1/chat/completions` 做了第一步：禁止 anthropic）。
4. **更强的可观测性链路**：为一次请求记录 attempt 决策链（skip/fail/success + reason），定位问题时更接近 AnyHub 的体验。

> 注：以上建议并不要求引入 AnyHub 的技术栈；更多是“策略与边界”的对齐。
