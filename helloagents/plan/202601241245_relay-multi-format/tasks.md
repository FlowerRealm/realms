# 任务清单: relay-multi-format

目录: `helloagents/plan/202601241245_relay-multi-format/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 18
已完成: 4
完成率: 22%
```

---

## 任务列表

### 0. 调研与设计（对齐 new-api 行为）
- [√] 0.1 拉取并阅读 new-api：梳理 OpenAI/Claude/Gemini 处理链路（见 `.external/new-api/relay/*_handler.go`）
- [√] 0.2 梳理 Realms 现有 `/v1/responses`、`/v1/messages` 与 `upstream.Executor` 的转发/计费路径
- [√] 0.3 输出方案提案：`proposal.md`
- [√] 0.4 输出任务清单：`tasks.md`

### 1. 通用能力：按 selection 改写 Path/Body/Header
- [ ] 1.1 抽象 `OutboundRequest`（path/body/header）并改造 handler 的 `rewriteBody` 为 `rewriteRequest`
- [ ] 1.2 每次 failover 尝试对 `*http.Request` 做 clone 并覆盖 path/header
- [ ] 1.3 单测：同一个请求在不同 selection 下可以得到不同 path/body（至少覆盖 Gemini 的 path 改写）

### 2. 新增 Gemini 渠道类型（上游侧）
- [ ] 2.1 DB/Store：新增 `gemini` credential 表与读写（API Key 加密存储、hint、status、限额字段对齐）
- [ ] 2.2 Scheduler：支持列出 Gemini credentials 并生成 selection（新增 CredentialTypeGemini）
- [ ] 2.3 Upstream Executor：实现 Gemini 鉴权与 URL 拼接（`x-goog-api-key` 或 query key 策略二选一）
- [ ] 2.4 管理后台：渠道类型增加 `gemini`，并支持 Gemini credential 的新增/禁用/删除/显示 hint

### 3. 新增北向 Gemini API（入站侧，Phase A）
- [ ] 3.1 路由：注册 `POST /v1beta/models/{model}:generateContent` 与 `:streamGenerateContent`
- [ ] 3.2 Handler：实现 Gemini 请求的认证、配额预留、模型映射与调度 failover
- [ ] 3.3 Usage：支持从 `usageMetadata` 提取 tokens（流式/非流式）
- [ ] 3.4 单测：Gemini 非流式/流式各 1 个（含 usage 提取）

### 4. 多格式中转（Phase B）
- [ ] 4.1 定义最小“通用消息表示”（system/user/assistant + text）
- [ ] 4.2 转换：OpenAI Responses → Anthropic Messages（文本/角色映射 + max tokens 映射）
- [ ] 4.3 转换：OpenAI Responses → Gemini GenerateContent（contents/parts + generationConfig.maxOutputTokens）
- [ ] 4.4 转换：Anthropic Messages → OpenAI Responses（messages → input）
- [ ] 4.5 转换：Gemini GenerateContent → OpenAI Responses（contents → input）
- [ ] 4.6 调度集成：当 selection.ChannelType 与入站格式不一致时自动选择转换器
- [ ] 4.7 单测：每个转换方向至少 1 个正常用例 + 1 个边界用例

### 5. 可选增强（Phase C，对齐 new-api 配置能力）
- [ ] 5.1 默认过滤敏感/计费字段（`service_tier/safety_identifier` 等），并提供开关（全局或渠道级）
- [ ] 5.2 ParamOverride：提供 JSON 级 set/delete/move（存储结构与管理后台编辑）

### 6. 验证
- [ ] 6.1 运行测试：`go test ./...`

