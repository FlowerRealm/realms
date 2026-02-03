# 变更提案: 移除 group 概念（单租户化）

## 需求背景

当前实现以 `group` 作为租户隔离维度（DB 字段、调度、RBAC、管理后台筛选等）。但本项目定位为“中转服务”，上游与模型定价均由管理员统一控制，用户不应参与选择上游或跨租户协作。继续保留 `group` 会带来额外复杂度与更高的维护成本（同时更容易遗漏导致越权/计费混乱）。

本次变更在用户已明确“数据库可以全删”的前提下，将系统收敛为单租户模型：所有用户共享管理员配置的上游资源与定价规则，用量按用户维度可见。

## 变更内容

1. **移除 group 领域概念**：删除 `groups` 表与所有 `group_id` 字段/索引；删除与 `group` 相关的结构体/SQL/业务分支。
2. **权限收敛**：删除 `group_admin` 角色与“按组”的权限语义；管理后台仅 `root` 可访问。
3. **上游配置全局化**：`upstream_channels` 不再按组区分；调度器选择逻辑不再依赖 group 维度，用户请求统一从全局上游池中选择。
4. **模型定价全局化**：`pricing_models` 为全局表，由管理员维护；成本估算与订阅限额计算不再按组取数。
5. **用量开放用户查看（SSR + API）**：保留 `/subscription` 页面；新增 JSON API 供用户程序化查询自身用量（接口风格参考 `/home/flowerrealm/new-api`）。

## 影响范围

- **模块:** store / scheduler / quota / middleware / web / admin / api(openai)
- **文件:** `internal/store/*`、`internal/scheduler/*`、`internal/quota/*`、`internal/middleware/*`、`internal/web/*`、`internal/admin/*`、`internal/server/app.go`
- **API:**
  - 移除：`GET/POST /admin/groups`
  - 新增：`GET /api/usage/windows`、`GET /api/usage/events`（会话与/或 Token 鉴权）
- **数据:** 迁移文件 `0001_init.sql`、`0002_subscriptions.sql` 需要重写；数据库需重建

## 核心场景

### 需求: 全局上游与模型定价由管理员配置
**模块:** admin / store / scheduler / quota

#### 场景: 管理员配置并对所有用户生效
前置条件：管理员以 `root` 登录管理后台。
- 管理员创建/维护上游 Channel、Endpoint、Credential/Account（不再选择 group）。
- 管理员维护 `pricing_models` 定价规则（pattern + 单价 + priority）。
- 普通用户通过数据面 Token 调用 `/v1/*` 时，系统从全局上游池中调度，并按全局定价估算成本与扣减订阅额度。

### 需求: 用户查看用量（SSR + JSON API）
**模块:** web / store / quota

#### 场景: Web 页面查看
前置条件：用户已登录 Web 控制台并存在有效订阅。
- 用户访问 `/subscription` 可查看最近 5h/7d/30d 的已结算/预留/限额/剩余。
- 展示口径以本服务的 `usage_events` 汇总为准。

#### 场景: API 查询
前置条件：用户持有会话 Cookie 或数据面 Token（由你最终决定开放范围）。
- 用户调用 `/api/usage/windows` 获取窗口汇总数据（仅自身）。
- 用户调用 `/api/usage/events` 分页获取最近用量事件列表（仅自身）。

## 风险评估

- **风险:** 破坏性变更（schema/鉴权/调度/计费均受影响），遗漏会导致鉴权失败、用量统计错误或潜在越权。
- **缓解:** 以“可全删 DB”为前提重写迁移；编译期移除字段/接口强制收敛；补齐关键单测；对新增 usage API 强制按 `principal.user_id` 过滤。

