# 变更提案: 合并 Endpoint 页面与 Key 管理页面

## 需求背景

当前管理后台将 **Endpoint 配置**（Base URL）与 **Key 管理**（OpenAI Credentials）拆分在两个页面中。由于每个 Channel 仅允许配置 1 个 Endpoint，日常操作经常需要在两个页面之间来回跳转，增加了配置与排障成本。

本变更目标是把“Endpoint 配置”和“Key 管理”收敛到同一页面，降低操作路径；旧 Key 管理页不再保留。

## 变更内容

1. 在 `Channel Endpoint` 页面内直接展示并管理 OpenAI Keys（列表 / 新增 / 删除）
2. 删除旧的 OpenAI Credentials 页面与入口，统一在合并页完成操作
3. 更新 Channels 列表中的相关入口，默认进入合并页

## 影响范围

- **模块:** `internal/admin`、`internal/server`
- **文件:**
  - `internal/admin/server.go`
  - `internal/admin/templates/endpoints.html`
  - `internal/admin/templates/channels.html`
  - `internal/server/app.go`（如需，仅涉及路由/兼容入口层面）
- **API:**
  - `GET /admin/channels/{channel_id}/endpoints`（页面内容变化：新增 Keys 区块）
  - `POST /admin/endpoints/{endpoint_id}/openai-credentials`（保持：新增 Key）
  - `POST /admin/openai-credentials/{credential_id}/delete`（保持：删除 Key）
- **数据:** 无（仅展示/操作既有 `openai_compatible_credentials` 数据）

## 核心场景

### 需求: merge-endpoint-and-keys
**模块:** `internal/admin`

将 Endpoint 配置与 Key 管理合并到同一页面。

#### 场景: manage-keys-in-endpoint-page

打开 `GET /admin/channels/{channel_id}/endpoints`：
- 能看到 Endpoint 的 Base URL 配置与当前 Key 列表
- 能在同页添加 Key（加密入库，只展示 hint）
- 能在同页删除 Key（有不可恢复确认）
- 已有 `GET /admin/endpoints/{endpoint_id}/openai-credentials` 书签依然可用（自动跳转到合并页）

## 风险评估

- **风险:** 旧链接/导航入口失效  
  **缓解:** 保留旧 GET 路由并重定向；页面内按钮改为同页操作
- **风险:** UI 合并导致敏感信息泄露（展示明文 key）  
  **缓解:** 仅展示 `APIKeyHint`；不引入任何“查看明文 key”能力；继续使用 CSRF
- **风险:** Channel 类型差异（`codex_oauth` vs `openai_compatible`）处理不当  
  **缓解:** 页面按 `Channel.Type` 条件渲染：`openai_compatible` 显示 Keys；`codex_oauth` 保持账号管理入口
