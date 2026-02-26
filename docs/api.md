# API 手册

> 本页基于 `helloagents/wiki/api.md` 摘要整理。若你发现描述与代码行为不一致，请以代码为准并提 Issue/PR 修正文档。

## 概述

Realms 提供 OpenAI 兼容（Responses）与 Anthropic 兼容（Messages）的 API 入口，用于将客户端请求转发到上游，并在中转层提供最小可用的调度与 SSE 透传能力。

## 认证方式

- **下游访问（调用本服务，多用户）:** `Authorization: Bearer <user_api_key>`（可选兼容 `x-api-key`）
  - `user_api_key` 可在 Web 控制台 `/tokens` 创建与管理（示例：`sk_...`）
- **上游访问（本服务调用上游）:** 由管理后台配置并注入（OpenAI 兼容 API Key / Anthropic API Key / Codex OAuth 凭据）

### personal 模式补充（REALMS_MODE=personal）

- **管理 Key（必需）**：首次打开 `/login` 设置（对应 `POST /api/personal/bootstrap`），用于解锁管理面（`/api/admin/*`、`/api/channel*` 等）。
- **数据面 API Key（建议，可创建多个）**：通过 `POST /api/personal/keys` 创建，生成后仅返回一次明文；之后用这些 Key 调用数据面 `/v1/*`（`Authorization: Bearer ...` 或 `x-api-key`）。
  - 说明：数据面 API Key **不具备**管理权限（不能访问 `/api/admin/*`），且默认不使用“管理 Key”调用 `/v1/*`（避免把管理权限的 Key 分发给下游客户端）。

## 常用端点

### [GET] /healthz

健康检查（公开），包含 DB 状态与构建信息（版本/构建时间）。

### personal 模式：Key 管理（需要管理 Key）

> 说明：以下端点仅在 `REALMS_MODE=personal` 下挂载。  
> 认证：`Authorization: Bearer <管理 Key>`（或 `x-api-key`）。

#### [POST] /api/personal/keys

创建一个“数据面 API Key”（用于调用 `/v1/*`）。创建成功后 Key 明文只返回一次。

#### [GET] /api/personal/keys

列出已创建的“数据面 API Key”（仅返回 hint，不返回明文）。

#### [POST] /api/personal/keys/{key_id}/revoke

撤销一个“数据面 API Key”。

### Realms 扩展：Usage（按当前 API key）

> 说明：以下为 Realms 扩展端点（非 OpenAI 标准）。  
> 认证：同数据面一致（`Authorization: Bearer <user_api_key>` 或 `x-api-key`）。  
> 约束：仅允许查询**当前 key** 的用量，不支持 `token_id` / `token_ids` 参数。

#### [GET] /v1/usage/windows

按时间范围返回窗口汇总（请求数、Token、RPM/TPM、费用等）。

Query（可选）：
- `start` / `end`：`YYYY-MM-DD`（默认当天）
- `tz`：IANA 时区名（默认 `UTC`）

#### [GET] /v1/usage/events

列出用量事件（分页）。

Query（可选）：
- `limit`：1-500（默认 100）
- `before_id`：向前翻页游标
- `start` / `end` / `tz`：同上

#### [GET] /v1/usage/events/{event_id}/detail

获取用量事件详情（含 pricing breakdown）。

#### [GET] /v1/usage/timeseries

按小时/天聚合的时间序列。

Query（可选）：
- `start` / `end` / `tz`
- `granularity`：`hour|day`（默认 `hour`）

### OpenAI Responses

#### [POST] /v1/responses

OpenAI Responses create（支持 `stream=true` SSE 逐事件透传）。

#### [GET] /v1/responses/{response_id}

Retrieve stored Response（支持 query `stream=true` SSE 透传）。

#### [DELETE] /v1/responses/{response_id}

Delete stored Response。

#### [POST] /v1/responses/{response_id}/cancel

Cancel（仅对已登记对象生效）。

#### [GET] /v1/responses/{response_id}/input_items

List input items（仅对已登记对象生效）。

#### [POST] /v1/responses/compact

Compact（用于压缩输入上下文，行为由上游决定）。

#### [POST] /v1/responses/input_tokens

Input tokens（仅计算输入 tokens 的工具端点，行为由上游决定）。

> 安全/隔离：所有带 `{response_id}` 的拓展操作会先做本地“对象归属”校验：未登记或不属于当前用户时返回 404。  
> 限制：若该 Response 是通过 `codex_oauth` 上游创建的，上游当前不支持 `/v1/responses/{id}` 等拓展端点，Realms 会返回 501。

### OpenAI Chat Completions（stored）

#### [POST] /v1/chat/completions

Chat Completions create。若请求携带 `store=true`，Realms 会自动注入归属标记 `metadata.realms_owner`，用于后续 list/校验。

#### [GET] /v1/chat/completions

List stored chat completions。Realms 会强制按当前用户过滤（忽略用户自带的 metadata 过滤条件），并用本地登记做二次过滤兜底。

#### [GET] /v1/chat/completions/{completion_id}

Retrieve stored chat completion（仅对已登记对象生效）。

#### [POST] /v1/chat/completions/{completion_id}

Update metadata（仅对已登记对象生效；并强制保留归属 metadata）。

#### [DELETE] /v1/chat/completions/{completion_id}

Delete stored chat completion（仅对已登记对象生效）。

#### [GET] /v1/chat/completions/{completion_id}/messages

List messages for a stored chat completion（仅对已登记对象生效）。

### Models

#### [GET] /v1/models

列出已启用且有可用绑定的模型（Realms 托管模型目录）。

#### [GET] /v1/models/{model}

获取单个模型对象（与 `/v1/models` 输出口径一致）。

> 详细语义、failover 与字段策略请见项目源代码；若文档与代码不一致，以代码为准。
