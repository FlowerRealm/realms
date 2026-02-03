# 任务清单: 合并 Codex OAuth 账号管理到 Endpoint 页面（轻量迭代）

目录: `helloagents/plan/202601142013_merge_codex_accounts_into_endpoints/`

---

## 1. 管理后台 UI
- [√] 1.1 在 `internal/admin/server.go` 合并 Codex OAuth Accounts 数据到 `Endpoints` 页面渲染，并将 `/admin/endpoints/{endpoint_id}/codex-accounts` 改为重定向
- [√] 1.2 在 `internal/admin/templates/endpoints.html` 增加 Codex OAuth 授权/账号列表模块，移除跳转按钮
- [√] 1.3 更新 `internal/admin/templates/channels.html` 的入口，指向 Endpoint 页面锚点

## 2. 路由与跳转
- [√] 2.1 调整 Codex OAuth 相关 handler 的 redirect 目标到 `/admin/channels/{channel_id}/endpoints#accounts`

## 3. 文档与验证
- [√] 3.1 更新 `helloagents/wiki/api.md`（Codex OAuth 账号管理入口说明）
- [√] 3.2 更新 `helloagents/CHANGELOG.md`
- [√] 3.3 运行 `go test ./...`
