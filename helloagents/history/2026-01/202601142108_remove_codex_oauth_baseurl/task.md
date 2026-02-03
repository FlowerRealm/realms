# 任务清单: 移除 Codex OAuth 的 BaseURL 配置入口（轻量迭代）

目录: `helloagents/plan/202601142108_remove_codex_oauth_baseurl/`

---

## 1. 管理后台 UI
- [√] 1.1 `internal/admin/templates/endpoints.html`：对 `codex_oauth` 隐藏 Base URL 配置区，仅保留账号授权/录入/列表
- [√] 1.2 `internal/admin/templates/channels.html`：对 `codex_oauth` 隐藏 Base URL 展示（标注为内置/无需配置）

## 2. 后端约束
- [√] 2.1 `internal/admin/server.go`：禁止通过 `POST /admin/channels/{channel_id}/endpoints` 修改 `codex_oauth` 的 base_url（保持为内置默认值）

## 3. 文档与验证
- [√] 3.1 更新 `helloagents/wiki/api.md`（说明 `codex_oauth` base_url 固定，不再提供配置入口）
- [√] 3.2 更新 `helloagents/CHANGELOG.md`
- [√] 3.3 运行 `go test ./...`
