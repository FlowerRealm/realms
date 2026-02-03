# 技术设计: Claude Code 支持（Anthropic Messages /v1/messages 中转）

## 技术方案

### 核心技术
- Go `net/http`（沿用现有服务结构）
- 现有三层调度/失败切换：`internal/scheduler` + `internal/upstream` + `internal/api`
- SSE 透传工具：`internal/upstream/PumpSSE`（大行 buffer、ping、idle timeout、安全并发写）
- MySQL 迁移：`internal/store/migrations/*.sql`

### 实现要点

#### 1) 数据模型与上游类型
- 新增 `store.UpstreamTypeAnthropic = "anthropic"`
- 新增 `anthropic_credentials` 表（结构对齐 `openai_compatible_credentials`）：
  - `endpoint_id` 外键关联
  - `api_key_enc` 明文 BLOB（沿用当前“明文入库”的边界）
  - `limit_sessions/limit_rpm/limit_tpm` 作为账号级限额
- Store 增加 CRUD：
  - `ListAnthropicCredentialsByEndpoint`
  - `CreateAnthropicCredential`
  - `GetAnthropicCredentialSecret`
  - `UpdateAnthropicCredentialLimits`
  - `DeleteAnthropicCredential`

#### 2) Scheduler：选择 anthropic credential
- `internal/scheduler` 增加 `CredentialTypeAnthropic`
- 在 `selectCredential()` 中按 `ch.Type == anthropic`：
  - 过滤 `status=1`、冷却/限额（sessions/rpm/tpm）
  - 与 OpenAI/Codex 一致：用 `CredentialKey = "anthropic:<id>"` 参与 cooling/TPM/RPM 统计

#### 3) Upstream executor：构造 Anthropic 上游请求
- `internal/upstream/executor.go` 增加 `CredentialTypeAnthropic` 分支：
  - 允许的目标路径：首版仅支持 `/v1/messages`（避免误转发其他路径）
  - 透传必要头：
    - `anthropic-version`：优先使用下游同名 header；缺失则默认 `2023-06-01`
    - `anthropic-beta`：按下游透传
  - 注入鉴权头：
    - `x-api-key: <upstream_api_key>`
  - 与现有逻辑一致剥离敏感头：下游 `Authorization/x-api-key/Cookie` 等不能透传
- 超时策略：
  - `internal/middleware/stream_timeout.go`：把 `/v1/messages` 识别为可流式端点
  - `internal/upstream/executor.go:isStreamRequest`：把 `/v1/messages` 纳入“流式请求”识别，避免 request-level timeout 误杀长连接

#### 4) 北向 /v1/messages handler：failover + SSE + quota
- 在 `internal/server/app.go` 注册 `POST /v1/messages`
- Handler 设计对齐现有 `openai/handler.go`：
  - TokenAuth → 读取 cached body → JSON decode（仅提取 `model/stream/max_tokens` 与 routeKey）
  - 模型路由：
    - 非 passthrough：校验 managed model 启用 + 读取 `channel_models` 绑定，构建 `AllowChannelIDs` 白名单，并在选中渠道后把 `payload.model` 改写为对应 `upstream_model`
    - passthrough：仅在需要计费时要求模型存在（保持现有约束）
  - 渠道类型约束（首版）：
    - `RequireChannelType = anthropic`，避免在未实现“Claude→OpenAI”转换前错误路由到 openai_compatible
  - SSE：
    - 识别 `stream=true` 或上游返回 `text/event-stream`
    - 使用 `upstream.PumpSSE` 透传，并在 hooks.OnData 中解析 JSON，提取 usage tokens
  - 非流式：
    - readLimited 响应体并转发；解析 usage tokens
  - quota：
    - Reserve 使用 `max_tokens` 作为 MaxOutputTokens 口径
    - 成功 commit，失败 void（保持与 `/v1/responses` 一致）

## 架构决策 ADR

### ADR-008: /v1/messages 首版采用原生 Anthropic 上游直通（不做跨协议统一转换）
**上下文:** `/v1/messages` 可以选择“统一转换为内部 Responses 语义再转发”，也可以“原生直通 Anthropic 上游”。  
**决策:** 首版仅支持路由到 `anthropic` 上游类型，保持请求/响应最大程度透传，仅做模型映射、鉴权注入、SSE 透传与用量抽取。  
**理由:** Messages 协议语义复杂（tool_use/thinking/多模态），强行转换容易引入语义偏差与兼容性坑；直通实现最简单、风险最小。  
**替代方案:** Claude→OpenAI（Responses/ChatCompletions）统一转换 → 拒绝原因: 转换面太大且难以在短期内覆盖 Claude Code 的全部边界。  
**影响:** 需要在管理后台明确“/v1/messages 仅选择 anthropic 渠道”；如未来确需跨协议，可在新 ADR 下分阶段引入转换器。

## API 设计

### [POST] /v1/messages
- **认证:** `Authorization: Bearer <rlm_...>` 或 `x-api-key: <rlm_...>`（沿用 Realms 数据面 token；并非 Anthropic 官方 key）
- **请求/响应:** 尽量透传 Anthropic Messages 语义；Realms 仅做 `model` 映射与必要头注入
- **流式:** `stream=true` 时透传 SSE（`text/event-stream`）

## 数据模型（计划）

```sql
-- anthropic_credentials（示意，最终以迁移为准）
-- id BIGINT PK
-- endpoint_id BIGINT NOT NULL
-- name VARCHAR(128) NULL
-- api_key_enc BLOB NOT NULL
-- api_key_hint VARCHAR(32) NULL
-- status TINYINT NOT NULL DEFAULT 1
-- limit_sessions/limit_rpm/limit_tpm INT NULL
-- last_used_at DATETIME NULL
-- created_at/updated_at DATETIME NOT NULL
```

## 安全与性能
- **安全:**
  - 下游敏感头剥离；上游仅注入必须鉴权头（`x-api-key`）
  - base_url 继续走 `ValidateBaseURL`（防 SSRF）
  - request body 继续受 `BodyCache + MaxBodyBytes` 限制
- **性能:**
  - 仅解析少量字段，不对消息内容做深度改写
  - SSE 透传使用现有 pump（大行 buffer + idle timeout）

## 测试与部署
- **测试:**
  - executor：anthropic header 注入与 path 约束
  - middleware：`/v1/messages` 的 stream timeout 分支
  - handler：非流式/流式的用量抽取与 quota commit/void（用 httptest + fake upstream）
- **部署:**
  - 管理后台新增 anthropic channel 与 key 后，绑定模型即可对外提供 `/v1/messages`

