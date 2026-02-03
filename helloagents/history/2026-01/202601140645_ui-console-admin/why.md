# 变更提案: UI 补全（订阅/模型列表/用户管理/后台入口）

## 需求背景

当前系统已具备最小可用的 SSR Web 控制台与 SSR 管理后台，但 UI 仍缺少若干关键页面与入口，导致：

1. 普通用户只能看到 Token 管理与调用示例，缺少“模型列表/用量/订阅状态”等自助信息。
2. 管理员缺少用户管理入口（用户列表、启用/禁用、角色/分组调整等）。
3. 管理后台入口在控制台中不明显，影响可发现性。
4. 对 Codex OAuth 上游账号的订阅状态（来自 id_token claims）无法直观看到。

## 变更内容

1. Web 控制台（SSR）补齐导航与页面：模型列表、订阅/用量（MVP 口径）。
2. 管理后台（SSR）补齐用户管理页面：用户列表/创建/启用禁用/角色与分组调整；补齐分组列表与创建。
3. 在控制台 UI 中增加“管理后台”入口（仅对 `root/group_admin` 展示）。
4. 在 Codex OAuth Accounts 页面展示订阅状态（从 `id_token` 解析 `plan_type/subscription_active_*` 等字段，缺失则显示 `-`）。

## 影响范围

- **模块:**
  - `internal/web`（Web 控制台 SSR）
  - `internal/admin`（管理后台 SSR）
  - `internal/store`（补齐用户/分组查询与更新）
  - `internal/server`（路由与中间件链）
- **文件:**
  - `internal/web/server.go`
  - `internal/web/templates/*.html`
  - `internal/admin/server.go`
  - `internal/admin/templates/*.html`
  - `internal/store/*.go`
  - `internal/server/app.go`
  - `helloagents/wiki/api.md`
  - `helloagents/CHANGELOG.md`

## 核心场景

### 需求: Web 控制台补齐关键页面
**模块:** web

#### 场景: 用户查看模型列表
用户登录后进入“模型列表”页，系统从当前分组已配置的 `openai_compatible` 上游请求 `/v1/models` 并展示列表；若未配置或仅配置 `codex_oauth`，给出明确提示。

#### 场景: 用户查看订阅/用量（MVP）
用户登录后进入“订阅/用量”页，展示最近 5h / 7d / 30d 的已结算成本汇总（`usd_micros`），并明确“套餐/订阅额度系统尚未接入/未启用限制”的当前口径。

#### 场景: 管理员快速进入后台
当用户角色为 `root/group_admin` 时，在控制台导航中可见“管理后台”入口，点击可进入 `/admin`。

### 需求: 管理后台补齐用户管理
**模块:** admin

#### 场景: root 管理用户与分组
root 可查看全量用户、按 `group_id` 过滤，创建用户（指定分组/角色），以及对用户执行启用/禁用、调整角色、调整分组。

#### 场景: group_admin 管理本组用户
group_admin 仅能查看与管理本组用户；不得跨组调整，且不得授予 `root`。

### 需求: 管理后台展示订阅状态（Codex OAuth 上游）
**模块:** admin

#### 场景: 查看 Codex OAuth Accounts 的订阅信息
在 `/admin/endpoints/{endpoint_id}/codex-accounts` 的列表中展示 `plan_type` 与 `subscription_active_*`（如存在），便于快速判断账号订阅状态。

## 风险评估

- **风险:** 新增管理操作扩大权限面（用户/分组变更）。  
  **缓解:** 严格复用现有 RBAC（`root/group_admin`），并在 handler 内做跨组限制与角色白名单校验。
- **风险:** 不当展示导致敏感信息泄漏（token/id_token）。  
  **缓解:** UI 仅展示脱敏/非敏感字段；不输出明文 access_token/refresh_token/id_token；保持日志不记录敏感输入。

