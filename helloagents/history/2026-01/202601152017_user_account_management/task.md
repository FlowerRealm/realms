# 任务清单: 用户账号管理（账号名/邮箱/密码）

目录: `helloagents/plan/202601152017_user_account_management/`

---

## 1. 数据与 store 层
- [√] 1.1 新增迁移：为 `users` 增加 `username` 字段与唯一索引（允许 NULL）。
- [√] 1.2 扩展 `internal/store/models.go`：`User` 增加 `Username` 字段。
- [√] 1.3 扩展 `internal/store`：新增/调整用户查询与更新（按 username 查询、更新 email/username/password、按 user_id 清理 sessions）。

## 2. Web 控制台（普通用户）
- [√] 2.1 调整登录页与登录逻辑：支持“邮箱或账号名”登录，保持兼容旧字段。
- [√] 2.2 新增账号设置页：GET 展示当前信息；POST 支持修改账号名/邮箱（验证码）/密码（旧密码）。
- [√] 2.3 账号信息变更成功后强制登出（清理 sessions + 清 Cookie）。

## 3. 管理后台（root）
- [√] 3.1 增强 `/admin/users`：展示账号名；创建用户时支持可选账号名。
- [√] 3.2 新增管理员操作：修改用户邮箱（验证码）/账号名、重置密码（无需旧密码）。
- [√] 3.3 管理员修改成功后强制登出目标用户（清理 sessions）。

## 4. 安全检查
- [√] 4.1 输入校验：email/username/password 长度与格式校验；避免绕过验证码与旧密码校验。
- [√] 4.2 权限控制：确认普通用户只能改自己；root 才能改他人。

## 5. 文档更新
- [√] 5.1 更新 `helloagents/CHANGELOG.md`（Unreleased 记录本次新增/变更）。
- [√] 5.2 更新 `helloagents/wiki/modules/realms.md`：补充账号名登录与账号设置页说明，更新最后更新日期。
- [√] 5.3 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`。

## 6. 测试
- [√] 6.1 执行 `go test ./...` 并修复编译/模板/迁移相关问题。
