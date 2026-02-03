# 任务清单: 移除 group 概念（单租户化）

目录: `helloagents/plan/202601141423_remove_group/`

---

## 1. 数据库迁移（schema）
- [√] 1.1 在 `internal/store/migrations/0001_init.sql` 中删除 `groups` 表与所有 `group_id` 字段/索引，验证 why.md#变更内容
- [√] 1.2 在 `internal/store/migrations/0002_subscriptions.sql` 中移除 `user_subscriptions.group_id` 并调整默认定价插入，验证 why.md#变更内容

## 2. Store（模型与 SQL）
- [√] 2.1 在 `internal/store/models.go` 中移除 `Group` 与所有 `GroupID` 字段，验证编译通过
- [√] 2.2 在 `internal/store/store.go` 中移除 `EnsureDefaultGroup` 与 `group_admin` 角色，调整用户/Token 鉴权 SQL，验证 why.md#需求-全局上游与模型定价由管理员配置
- [√] 2.3 在 `internal/store/upstreams.go` 中将 channel 改为全局读写（不按 group），验证 why.md#需求-全局上游与模型定价由管理员配置
- [√] 2.4 在 `internal/store/usage.go` 中移除 `group_id` 维度并提供用户维度汇总/列表能力，验证 why.md#需求-用户查看用量（SSR--JSON-API）
- [√] 2.5 在 `internal/store/subscriptions.go` 中将订阅查询/购买逻辑改为 `user_id` 唯一，验证 why.md#需求-用户查看用量（SSR--JSON-API）
- [√] 2.6 在 `internal/store/pricing.go` 中移除 scope/group 逻辑，保留全局定价 CRUD，验证 why.md#变更内容

## 3. Scheduler（去 group 化）
- [√] 3.1 在 `internal/scheduler/scheduler.go` 中移除 `group_id` 参数与按组取 channel 的逻辑，验证 why.md#需求-全局上游与模型定价由管理员配置
- [√] 3.2 在 `internal/scheduler/state.go` 中移除 binding/affinity 的 group 维度 key，更新对应单测，验证编译+测试

## 4. 鉴权与中间件
- [√] 4.1 在 `internal/auth/auth.go` 中移除 `Principal.GroupID`，同步更新引用点，验证编译通过
- [√] 4.2 在 `internal/middleware/token_auth.go` 与 `internal/middleware/session_auth.go` 中移除 group 相关字段填充，验证登录/数据面鉴权正常
- [√] 4.3 在 `internal/middleware/logging.go` 与 `internal/middleware/require_role.go` 中移除 group 相关日志与 RBAC 文案/逻辑，验证 why.md#变更内容

## 5. HTTP 路由与 Handler
- [√] 5.1 在 `internal/server/app.go` 中移除 `/admin/groups` 路由与 `group_admin` 权限链，新增 usage API 路由，验证路由可用
- [√] 5.2 在 `internal/api/openai/handler.go` 中移除 group 相关参数传递并适配新 scheduler/quota 接口，验证 `/v1/*` 可用

## 6. 管理后台（SSR）
- [√] 6.1 在 `internal/admin/server.go` 中移除 users/channels 的 group 过滤与校验逻辑，验证 why.md#需求-全局上游与模型定价由管理员配置
- [√] 6.2 在 `internal/admin/templates/users.html` 与 `internal/admin/templates/channels.html` 中移除 group 输入/展示，验证页面渲染
- [√] 6.3 在 `internal/admin/templates/home.html` 与 `internal/admin/templates/base.html` 中移除 Groups 导航，验证页面渲染

## 7. Web 控制台（SSR）
- [√] 7.1 在 `internal/web/server.go` 中移除注册时的默认分组初始化与 group 参数传递，验证注册/登录可用
- [√] 7.2 在 `internal/web/server.go` 中将 `/models` 与 `/subscription` 的上游选择与用量汇总改为用户维度，验证 why.md#需求-用户查看用量（SSR--JSON-API）
- [√] 7.3 在 `internal/web/templates/base.html` 与 `internal/web/templates/dashboard.html` 中移除 group 展示与 group_admin 判断，验证页面渲染

## 8. 用量 API（JSON）
- [√] 8.1 新增 `internal/web/api_usage.go` 实现 `GET /api/usage/windows`（当前挂载在 TokenAuth 路由），验证 why.md#需求-用户查看用量（SSR--JSON-API）
- [√] 8.2 新增 `internal/web/api_usage.go` 实现 `GET /api/usage/events`（分页；当前挂载在 TokenAuth 路由），参考 `/home/flowerrealm/new-api` 的 usage 设计，验证 why.md#需求-用户查看用量（SSR--JSON-API）
- [√] 8.3 可选：为以上两个接口增加 TokenAuth 入口（同返回结构），验证仅返回当前用户数据

## 9. 配额/计费
- [√] 9.1 在 `internal/quota/quota.go` 与 `internal/quota/subscription.go` 中移除 group 维度，并改用全局定价估算，验证订阅限额正确

## 10. 安全检查
- [√] 10.1 执行安全检查（权限控制、越权查询、SSRF 校验未弱化、敏感信息不泄露）

## 11. 文档更新（知识库）
- [√] 11.1 更新 `helloagents/wiki/data.md`：移除 group 与 group_id 描述，保持与代码一致
- [√] 11.2 更新 `helloagents/wiki/api.md`：删除 `/admin/groups`，新增 `/api/usage/*`，修正权限说明
- [-] 11.3 更新 `README.md`：移除 `group_admin` 相关描述（如存在；本仓库 README 未检索到该内容）

## 12. 测试
- [√] 12.1 执行 `go test ./...`，修复编译/单测失败
