# API 手册

> 本页基于 `helloagents/wiki/api.md` 摘要整理。若你发现描述与代码行为不一致，请以代码为准并提 Issue/PR 修正文档。

## 概述

Realms 提供 OpenAI 兼容（Responses）与 Anthropic 兼容（Messages）的 API 入口，用于将客户端请求转发到上游，并在中转层提供最小可用的调度与 SSE 透传能力。

## 认证方式

- **下游访问（调用本服务，多用户）:** `Authorization: Bearer <user_api_key>`（可选兼容 `x-api-key`）
- **上游访问（本服务调用上游）:** 由管理后台配置并注入（OpenAI 兼容 API Key / Anthropic API Key / Codex OAuth 凭据）

## 常用端点

### [GET] /healthz

健康检查（公开），包含 DB 状态与构建信息（版本/构建时间）。

### [GET] /api/version

构建信息（公开），用于页脚版本展示与排障定位。

### [POST] /v1/responses

OpenAI Responses create（支持 `stream=true` SSE 逐事件透传）。

> 详细语义、failover 与字段策略请见项目源代码与后续补充文档。
