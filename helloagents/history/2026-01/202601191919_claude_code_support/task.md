# 任务清单: Realms 接入 Claude Code（参考 claude-code-hub）

目录: `helloagents/plan/202601191919_claude_code_support/`

---

## 1. 数据面接口（Claude Code）
- [ ] 1.1 新增 `internal/api/anthropic/handler.go`：实现 `POST /v1/messages` 的请求解析（model/stream）与基础转发框架，验证 why.md#需求-claude-code-可直接接入-realms
- [ ] 1.2 在 `internal/server/app.go` 注册 `POST /v1/messages` 路由，复用现有 `apiChain` 中间件，验证 why.md#需求-claude-code-可直接接入-realms
- [ ? ] 1.3 观测 Claude Code 实际请求，如包含 `POST /v1/messages/count_tokens` 则补齐接口与转发（否则跳过），验证 why.md#需求-claude-code-可直接接入-realms

## 2. 模型与上游绑定（避免误路由）
- [ ] 2.1 在 `internal/api/anthropic/handler.go` 中复用 managed model 校验：模型必须启用，否则返回明确错误，验证 why.md#需求-管理员可控制哪些上游承载-claude-code以及-claude-模型如何映射
- [ ] 2.2 在 `internal/api/anthropic/handler.go` 中复用渠道模型绑定：按模型 bindings 构造 `AllowChannelIDs`，并在选中渠道后重写 payload.model 为上游模型名，验证 why.md#需求-管理员可控制哪些上游承载-claude-code以及-claude-模型如何映射
- [ ] 2.3 复用 `scheduler.GroupRouter` + failover 策略：仅在未写回时 failover，SSE 开始写回后禁止 failover，验证 why.md#需求-claude-code-可直接接入-realms

## 3. 上游执行器兼容（Anthropic 鉴权与流式）
- [ ] 3.1 修改 `internal/upstream/executor.go`：将 `/v1/messages` 纳入流式识别（基于 Accept 或 payload.stream），避免对上游流式请求施加非流式超时，验证 why.md#需求-claude-code-可直接接入-realms
- [ ] 3.2 修改 `internal/upstream/executor.go`：在 OpenAICompatible credential 注入时，同时设置 `Authorization: Bearer <key>` 与 `x-api-key: <key>`，并保持下游鉴权不透传，验证 why.md#需求-claude-code-可直接接入-realms

## 4. 用量与计费（usage 提取）
- [ ] 4.1 在 `internal/api/anthropic/handler.go` 非流式分支：从 JSON `usage` 提取 input/output tokens 并 Commit，验证 why.md#需求-用量成本可追踪并可计费
- [ ] 4.2 在 `internal/api/anthropic/handler.go` SSE 分支：复用 `upstream.PumpSSE` 的 OnData 钩子提取 usage 并 Commit，验证 why.md#需求-用量成本可追踪并可计费
- [ ] 4.3 明确“未知模型”的策略：默认严格（模型不存在则拒绝/无法计费），如需要更接近 claude-code-hub 的行为再单独加开关，验证 why.md#风险评估

## 5. 文档与控制台
- [ ] 5.1 更新 `README.md`：补充 Claude Code 的 `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` 配置示例与前置条件（模型/绑定），验证 why.md#需求-claude-code-可直接接入-realms
- [ ] 5.2 更新 `internal/web/templates/*`（或新增页面）：提供 Claude Code 配置指引（类似 usage-doc），验证 why.md#需求-claude-code-可直接接入-realms
- [ ] 5.3 更新 `helloagents/wiki/api.md`：补充新增的 `/v1/messages` 接口说明与鉴权方式（落地实现后），验证 why.md#变更内容

## 6. 安全检查
- [ ] 6.1 执行安全检查：输入校验、鉴权边界、敏感信息（上游密钥）不泄露、SSRF 边界不扩大、failover 不产生重复计费，验证 why.md#风险评估

## 7. 测试
- [ ] 7.1 为 `internal/api/anthropic/handler.go` 增加单元测试（stream/non-stream、绑定重写、错误分支），并运行 `go test ./...`
- [ ] 7.2 为 `internal/upstream/executor.go` 的变更增加回归测试（`/v1/messages` 流式识别与 header 注入），并运行 `go test ./...`

