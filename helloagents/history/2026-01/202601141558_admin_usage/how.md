# 技术方案：管理后台全局用量统计

## 方案概述

在不改动 schema 的前提下，基于现有 `usage_events` 做聚合查询：
- 全局窗口汇总：按 `time >= since` 汇总 `committed_usd_micros`，并统计仍未过期的 `reserved_usd_micros`；
- Top 用户排行：同窗口维度按 `user_id` 聚合并 join `users` 取 `email/role/status` 展示。

管理面通过 SSR 页面 `/admin/usage` 展示，权限沿用现有 `adminChain`（SessionAuth + RequireRoles(root) + CSRF）。

## 数据口径

- **已结算（committed）**：`state='committed'` 的 `committed_usd_micros` 求和。
- **预留（reserved）**：`state='reserved' AND reserve_expires_at >= now` 的 `reserved_usd_micros` 求和。
- **窗口**：与订阅限额窗口一致，固定为 `5h / 7d / 30d`。

## 代码改动点

1. `internal/store/usage.go`
   - 增加全局汇总查询（不带 user_id 过滤）。
   - 增加 Top 用户聚合查询（group by user_id，join users）。

2. `internal/admin/`
   - 新增 handler：读取当前用户（root）并渲染 `/admin/usage`。
   - 新增模板：全局用量窗口表 + Top 用户表。
   - 管理后台侧边栏增加“用量统计”入口。

3. `internal/server/app.go`
   - 注册 `GET /admin/usage` 路由到 adminChain。

4. 知识库同步
   - 更新 `helloagents/wiki/api.md`，补齐管理面新页面描述。
   - 更新 `helloagents/CHANGELOG.md` 记录新增能力。

## 风险与规避

- **风险：全局聚合查询在大表上变慢**
  - 规避：所有查询均带 `time >= since`；现有索引包含 `idx_usage_events_time`，且聚合仅统计近期窗口（最大 30d）。
- **风险：越权访问**
  - 规避：路由走 `adminChain`，仅 root session 可访问。
