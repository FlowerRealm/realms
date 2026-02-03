# 任务清单: UI 补全（订阅/模型列表/用户管理/后台入口）

目录: `helloagents/history/2026-01/202601140645_ui-console-admin/`

---

## 1. Web 控制台（SSR）
- [√] 1.1 在 `internal/server/app.go` 中补齐 Web 路由：`/models`、`/subscription`，并修正 Token/Logout 的中间件链不应要求管理员角色，验证 why.md#需求-web-控制台补齐关键页面-场景-管理员快速进入后台
- [√] 1.2 在 `internal/web/templates/base.html` 中增加登录后导航（控制台/模型列表/订阅用量/管理后台），验证 why.md#需求-web-控制台补齐关键页面-场景-管理员快速进入后台
- [√] 1.3 在 `internal/web/server.go` + `internal/web/templates/models.html` 实现模型列表页面（上游 `/v1/models`），验证 why.md#需求-web-控制台补齐关键页面-场景-用户查看模型列表
- [√] 1.4 在 `internal/web/server.go` + `internal/web/templates/subscription.html` 实现订阅/用量页面（最近 5h/7d/30d 汇总），验证 why.md#需求-web-控制台补齐关键页面-场景-用户查看订阅用量mvp

## 2. 管理后台（SSR）
- [√] 2.1 在 `internal/store/*.go` 中补齐用户/分组管理所需的 store 方法（list/create/update），验证 why.md#需求-管理后台补齐用户管理-场景-root-管理用户与分组
- [√] 2.2 在 `internal/admin/server.go` + `internal/admin/templates/users.html` 实现 `/admin/users`（列表/创建/更新），并做跨组/角色校验，验证 why.md#需求-管理后台补齐用户管理-场景-group_admin-管理本组用户
- [√] 2.3 在 `internal/admin/server.go` + `internal/admin/templates/groups.html` 实现 `/admin/groups`（列表/创建），验证 why.md#需求-管理后台补齐用户管理-场景-root-管理用户与分组
- [√] 2.4 在 `internal/admin/server.go` 与 `internal/admin/templates/codex_accounts.html` 展示订阅状态（解析 id_token claims），验证 why.md#需求-管理后台展示订阅状态codex-oauth-上游-场景-查看-codex-oauth-accounts-的订阅信息

## 3. 安全检查
- [√] 3.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/api.md` 补齐 Web 控制台与新增管理面入口说明
- [√] 4.2 更新 `helloagents/CHANGELOG.md`（Unreleased）记录本次 UI 补全

## 5. 测试
- [√] 5.1 运行 `go test ./...`
