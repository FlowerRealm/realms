# 任务清单: 管理后台功能禁用（Feature Bans）

目录: `helloagents/plan/202601181132_feature_bans/`

---

## 1. Feature 定义与 settings keys
- [√] 1.1 在 `internal/store/app_settings.go` 增加 `feature_disable_*` 相关常量（bool key），并补齐 reset 清理列表所需 key，验证 why.md#需求-功能禁用开关
- [√] 1.2 在 `helloagents/wiki/data.md` 同步 `app_settings` 新增 keys 与语义，验证 why.md#需求-功能禁用开关

## 2. Feature Gate 中间件
- [√] 2.1 在 `internal/middleware/` 增加 feature gate 中间件（禁用→404；启用→放行），验证 why.md#需求-功能禁用开关
- [√] 2.2 在 feature gate 系统中纳入 `self_mode` 硬禁用判定（billing/tickets 等），验证 why.md#需求-self_mode-兼容

## 3. 路由接入（后端拒绝访问）
- [√] 3.1 在 `internal/server/app.go` 为用户侧路由组接入 feature gate（如 `/chat`、`/announcements`、`/tickets*`、`/subscription`、`/topup`、`/pay/*`），验证 why.md#需求-功能禁用开关
- [√] 3.2 在 `internal/server/app.go` 为管理侧路由组接入 feature gate（如 `/admin/channels`、`/admin/users`、`/admin/announcements`、计费相关 admin 页面等），验证 why.md#需求-功能禁用开关
- [√] 3.3 确保 `/admin/settings` 不受 feature gate 控制，且后端忽略“禁用系统设置”的写入，验证 why.md#需求-安全保护避免自锁

## 4. 管理后台系统设置页（读写与 UI）
- [√] 4.1 在 `internal/admin/server.go` 的 `Settings/UpdateSettings` 中增加 feature bans 的读取、保存与 reset 清理，验证 why.md#需求-恢复默认
- [√] 4.2 在 `internal/admin/templates/settings.html` 新增「功能禁用」分区（包含 self_mode 强制禁用提示），验证 why.md#需求-功能禁用开关

## 5. UI 入口隐藏（用户/管理后台）
- [√] 5.1 在 `internal/web/templates/base.html` 隐藏被禁用功能的侧边栏入口，验证 why.md#需求-功能禁用开关
- [√] 5.2 在 `internal/admin/templates/base.html` 隐藏被禁用功能的侧边栏入口（系统设置入口保留），验证 why.md#需求-安全保护避免自锁
- [√] 5.3 （可选）在 `internal/web/templates/dashboard.html` 等入口页同步移除/隐藏被禁用功能的快捷入口，避免“死入口”

## 6. 安全检查
- [√] 6.1 执行安全检查（权限控制、锁死风险、默认值、self_mode 覆盖逻辑），按 G9 记录风险与规避

## 7. 测试
- [-] 7.1 更新/新增 `internal/server/app_test.go`：覆盖至少 billing/tickets 的禁用回归，以及新增的 feature gate 关键路由 404 行为（当前无 DB/可注入 stub，已用 middleware 单测覆盖核心 gate 行为）
- [√] 7.2 新增 `internal/middleware/*_test.go`：用 fake/stub 验证 feature gate 中间件（禁用/启用/self_mode 硬禁用）

## 8. 文档更新
- [√] 8.1 更新 `helloagents/wiki/modules/realms.md`：补充 feature bans 与 self_mode 的关系、以及 DB 删除 keys 的救援方式
