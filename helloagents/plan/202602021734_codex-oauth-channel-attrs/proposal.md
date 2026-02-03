# 变更提案：将 Codex OAuth 从“内置字段”改为“渠道属性”（与 OpenAI/Anthropic 并列）

目标：把 Codex OAuth 的配置与生命周期从“全局内置（Config 字段 + 内置 Channel 特判）”收敛为“普通上游渠道的一种类型”，与 `openai_compatible` / `anthropic` 并列；同时 **OAuth 客户端参数使用内置默认值**（不再作为全局配置字段，也不要求用户填写/落库）。

---

## 1. 背景与现状（代码事实）

当前 Codex OAuth 存在两层“内置”语义，导致它与 OpenAI/Anthropic 渠道不一致：

1) **全局内置字段：** `internal/config/config.go` 中 `Config.CodexOAuth` + `REALMS_CODEX_OAUTH_*` 环境变量覆盖；`internal/server/app.go` 与 `internal/upstream/executor.go` 用该字段来决定是否创建 Codex OAuth client/flow。
2) **内置渠道特判：** `codex_oauth` 渠道由迁移/SQLite schema seed 写入，并在管理面被强制标记为“内置不可创建/编辑/删除/测试/排序”等（`router/channels_api_routes.go`、`web/src/pages/admin/ChannelsPage.tsx`、`internal/store/admin_export_import.go` 等）。

这与 README “创建 Channel → 发起 OAuth 授权”的描述也产生了不一致（当前 SPA 甚至不允许创建 `codex_oauth`）。

---

## 2. 目标状态（期望行为）

1) `codex_oauth` 与 `openai_compatible` / `anthropic` 一样：
   - 允许创建/编辑/删除/排序（按现有权限模型 root session）。
   - `base_url` 作为 endpoint 字段可配置（默认值可预填）。
2) Codex OAuth 的 OAuth 客户端参数 **不再来自全局 Config 字段**，改为使用 **内置默认值**（不对外配置，不落库）。
3) 数据面/调度层仍保持现状：`codex_oauth_accounts` 仍按 endpoint 绑定与选取；token refresh 使用内置默认 `token_url/client_id`。

---

## 3. 设计方案（推荐实现）

### 3.1 OAuth 客户端参数不落库

不新增表、不加新列：OAuth 客户端参数（`client_id/authorize_url/token_url/scope/prompt`）使用内置默认值，不对外暴露配置入口。

说明：
- `redirect_uri` 固定走本机 `localhost`（用于模拟 codex 登录），并由服务端根据自身监听端口推导为 `http://localhost:{port}/auth/callback`。

### 3.2 移除全局 `Config.CodexOAuth` 依赖

- `internal/config/config.go`：删除 `CodexOAuthConfig` 与 `Config.CodexOAuth` 字段及 `REALMS_CODEX_OAUTH_*` 覆盖逻辑。
- `internal/codexoauth`：引入独立的 `codexoauth.Config`（不依赖 `internal/config`），Client/Flow 使用该类型。
- `internal/server/app.go`：
  - Codex OAuth Flow 不再由 `cfg.CodexOAuth.Enable` 决定“是否存在”，改为始终存在（或以“是否配置/是否有 codex_oauth channel”惰性启用）。
  - Flow 在 `Start/Callback/Complete` 时使用内置默认 OAuth 参数并执行 OAuth。
- `internal/upstream/executor.go` 与 `internal/server/app.go` 的 quota/refresh 逻辑：
  - token refresh 不再使用全局 client，统一使用内置默认 `token_url/client_id` 刷新。

### 3.3 调度层（scheduler）补充必要字段（可选但推荐）

由于 OAuth 客户端参数不再可配置，因此无需在调度层透传 `token_url/client_id`。

### 3.4 管理面 API 与 SPA 对齐

管理面需要把 `codex_oauth` 当作普通类型处理：

- `router/channels_api_routes.go`：
  - `createChannelHandler`：允许 `type == codex_oauth`（创建 endpoint；忽略 key）。
  - `updateChannelHandler`/各种 `updateChannel*Handler`：移除“内置不允许编辑”的 codex 特判。
  - `deleteChannelHandler`：移除“内置不允许删除”的特判。
  - `testChannelHandler`：仍可保留“不支持测试”的限制（与现状一致）。
- `web/src/pages/admin/ChannelsPage.tsx`：
  - 创建渠道类型下拉允许选择 `codex_oauth`，并移除“（内置）”文案与禁用逻辑。
  - 不提供 OAuth 客户端参数配置表单（内置默认值）。

> 备注：当前仓库缺少 Codex OAuth “账号列表/授权按钮”等 SPA 页面逻辑（Flow 也未被任何 API 调用）。本提案聚焦于“配置归属与渠道并列”，账号管理可作为后续增量，但会在任务清单中给出可选项。

### 3.5 导入导出（admin_export_import）去特判

`internal/store/admin_export_import.go` 目前假设全库只有 1 个 `codex_oauth` channel（按 type 唯一定位）。
目标态应与其他渠道一致：用 `(type,name)` 定位并允许多条 `codex_oauth`（至少不再强依赖“唯一内置”）。

---

## 4. 迁移策略

由于本次是“直接修改、不保留向后兼容”的重构（不再从 env 读取 Codex OAuth 参数），需要确保数据库中已有 codex channel 的 setting 被补齐，否则 OAuth 将无法发起。

推荐策略：
- **SQLite：** 更新 `internal/store/schema_sqlite.sql` 的 seed，为 `codex_oauth` channel 写入默认 `setting.codex_oauth`；并在 `EnsureSQLiteSchema` 增加一次性 backfill（对旧库为空 setting 的 codex channel 注入默认值）。
- **MySQL：** 新增迁移（如 `00xx_codex_oauth_channel_setting_backfill.sql`），对 `type='codex_oauth' AND (setting IS NULL OR setting='')` 的记录写入默认 JSON。

---

## 5. 风险与回滚

- 风险：若部署方 `site_base_url/public_base_url` 误配，OAuth 回调 `redirect_uri` 将错误 → 授权失败。
  - 缓解：在 UI/接口返回中显式展示当前推导出的 callback URL（只读提示），并在错误页提示检查 `site_base_url/public_base_url`。
- 回滚：保留 DB 中 `setting.codex_oauth` 不影响回滚；若需要临时回退到旧版全局 env，可在回滚分支恢复 `Config.CodexOAuth` 的读取。

---

## 6. 验收标准

1) 后端：不再存在 `Config.CodexOAuth` 字段与 `REALMS_CODEX_OAUTH_*` 读取逻辑；Codex OAuth 相关逻辑只从 channel setting 中取值。
2) 管理面：可以创建/编辑/删除 `codex_oauth` channel；不再显示“内置”且不再有 codex 特判限制（除“测试不支持”外）。
3) 导入导出：不再依赖“唯一内置 codex_oauth”，且导出中能携带 `setting.codex_oauth`（不包含账号 token）。
4) 测试：`go test ./...` 通过；相关单测更新覆盖：
   - scheduler 选择 codex 账号时能带上必要配置（若采用 Selection 透传方案）
   - OAuth 授权/回调流程在缺失配置时给出明确错误
