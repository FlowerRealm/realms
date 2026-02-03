# 调研：Codex CLI 使用的协议形态（用于 API 中转/网关）

> 目标：确认 Codex CLI 实际对接上游模型时使用的“线协议”（wire API）与流式形态，以便实现兼容的中转服务。  
> 结论优先级：以 OpenAI 官方文档为准（因为 Codex CLI 会随版本演进）。  
> 调研日期：2026-01-13

---

## 1. 结论（TL;DR）

如果你的中转服务要给 **Codex CLI** 用，并且目标模型包含 **GPT-5-Codex**：
- **必须优先支持 OpenAI Responses API**（HTTP JSON）。官方已明确 GPT-5-Codex “可在 Responses API 使用”，并可通过 API Key 在 Codex CLI 中使用。  
- **流式输出形态为 SSE（text/event-stream）**，Codex CLI 配置项里存在 `stream_idle_timeout_ms` / `stream_max_retries` 等“流式连接”参数，指向的就是 SSE 流式语义。  
- Codex CLI 同时支持 `wire_api = "responses"` 与 `wire_api = "chat"`，但本项目仅支持 `wire_api="responses"`（`POST /v1/responses`）；如需对接本服务请在 Codex CLI 中选择 Responses wire API。

---

## 2. Codex CLI 如何选择 wire API（Responses / Chat）

Codex CLI 支持配置多个模型提供商（model providers），并通过 `wire_api` 指定对接协议：
- `wire_api = "responses"`：使用 **Responses API**
- `wire_api = "chat"`：使用 **Chat Completions API**（兼容用途）

相关配置在官方 “Configuration Reference” 中给出（字段：`model_providers.<id>.wire_api`）。  

**配置文件位置：** `~/.codex/config.toml`

---

## 3. 对中转服务的接口要求（建议的最小集合）

以 “Codex CLI + OpenAI Provider + Responses wire API” 为目标，建议你的中转服务至少实现：

### 3.1 HTTP 基础
- **Base URL**：形如 `http(s)://<your-proxy-host>/v1`（Codex CLI 文档对 `base_url`/`OPENAI_BASE_URL` 的示例均以 `/v1` 作为 API 根）
- **认证**：兼容 `Authorization: Bearer <token>`（Codex CLI 的 provider 通过 `env_key` 读取 key，最终以标准 OpenAI API 方式发起请求）

### 3.2 核心端点（Responses）
- `POST /v1/responses`
  - 非流式：返回 JSON
  - 流式：返回 `Content-Type: text/event-stream`，按 OpenAI Responses Streaming 事件序列持续输出

### 3.3 可选端点（兼容/体验）
- `GET /v1/models`：部分客户端会用来探测模型（Codex CLI 是否强依赖取决于版本实现；实现成本低，建议支持）
  - 说明：本项目不再提供 `POST /v1/chat/completions`，统一以 Responses 作为唯一数据面入口

---

## 4. SSE 流式：你需要“真的”实现流式

中转层最容易踩坑的是：把流式响应读完再一次性回写，导致 CLI 端表现为“卡住后整段输出”。

Codex CLI 配置里有以下与流式直接相关的参数（表明它在客户端侧会对流式进行超时/重试控制）：
- `stream_idle_timeout_ms`：SSE 流式空闲超时
- `stream_max_retries`：SSE 流式中断后的最大重试次数

因此中转实现要点（最小）：
- 透传 `text/event-stream`（不要对 SSE 做 gzip 响应压缩）
- 每个事件写入后及时 flush
- 上游到下游的“事件边界”保持一致（不要拼接/丢事件）

---

## 5. Codex CLI 指向中转的方式（配置入口）

Codex CLI 提供两种常见方式将 OpenAI provider 指向自建中转：

1) 使用环境变量覆盖 OpenAI base url（适配 LLM proxy/router）  
   - `OPENAI_BASE_URL=http(s)://<your-proxy-host>/v1`

2) 在 `~/.codex/config.toml` 中定义自建 provider（更灵活，可加 header/query_params）  
   - `model_providers.<id>.base_url = "http(s)://<your-proxy-host>/v1"`
   - `model_providers.<id>.wire_api = "responses"`
   - `model_providers.<id>.env_key = "<YOUR_ENV_KEY_NAME>"`

---

## 6. 参考资料（官方）

> 说明：下面仅列出“能决定协议形态”的官方入口，避免引用二手文章导致偏差。

- Codex changelog：GPT-5-Codex 可用于 Responses API，并可用 API Key 在 Codex CLI 使用  
  - https://developers.openai.com/codex/changelog/
- Codex config reference：`wire_api` 与流式参数（`stream_idle_timeout_ms` / `stream_max_retries`）  
  - https://developers.openai.com/codex/config-reference/
- Codex advanced config：支持 `OPENAI_BASE_URL` 覆盖与自定义 model providers  
  - https://developers.openai.com/codex/config-advanced/
- OpenAI Responses API streaming（SSE 事件流规范）  
  - https://platform.openai.com/docs/guides/streaming-responses
