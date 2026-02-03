# 变更提案: relay-multi-format

## 元信息
```yaml
类型: 功能增强
方案类型: implementation
优先级: P1
状态: 设计中
创建: 2026-01-24
```

---

## 1. 背景/问题

Realms 当前能力与约束：
- 北向接口：
  - `POST /v1/responses`（OpenAI Responses 兼容）
  - `POST /v1/messages`（Anthropic Messages 兼容）
- 上游渠道类型（`upstream_channels.type`）：
  - `openai_compatible` / `anthropic` / `codex_oauth`
- 调度基于 public model → 渠道绑定模型（`managed_models` + `channel_models/channel_model_bindings`），同一 public model 可以绑定到多个渠道。

现象与动机：
- 自定义 base_url 渠道在 `POST /v1/responses` 下可能返回 `{"detail":"Unsupported parameter: max_output_tokens"}`，导致“渠道可用但被误判不可用”。该兼容点已通过自动重试（`max_output_tokens` → `max_tokens`）修复（见 `internal/upstream/executor.go`）。
- 目前缺口：没有 Gemini 原生入口与 gemini 上游类型；并且当“入站格式 ≠ 上游渠道类型”时，无法像 new-api 一样做请求格式转换，导致跨渠道类型 failover 失败（例如：只有 `anthropic` 渠道时，`/v1/responses` 无法完成请求）。

本提案目标：对齐 new-api 的“信息处理 + 多格式中转”思路，在 Realms 现有调度/计费/日志框架内补齐 OpenAI/Anthropic/Gemini 三种格式的中转能力。

---

## 2. new-api 处理链路摘要（用于对齐行为）

说明：以下为“处理流程级别”的抄写摘要，便于对齐行为；对应源码位于本仓库本地目录 `.external/new-api/`。

### 2.1 OpenAI（TextHelper/Responses）
入口（示例）：
- `.external/new-api/relay/compatible_handler.go`（OpenAI ChatCompletions/Embeddings 等）
- `.external/new-api/relay/responses_handler.go`（OpenAI Responses）

主链路（抽象）：
1) `InitChannelMeta` 初始化渠道元信息（分组倍率/渠道设置/覆盖参数等）
2) 解析并 `DeepCopy` 请求对象（避免副作用）
3) `ModelMappedHelper`：origin model → upstream model
4) 处理 stream 相关选项（如 `stream_options.include_usage`）
5) 选择 adaptor（`GetAdaptor(info.ApiType)`）并 `Init(info)`
6) 选择“透传 body”或“转换请求”：
   - 透传：读取原始 body 直接发上游
   - 转换：`ConvertOpenAIRequest` →（可选）system prompt 注入/override → `RemoveDisabledFields`（`service_tier/store/safety_identifier`）→ `ApplyParamOverride`
7) `DoRequest` 发起上游请求
8) 错误处理：`RelayErrorHandler` + 状态码映射/重置
9) `DoResponse`：流式/非流式转发并提取 usage
10) `postConsumeQuota`：按 usage 计费/扣费

### 2.2 Anthropic（ClaudeHelper）
入口：
- `.external/new-api/relay/claude_handler.go`

主链路（抽象）：
1) `InitChannelMeta` → `DeepCopy`
2) `ModelMappedHelper`
3) 补齐默认 `max_tokens`（按模型配置）
4) （可选）ThinkingAdapter 与 system prompt 注入/override
5) adaptor：`ConvertClaudeRequest` → `RemoveDisabledFields` → `ApplyParamOverride`
6) `DoRequest` → 错误映射 → `DoResponse`
7) 按 usage 扣费（Claude 口径）

### 2.3 Gemini（GeminiHelper）
入口：
- `.external/new-api/relay/gemini_handler.go`
- `.external/new-api/relay/channel/gemini/relay-gemini-native.go`（usageMetadata 解析）

主链路（抽象）：
1) `InitChannelMeta` → `DeepCopy`
2) `ModelMappedHelper`
3) ThinkingAdapter（按渠道/模型配置）
4) system instructions 注入/override（Gemini 语义）
5) adaptor：`ConvertGeminiRequest` → `ApplyParamOverride`
6) `DoRequest` → 错误映射 → `DoResponse`
7) usage：优先从 Gemini 原生 `usageMetadata` 计算（prompt/candidates/total/thoughts）

### 2.4 Codex（与本问题相关的参数处理）
参考：
- `.external/new-api/relay/channel/codex/adaptor.go`

要点：
- Codex 通道会移除 `max_output_tokens` 等不被接受的字段，并对 `store` 等字段做强约束。

---

## 3. 目标（Realms 侧）

### 3.1 支持的入站格式（北向）
- OpenAI Responses：`POST /v1/responses`（保持）
- Anthropic Messages：`POST /v1/messages`（保持）
- Gemini GenerateContent（新增，最小集）：
  - `POST /v1beta/models/{model}:generateContent`
  - `POST /v1beta/models/{model}:streamGenerateContent`

### 3.2 支持的上游类型（渠道）
- `openai_compatible` / `anthropic` / `codex_oauth`（保持）
- `gemini`（新增，API Key 形态优先）

### 3.3 “多格式中转”能力
在 failover 过程中允许：
- 当 `InboundFormat != sel.ChannelType` 时，按规则进行“路径+body”改写/转换后再请求上游，而不是直接失败并跳过该渠道。

