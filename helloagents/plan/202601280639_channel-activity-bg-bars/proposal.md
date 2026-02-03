# 变更提案: channel-activity-bg-bars

## 元信息
```yaml
类型: 新功能
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-28
```

---

## 1. 需求

参考 `claude-proxy` 的“渠道编排”体验：在 Realms 管理后台 `/admin/channels` 的渠道行内，以**柱形统计图**作为渠道背景，展示该渠道最近一段时间的活跃度（请求量与失败情况）。

要求：
- 展示位置：每个渠道行的“渠道详情”单元格背景（不影响点击与交互）
- 图形：柱形图（按时间分段，从旧到新）
- 口径：按最终选中 upstream（`usage_events.upstream_channel_id`）归因
- 数据源：DB（满足多实例汇总）
- 与现有“供应商实时统计”窗口下拉共用时间范围

---

## 2. 方案

### 2.1 数据与分段

- 数据表：`usage_events`
- 过滤条件：
  - `upstream_channel_id IS NOT NULL`
  - `status_code > 0`（排除进行中）
- 成功判定：`status_code` 为 2xx
- 分桶：
  - 根据选定窗口自动计算 `bucket_seconds` 与 `segments`
  - 将 `now` 对齐到 bucket 边界，避免刷新时柱子整体抖动

### 2.2 管理后台 API

- 新增：`GET /admin/channels/activity?window=...`
- 返回：
  - `bucket_seconds`、`segments`、`since/until`
  - `channels[channel_id] = [{request_count,failure_count}...]`

### 2.3 UI 渲染

- 每行增加一个绝对定位的背景容器 `div.channel-activity-bg`
- 前端通过 fetch 拉取活动分段数据，按 channel_id 渲染 SVG 柱形图
- 颜色按成功率分档（绿→黄→橙→红），高度按该渠道的历史最大值归一化（同一窗口内避免突然变高）

---

## 3. 验收标准

- [√] `/admin/channels` 每个渠道行展示柱形背景
- [√] 切换窗口/点击刷新时柱形背景随数据更新
- [√] 支持多实例（仅依赖 DB）
- [√] `go test ./...` 通过

