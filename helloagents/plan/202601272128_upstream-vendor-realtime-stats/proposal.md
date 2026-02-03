# 变更提案: upstream-vendor-realtime-stats

## 元信息
```yaml
类型: 新功能
方案类型: implementation
优先级: P1
状态: 规划中
创建: 2026-01-27
```

---

## 1. 需求

在管理后台提供“按上游供应商（Channel Type）”的实时统计能力（参考 `BenedictKing/claude-proxy` 的渠道监控体验）：

- 指标：
  - 请求数
  - 成功率
- 展示方式：Web 面板查看；每次刷新页面/点击刷新即可更新
- 持久化：多实例部署下可汇总（基于共享 DB）

---

## 2. 现状（代码阅读要点）

- Realms 已持久化记录每次数据面请求：`usage_events`（含 `time/status_code/upstream_channel_id/...`）。
- 已有按渠道（`upstream_channel_id`）聚合的用量统计：`Store.GetUsageStatsByChannelRange`，但不包含“按供应商维度”的请求数与成功率。
- Scheduler 有运行态 RPM/FailScore（内存），但不满足持久化与多实例汇总诉求。

---

## 3. 方案

### 3.1 数据口径

- 供应商（Vendor）定义：`upstream_channels.type`（例如 `openai_compatible/codex_oauth/anthropic`）。
- 请求数：统计窗口内 `usage_events` 记录数（按 vendor 归因）。
- 成功：`status_code` 在 `[200, 300)`（必要时可叠加 `error_class IS NULL` 作为更严格口径）。
- 成功率：
  - `done = status_code > 0`（排除进行中 `status_code=0` 的记录，避免拉低成功率）
  - `success_rate = success / done`

> 待确认：若需要“按 failover 的每次上游尝试（attempt）”统计，则需新增 attempt 级别事件落库；本方案默认按“最终选中的 upstream”归因。

### 3.2 后端（store + admin）

- Store 新增聚合查询：`GetRequestStatsByUpstreamTypeRange(since, until)`（返回各 vendor 的 requests/success/success_rate）。
- Admin 新增 AJAX API（仅 root）：
  - `GET /admin/upstream-vendors/stats?window=5m`（默认 `5m`）
  - 返回 JSON：按 vendor 列表输出统计数据。

### 3.3 管理后台 UI

- 入口建议：放在 `/admin/channels` 页头（与上游管理强相关）。
- 展示形式：卡片 + 表格（供应商 / 请求数 / 成功率），带“刷新”按钮与窗口下拉选择（1m/5m/15m/1h/6h/24h）。
- 刷新策略：页面加载自动刷新；下拉切换窗口或点击刷新按钮即可更新（符合“每次刷新查看是更新”）。

### 3.4 多实例 / 持久化

- 直接基于 DB 聚合：SQLite=单机可用；MySQL=多实例共享汇总（满足“需要”）。
- 性能：按 `time` 范围查询；利用现有索引（必要时再评估补充联合索引）。

---

## 4. 验收标准

- [ ] 管理后台可看到按 vendor 的请求数与成功率（窗口可选），点击刷新可即时更新
- [ ] 页面内明确成功率口径（success/request，按最终选中 upstream 归因）
- [ ] Store 聚合查询具备单元测试
- [ ] `go test ./...` 通过
