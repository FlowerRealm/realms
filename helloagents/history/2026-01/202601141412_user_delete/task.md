# 任务清单: user_delete

目录: `helloagents/plan/202601141412_user_delete/`

---

## 1. 管理后台（SSR）
- [√] 1.1 新增 root-only 用户删除接口：`POST /admin/users/{user_id}/delete`（含 CSRF），并在 `internal/server/app.go` 注册路由
- [√] 1.2 在 `internal/admin/server.go` 实现 DeleteUser handler：校验 root、禁止删除当前登录用户、调用 store 删除并重定向
- [√] 1.3 在 `internal/admin/templates/users.html` 增加“删除”按钮（仅 root 且非当前登录用户可见），带确认提示

## 2. 数据层（Store）
- [√] 2.1 在 `internal/store/admin.go` 增加 `DeleteUser`：事务内级联清理 `user_tokens/user_sessions/email_verifications/user_subscriptions/usage_events/audit_events`，最后删除 `users`

## 3. 知识库
- [√] 3.1 更新 `helloagents/wiki/api.md`：补充用户删除接口说明
- [√] 3.2 更新 `helloagents/CHANGELOG.md`：记录新增能力

## 4. 验证
- [√] 4.1 `go test ./...`（至少保证编译通过）
