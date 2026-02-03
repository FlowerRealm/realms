# 变更提案: Claude Code 支持（Anthropic Messages /v1/messages 中转）

## 需求背景

当前 Realms 的数据面仅提供 OpenAI 风格接口（`POST /v1/responses`、`GET /v1/models`）。  
而 Claude Code 采用 Anthropic Messages API（`POST /v1/messages` + SSE 流式），并依赖 tool use / thinking / 多模态等能力。

本变更目标是在 **不破坏既有 OpenAI 兼容接口** 的前提下：
1) 新增一套北向端点 `POST /v1/messages`；  
2) 新增 `anthropic` 上游类型，使其纳入现有的 scheduler/failover、配额、审计与故障日志体系；  
3) 让 Claude Code 可以把 Realms 当成 Anthropic Messages 兼容网关使用。

## 变更内容
1. **新增北向端点**：`POST /v1/messages`
2. **新增上游类型**：`anthropic`（channel/endpoint/credential），提供管理后台配置入口
3. **中转链路对齐**：
   - 模型路由：`model(public)` → `channel_models.upstream_model`（按渠道绑定模型决定上游模型名）
   - 鉴权注入：上游 `x-api-key` / `anthropic-version`（透传/默认）
   - SSE 透传：保持流式语义，写回后禁止 failover
4. **计费/用量提取**：从 Anthropic `usage` 中抽取 tokens（input/output + cache tokens），用于 quota 结算与调度 TPM 统计
5. **运维与排障**：沿用现有 proxy failure log / audit event 口径，便于定位渠道/凭据/错误类型

## 影响范围
- **模块:**
  - `internal/server`（路由注册 `/v1/messages`）
  - `internal/api/*`（新增 Messages handler 与 failover/SSE/计费口径）
  - `internal/upstream`（新增 Anthropic 上游请求构造与鉴权注入）
  - `internal/scheduler`（新增 anthropic credential 选择）
  - `internal/store`（新增 anthropic_credentials 与管理面读写）
  - `internal/admin`（支持创建 anthropic channel、管理 keys、配置限额）
  - `internal/middleware`（把 `/v1/messages` 识别为流式端点，避免 timeout 误伤）
- **API:**
  - `POST /v1/messages`（新增）
- **数据:**
  - 新增 `anthropic_credentials`（或同等结构）用于存储上游 key（明文 BLOB，沿用当前策略）

## 核心场景

### 需求: Claude Code 通过 /v1/messages 调用
**模块:** server/api/upstream

#### 场景: 非流式 messages
- Claude Code 请求 `POST /v1/messages`（`stream=false` 或不启用流式）能得到与 Anthropic 兼容的 JSON 响应
- 失败时返回 Anthropic 风格错误对象（至少不返回 OpenAI 的 `{error:{...}}` 结构）

#### 场景: 流式 messages（SSE）
- `stream=true` 时返回 `Content-Type: text/event-stream`，并能持续 flush（不被 gzip/缓冲破坏流式语义）
- SSE 一旦开始写回，不再 failover（避免部分写回导致协议破坏）

### 需求: 上游 429/5xx 可自动切换（failover）
**模块:** scheduler/api

#### 场景: 写回前可重试切换 key
- 上游返回 `429/5xx/timeout` 等可重试失败时，自动切换到同渠道/同分组下的其他 credential（或其他渠道）
- 失败重试不会导致配额被重复扣减（失败应 void / 不 commit）

### 需求: Anthropic usage 可用于计费与限额统计
**模块:** api/quota/scheduler

#### 场景: 提取 input/output/cache tokens
- 非流式：从响应 JSON 的 `usage` 提取 `input_tokens/output_tokens/cache_*_input_tokens`
- 流式：从 SSE 的 `data:` JSON 事件中提取 `usage`，用于 quota commit 与 TPM 统计

### 需求: 管理后台可配置 anthropic 上游
**模块:** store/admin

#### 场景: 管理员创建 anthropic channel 并配置多个 API key
- 可创建 `type=anthropic` 渠道并设置 base_url
- 可在渠道端点页新增/删除多个 anthropic API key，并可设置 sessions/rpm/tpm 限额

## 风险评估
- **风险:** SSE 被缓冲/压缩导致“流式变成一次性返回”
  - **缓解:** `/v1/messages` 纳入 stream-aware timeout；转发时禁用 `Accept-Encoding`；保持 `X-Accel-Buffering: no` 等策略与现有 SSE 透传一致
- **风险:** 下游鉴权/会话信息泄露到上游（Cookie/Authorization/x-api-key）
  - **缓解:** 上游请求构造阶段剥离敏感头，仅注入上游所需鉴权头
- **风险:** Anthropic usage 字段与现有口径不一致导致 TPM/计费统计偏差
  - **缓解:** 明确映射规则（input/output + cache_read/cache_creation 作为 cached input），并在测试中覆盖

