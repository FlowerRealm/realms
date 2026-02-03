# 任务清单：管理后台全局用量统计

目录: `helloagents/plan/202601141558_admin_usage/`

---

## 1. Store：全局用量聚合
- [√] 1.1 在 `internal/store/usage.go` 增加全局窗口汇总查询（committed/reserved）。
- [√] 1.2 在 `internal/store/usage.go` 增加 Top 用户聚合查询（按 user_id 聚合并关联 users）。

## 2. 管理后台（SSR）
- [√] 2.1 新增 `GET /admin/usage` handler（仅 root），展示 5h/7d/30d 全局用量。
- [√] 2.2 新增 `internal/admin/templates/usage.html` 模板（窗口汇总 + Top 用户）。
- [√] 2.3 在 `internal/admin/templates/base.html` 增加“用量统计”导航入口。

## 3. 路由接入
- [√] 3.1 在 `internal/server/app.go` 注册 `GET /admin/usage` 到 `adminChain`。

## 4. 文档与变更记录
- [√] 4.1 更新 `helloagents/wiki/api.md`：补齐 `/admin/usage`。
- [√] 4.2 更新 `helloagents/CHANGELOG.md`：记录新增管理面全局用量统计。

## 5. 验证
- [√] 5.1 执行 `go test ./...`。

## 6. 收尾（强制）
- [√] 6.1 更新本任务清单状态。
- [√] 6.2 迁移方案包到 `helloagents/history/2026-01/202601141558_admin_usage/` 并更新 `helloagents/history/index.md`。
