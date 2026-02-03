# 变更提案: Realms 接入 Claude Code（参考 claude-code-hub）

## 需求背景

Realms 当前定位为「OpenAI 风格 API 统一中转服务」（数据面）+「Web 控制台/管理后台」（控制面），并已覆盖：
- 数据面：`/v1/responses`、`/v1/chat/completions`、`/v1/models`
- 控制面：用户注册/登录、Token 管理、上游渠道/端点/凭证管理、用量统计、配额/计费等

但它目前**不提供** Anthropic/Claude Code 直接可用的接入路径（例如 `POST /v1/messages`），也缺少面向 Claude Code 的一键配置指引（`ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN`）。

`zsio/claude-code-hub` 提供了一个与我们目标非常接近的参考实现：把 `/v1/*` 当作「统一代理入口」，用 DB 做多租户鉴权、供应商路由、请求记录与用量统计，并提供面向 Claude Code 的配置文档页面。

因此，本方案目标是：**在不推翻 Realms 现有架构（尤其是调度/计费/安全）前提下**，补齐 Claude Code 所需的关键能力，使 Realms 成为 Claude Code 的可选 `ANTHROPIC_BASE_URL`。

## 变更内容

1. 增加 Claude Code 关键数据面接口（最小集）并实现上游转发、SSE 透传与 failover 策略。
2. 复用 Realms 现有的「模型目录 + 渠道绑定 + 调度器 + 计费/配额」体系，实现可控、可计费、可观测的 Claude Code 使用路径。
3. 在控制台补充 Claude Code 的配置指引（类似 claude-code-hub 的 usage-doc 页面目标）。

## 影响范围

- **模块**
  - `internal/server`：新增路由绑定（数据面）
  - `internal/api`：新增 Anthropic/Claude Code handler（或在现有 handler 中扩展）
  - `internal/upstream`：补齐 Anthropic 相关的 header 注入与流式识别（尽量不影响 OpenAI 路径）
  - `internal/admin` / `internal/web`：补充 Claude Code 使用说明（UI/模板）
  - `helloagents/wiki/*`：补充 API 文档与运维说明（在真正落地时更新）

- **文件（预估）**
  - 新增：`internal/api/anthropic/*`（或类似路径）
  - 修改：`internal/server/app.go`
  - 修改：`internal/upstream/executor.go`
  - 修改：`internal/web/templates/*`、`README.md`（文档）

## 核心场景

### 需求: Claude Code 可直接接入 Realms
**模块:** 数据面 / 代理

#### 场景: Claude Code 使用 Realms 作为 `ANTHROPIC_BASE_URL`
- 用户在 Realms 控制台创建数据面 Token（`Authorization: Bearer rlm_...` 或 `x-api-key`）
- 在 Claude Code 配置中设置：
  - `ANTHROPIC_BASE_URL=http(s)://<realms-host>`
  - `ANTHROPIC_AUTH_TOKEN=<rlm_...>`
- Claude Code 发起 `POST /v1/messages`（包含 `model`、`stream` 等字段）：
  - Realms 完成鉴权
  - Realms 选择可用上游并转发请求
  - 若为 SSE：逐事件转发并 flush（避免假流式）
  - 若上游错误且尚未写回：按策略 failover 到其他渠道

### 需求: 管理员可控制「哪些上游承载 Claude Code」以及「Claude 模型如何映射」
**模块:** 控制面 / 上游与模型管理

#### 场景: 配置 Claude 模型与上游渠道绑定
- 管理员在「模型管理」中创建/启用 Claude 模型（Public ID 使用 Claude Code 会发送的模型名）
- 在「渠道模型绑定」中把该模型绑定到承载 Anthropic API 的渠道，并配置上游模型名（可同名或按渠道差异映射）
- 运行时：数据面根据模型绑定限制可用渠道，避免把 Anthropic 请求误路由到 OpenAI 渠道

### 需求: 用量/成本可追踪并可计费
**模块:** 用量 / 配额

#### 场景: 从 Anthropic 响应中提取 usage 并落账
- 非流式：从响应 JSON 的 `usage` 字段提取 token 用量并 Commit
- 流式：从 SSE 的事件 `data:` 中提取包含 `usage` 的 JSON 片段并累计，最终 Commit
- 若模型无定价：按 Realms 现有策略（严格/宽松）处理（在技术方案中明确）

## 风险评估

- **风险: Claude Code 实际调用的接口集合不止 `/v1/messages`**
  - **缓解:** 先用最小集跑通；再通过访问日志/回放补齐（优先补 `count_tokens` 等高频接口）。

- **风险: Anthropic 上游对鉴权 header 兼容性差（Authorization vs x-api-key）**
  - **缓解:** 上游注入同时设置 `Authorization: Bearer <key>` 与 `x-api-key: <key>`，并透传 `anthropic-version` 等必要 header（以不破坏 OpenAI 路径为前提）。

- **风险: 计费模型不一致**
  - **缓解:** 复用 Realms 的 managed model pricing；对「未知模型」明确策略（推荐严格，必要时提供开关）。

