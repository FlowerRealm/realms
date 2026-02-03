# 变更提案: 管理后台可配置 Chat 联网搜索

## 需求背景

目前 `/chat` 的“联网搜索（API 请求）”需要通过配置文件 `config.yaml`（或环境变量）启用与配置。这对部署/迭代不够友好：每次调整搜索开关、上游地址或参数都需要改配置并重启服务。

本次变更希望在**前端管理 UI（管理后台 /admin/settings）**中直接配置联网搜索能力，并持久化到数据库（`app_settings`），遵循现有规则：**界面覆盖优先于配置文件默认**。

## 变更内容

1. 管理后台系统设置增加“联网搜索（SearXNG）”配置项：
   - enable（开关）
   - base_url（搜索上游地址）
   - timeout（请求超时）
   - max_results（结果数量上限）
   - user_agent（上游 UA）
2. Web `/chat` 与 `POST /api/chat/search` 按“配置文件默认 + 数据库覆盖”的**生效配置**运行，无需重启即可生效。

## 影响范围

- **模块:** `internal/admin`、`internal/web`、`internal/store`
- **文件:**
  - `internal/store/app_settings.go`
  - `internal/admin/server.go`
  - `internal/admin/templates/settings.html`
  - `internal/web/server.go`
  - `internal/web/chat.go`
  - `internal/server/app.go`
- **API:** 不新增接口（复用现有 `/admin/settings` 保存；`/api/chat/search` 行为按新配置生效）
- **数据:** 复用 `app_settings`（新增 key，无需新表）

## 核心场景

### 需求: 管理后台配置联网搜索
**模块:** 管理后台系统设置（`/admin/settings`）
管理员在 UI 中配置搜索服务参数。

#### 场景: 启用 SearXNG 搜索
管理员在系统设置中启用并填写 base_url
- 预期结果: 保存成功后无需重启，`/chat` 中“联网搜索”开关可用
- 预期结果: `POST /api/chat/search` 使用新配置对接搜索上游

#### 场景: 配置错误时提示
管理员填写不合法 base_url 或 timeout
- 预期结果: UI 保存返回明确错误信息，不写入无效配置

## 风险评估

- **风险:** 允许 UI 配置上游地址可能引入 SSRF 风险  
  **缓解:** 仅允许配置“固定 base_url”，客户端不可提交 URL；服务端仍做 URL 规范化与校验；并强制 Cookie + CSRF。
- **风险:** 配置错误导致搜索不可用  
  **缓解:** 校验与错误提示；无效覆盖值可忽略并回退默认配置（尽量不中断系统设置页）。

