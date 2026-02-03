# 任务清单: 移除弹窗提示

目录: `helloagents/plan/202601162129_remove_popup_prompts/`

---

## 1. 管理后台（Admin）
- [√] 1.1 在 `internal/admin/templates/base.html` 中禁用 `window.alert/confirm/prompt`，验证 why.md#需求-去除弹窗提示-场景-管理后台操作不再弹出确认或报错提示

## 2. 用户控制台（Web）
- [√] 2.1 在 `internal/web/templates/dashboard.html` 中移除未读公告自动弹窗逻辑并同步文案，验证 why.md#需求-去除弹窗提示-场景-登录-dashboard-不再自动弹出未读公告
- [√] 2.2 在 `internal/admin/templates/announcements.html` 中更新说明文案，避免与实际行为不一致

## 3. 安全检查
- [√] 3.1 执行安全检查（按G9：权限控制/CSRF 仍然生效；无敏感信息新增；无 EHRB 信号）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/modules/realms.md`（公告未读提示逻辑）
- [√] 4.2 更新 `helloagents/CHANGELOG.md`（记录移除弹窗提示）

## 5. 测试
- [√] 5.1 运行 `go test ./...` 并记录结果
