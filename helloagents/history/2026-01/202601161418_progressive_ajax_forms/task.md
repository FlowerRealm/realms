# 任务清单: 渐进式 AJAX 表单提交（不跳转 / 不污染 URL）

目录: `helloagents/plan/202601161418_progressive_ajax_forms/`

---

## 1. 通用 AJAX 表单能力（前端）
- [√] 1.1 在 `internal/admin/templates/base.html` 增加 `data-ajax="1"` 表单拦截与提示展示（toast/alert）
- [√] 1.2 在 `internal/web/templates/base.html` 增加同等能力（与 admin 保持一致的行为）
- [√] 1.3 处理重复提交：提交期间禁用按钮；失败恢复

## 2. 服务端 AJAX 分支（不重定向）
- [√] 2.1 在 `internal/admin` 增加 `isAjax(r)` 与 JSON 响应 helper（同包内）
- [√] 2.2 将以下 handler 的 `?msg/?err` 重定向改为“AJAX=JSON / 非AJAX=原PRG”：
  - [√] `internal/admin/server.go`（settings/users/channels 等）
  - [√] `internal/admin/models.go`
  - [√] `internal/admin/channel_groups.go`
  - [√] `internal/admin/subscriptions.go`
  - [√] `internal/admin/channel_models.go`
  - [√] `internal/admin/announcements.go`
  - [√] `internal/admin/channel_health.go`
- [ ] 2.3 （可选）在 `internal/web` 对需要的交互补齐 AJAX 分支（如后续确认 `tickets` 的 `msg=closed` 也要前端提示）

## 3. 逐页开启（模板表单标记）
- [√] 3.1 为管理后台相关页面表单增加 `data-ajax="1"`（必要时 `data-ajax-reload="1"`）：
  - [√] `internal/admin/templates/settings.html`
  - [√] `internal/admin/templates/users.html`
  - [√] `internal/admin/templates/models.html`
  - [√] `internal/admin/templates/announcements.html`
  - [√] `internal/admin/templates/subscriptions.html`
  - [√] `internal/admin/templates/channel_groups.html`
  - [√] `internal/admin/templates/channel_models.html`
  - [√] （按实际表单）`internal/admin/templates/channels.html` / `internal/admin/templates/endpoints.html`
- [√] 3.2 确认未标记的表单行为不变（渐进式）

## 4. 安全检查
- [√] 4.1 确认 CSRF header 快路径可用（`X-CSRF-Token`）
- [√] 4.2 确认提示渲染使用纯文本（避免 XSS）

## 5. 文档更新
- [√] 5.1 更新 `helloagents/wiki/modules/realms.md` 记录“AJAX 表单不跳转”策略
- [√] 5.2 更新 `helloagents/CHANGELOG.md` 增加修复/变更记录

## 6. 测试
- [√] 6.1 运行 `go test ./...`
