# API 手册

> 本页基于 `helloagents/wiki/api.md` 摘要整理。若你发现描述与代码行为不一致，请以代码为准并提 Issue/PR 修正文档。

## 概述

Realms 提供 OpenAI 兼容（Responses）与 Anthropic 兼容（Messages）的 API 入口，用于将客户端请求转发到上游，并在中转层提供最小可用的调度与 SSE 透传能力。

## 认证方式

- **下游访问（调用本服务，多用户）:** `Authorization: Bearer <user_api_key>`（可选兼容 `x-api-key`）
  - `user_api_key` 可在 Web 控制台 `/tokens` 创建与管理（示例：`sk_...`）
- **上游访问（本服务调用上游）:** 由管理后台配置并注入（OpenAI 兼容 API Key / Anthropic API Key / Codex OAuth 凭据）

## 常用端点

### [GET] /healthz

健康检查（公开），包含 DB 状态与构建信息（版本/构建时间）。

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
