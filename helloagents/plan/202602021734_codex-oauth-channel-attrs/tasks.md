# 任务清单：Codex OAuth 去“内置字段化”，改为 Channel 属性并并列管理

> 状态符号：`[ ]` 待执行 / `[√]` 已完成 / `[X]` 失败 / `[-]` 跳过

## A. 数据模型与存储（Channel 属性）

- [-] 扩展 `store.UpstreamChannelSetting`：新增 `codex_oauth` 子对象结构（client_id/authorize_url/token_url/scope/prompt）
  - 说明：需求调整为“OAuth 客户端参数内置默认值”，不对外配置，不落库
- [-] 补齐 store 侧读写：更新/读取 setting（复用现有 `UpdateUpstreamChannelNewAPISetting`），并提供 endpoint→channel→setting 的获取辅助函数
  - 说明：同上，不需要 endpoint→channel→setting 取 OAuth 参数

## B. 移除全局 CodexOAuth 配置（Config 字段）

- [√] 删除 `internal/config/config.go` 的 `CodexOAuthConfig`/`Config.CodexOAuth`/`REALMS_CODEX_OAUTH_*` 覆盖逻辑
- [√] `internal/codexoauth`：引入独立 `codexoauth.Config`（替代依赖 `internal/config`），重构 `Client/Flow`
- [√] `internal/server/app.go`：OAuth Flow/Quota Refresh 不再依赖全局 enable；改为使用内置默认 OAuth 参数
- [√] `internal/upstream/executor.go`：token refresh 不再使用全局 client；改为使用内置默认 `token_url/client_id`

## C. 调度层透传（可选但推荐）

- [-] `internal/scheduler/scheduler.go`：为 `codex_oauth` 分支在 `Selection` 中透传必要 refresh 参数（至少 token_url/client_id）
  - 说明：OAuth 参数不再可配置，无需透传
- [-] 更新 `internal/upstream/executor_test.go` / `internal/scheduler/scheduler_test.go` 覆盖新字段
  - 说明：同上

## D. 管理面并列化（API + SPA）

- [√] `router/channels_api_routes.go`：允许创建/编辑/删除 `codex_oauth`（移除“内置”限制；保留“不支持测试/不支持 key”的合理限制）
- [-] `router/channels_api_routes.go`：`updateChannelSettingHandler` 扩展以支持写入 `setting.codex_oauth`
  - 说明：OAuth 客户端参数不对外配置，删除该接口能力
- [√] `web/src/pages/admin/ChannelsPage.tsx`：创建渠道类型支持 `codex_oauth`；移除“（内置）”标签与禁用逻辑
- [-] `web/src/api/channels.ts`：扩展 `UpstreamChannelSetting` 类型以包含 `codex_oauth`（用于回填与保存）
  - 说明：同上，不再暴露该字段

> 可选增强（若本次要做到可用闭环）：
> - [-] 增加“Codex 账号”tab：授权按钮（调用后端 start API）+ 账号列表/禁用/删除
> - [-] 对 callback 页的 postMessage/localStorage 通知在 SPA 侧补齐监听与刷新

## E. 迁移与回填

- [-] SQLite：更新 `internal/store/schema_sqlite.sql` 的 codex seed，补齐默认 `setting.codex_oauth`
  - 说明：不再落库 OAuth 参数，无需 seed/backfill
- [-] SQLite：`internal/store/sqlite_schema.go` 增加 backfill（旧库 codex channel setting 为空时补齐默认值）
  - 说明：同上
- [-] MySQL：新增 migration 对历史 codex channel 做 backfill（仅空值时写入默认 JSON）
  - 说明：同上

## F. 文档与验证

- [√] 更新 `README.md`/`docs/*`：移除“内置 Codex OAuth”描述，改为“Codex OAuth 作为普通渠道类型配置”
- [√] 运行并通过：`go test ./...`
- [√] 前端：`cd web && npm run build`（如仓库已有 lint 约束则一并跑）
- [√] 知识库：`helloagents/CHANGELOG.md` 记录本次结构调整（实现完成后）
