# 任务清单: Claude Code 支持（Anthropic Messages /v1/messages 中转）

目录: `helloagents/plan/202601211621_claude_code_messages/`

---

## 1. Store 与迁移（新增 anthropic 上游凭据）
- [√] 1.1 新增 `anthropic_credentials` 表迁移与索引，并在 `internal/store` 增加 CRUD，验证 why.md#核心场景-需求-管理后台可配置-anthropic-上游-场景-管理员创建-anthropic-channel-并配置多个-api-key
- [-] 1.2 扩展导入/导出（如已有 admin export/import 逻辑），覆盖 anthropic credentials，验证 why.md#核心场景-需求-管理后台可配置-anthropic-上游-场景-管理员创建-anthropic-channel-并配置多个-api-key（导出/导入默认不包含敏感字段，避免泄露 API Key）

## 2. Scheduler 支持 anthropic credential 选择
- [√] 2.1 在 `internal/scheduler/*` 增加 `CredentialTypeAnthropic` 并实现选择逻辑（limit + cooling + 统计口径），验证 why.md#核心场景-需求-上游-4295xx-可自动切换failover-场景-写回前可重试切换-key

## 3. Upstream executor 支持 Anthropic 请求构造
- [√] 3.1 在 `internal/upstream/executor.go` 支持 `anthropic`：构造 `/v1/messages` 请求、注入 `x-api-key/anthropic-version`、剥离敏感头，验证 why.md#核心场景-需求-claude-code-通过-v1messages-调用-场景-流式-messagessse
- [√] 3.2 更新 stream 识别（`/v1/messages`）避免 timeout 误伤，并补齐单测，验证 why.md#核心场景-需求-claude-code-通过-v1messages-调用-场景-流式-messagessse

## 4. 数据面 API：新增 POST /v1/messages
- [√] 4.1 在 `internal/server/app.go` 注册 `POST /v1/messages`，并实现 Messages handler（模型映射、failover、SSE 透传、错误格式），验证 why.md#核心场景-需求-claude-code-通过-v1messages-调用
- [√] 4.2 在 handler 中实现 Anthropic usage 抽取（含 cache tokens）并对接 quota commit/void 与调度 TPM 统计，验证 why.md#核心场景-需求-anthropic-usage-可用于计费与限额统计-场景-提取-inputoutputcache-tokens

## 5. 管理后台：支持 anthropic 渠道与 keys
- [√] 5.1 放开创建 `type=anthropic` Channel（`internal/admin/*` + templates），并在端点页支持新增/删除 API key 与限额配置，验证 why.md#核心场景-需求-管理后台可配置-anthropic-上游-场景-管理员创建-anthropic-channel-并配置多个-api-key

## 6. 安全检查
- [√] 6.1 执行安全检查（按 G9）：敏感头剥离、SSRF(base_url)、SSE 不压缩/不缓存、鉴权与权限边界

## 7. 文档更新
- [√] 7.1 更新 `helloagents/wiki/api.md`：新增 `/v1/messages` 说明（鉴权、流式、错误格式）
- [√] 7.2 更新 `helloagents/wiki/modules/realms.md`：补充 anthropic 上游类型与 Claude Code 配置指引

## 8. 测试
- [√] 8.1 增加 handler/executor/middleware 关键路径单测（覆盖流式/非流式与 failover 关键分支）
- [√] 8.2 运行 `go test ./...`（优先确保新增包与改动包覆盖）