---

## 4. 方案设计

### 4.1 引入“按 selection 改写请求”的中转层（对齐 new-api adaptor 思路）

当前 Realms 的一次 proxy 尝试只能改写 body（通过 `rewriteBody(sel)`），无法改写 URL path；这会阻塞 Gemini（model 在 path）与跨格式转换（不同上游需要不同 endpoint path）。

改造方向：
- 在 handler 内，将“对每个 selection 的请求改写”升级为返回一个 `OutboundRequest`：
  - `Path`（必要时覆盖 `r.URL.Path`）
  - `Body`（JSON bytes）
  - `Header` 覆盖（可选：如 Gemini/Anthropic/OpenAI 的特定 header）
- 每次尝试时对下游 `*http.Request` 做轻量 clone，替换 path/header，再交给 `upstream.Executor` 发送。

> 该设计尽量保持 `upstream.Executor` 的职责边界：Executor 仍只关心 base_url 拼接与鉴权注入；“不同格式如何构造请求”交给上层中转层完成。

### 4.2 入站格式识别（按 path）
- `/v1/responses` → `InboundOpenAIResponses`
- `/v1/messages` → `InboundAnthropicMessages`
- `/v1beta/models/{model}:generateContent` → `InboundGeminiGenerateContent`
- `/v1beta/models/{model}:streamGenerateContent` → `InboundGeminiStreamGenerateContent`

### 4.3 模型映射（沿用现有 SSOT）
继续以 `managed_models` + `channel_models/channel_model_bindings` 为 SSOT：
- OpenAI/Anthropic：改写 body 的 `model` 字段为该渠道绑定的 `upstream_model`
- Gemini：改写 URL path 中的 `{model}` 为该渠道绑定的 `upstream_model`

### 4.4 Token 字段映射与默认值
对齐三种格式的“最大输出”字段：
- OpenAI Responses：`max_output_tokens`
- Anthropic Messages：`max_tokens`
- Gemini：`generationConfig.maxOutputTokens`

默认值来源仍使用 `limits.default_max_output_tokens`，但注入位置随入站格式而变。

跨格式转换时：
- 以“入站 max 输出”作为统一预算基准，转换到目标格式字段（类似 new-api：`max_tokens/max_completion_tokens → max_output_tokens` 的思路）。

### 4.5 Usage 提取与计费（补齐 Gemini）
现状：
- Realms 目前仅递归查找 JSON 中的 `usage` map（适配 OpenAI/Anthropic），无法识别 Gemini 的 `usageMetadata`。

补齐：
- 扩展 usage 提取逻辑：
  - OpenAI/Anthropic：沿用 `usage.*`（现有）
  - Gemini：支持 `usageMetadata.promptTokenCount / candidatesTokenCount / totalTokenCount / thoughtsTokenCount`，并将 `thoughtsTokenCount` 计入 output（对齐 new-api 的做法）
- 流式（SSE）：在 `PumpSSE` 的 `OnData` 中按 provider 解析 usage；非流式在完整 body 上解析 usage。

### 4.6 分阶段交付（降低一次性风险）

Phase A（最小可用）：
- 新增 gemini 渠道类型与北向 Gemini generateContent/streamGenerateContent 的“同格式直通代理”（不做跨格式转换）。

Phase B（多格式中转）：
- 在 `/v1/responses` 与 `/v1/messages` 的 failover 尝试中加入跨格式转换：
  - `OpenAI Responses -> Anthropic Messages`
  - `OpenAI Responses -> Gemini GenerateContent`
  - `Anthropic Messages -> OpenAI Responses`
  - `Gemini GenerateContent -> OpenAI Responses`（用于统一给 OpenAI 客户端的 fallback）
- 先覆盖纯文本 + 基础角色（system/user/assistant），工具调用/多模态先 best-effort 或延后。

Phase C（可选增强，对齐 new-api 配置能力）：
- `RemoveDisabledFields`（默认过滤 `service_tier/safety_identifier` 等）
- `ParamOverride`（JSON 级别 set/delete/move 等，作为渠道设置）

---

## 5. 风险与边界

- 跨格式转换的语义差异（tools、函数调用、多模态、结构化输出）无法保证 100% 对齐：Phase B 先保证“可用/可 failover”，再逐步扩展。
- Gemini 上游存在 Generative Language 与 Vertex AI 两套鉴权与 URL 体系：本期优先支持 API Key 形态；Vertex 作为后续扩展。
- URL path 改写会影响审计与 usage 记录的 endpoint 字段口径：需要明确记录“北向 endpoint”与“上游 endpoint”区别（目前 usage_events 已有 upstream_* 字段，可沿用）。

---

## 6. 验收标准

- [ ] Phase A：Gemini 原生 `generateContent/streamGenerateContent` 可用；usage 从 `usageMetadata` 解析并落库（usage_events）。
- [ ] Phase B：当某 public model 仅绑定 `anthropic/gemini` 渠道时，`POST /v1/responses` 仍可成功完成请求（通过转换）；反向同理（`/v1/messages` → `openai_compatible/gemini`）。
- [ ] 兼容性：自定义 base_url 渠道遇到 `Unsupported parameter: max_output_tokens` 时可自动重试成功（已完成）。
- [ ] `go test ./...` 通过。

