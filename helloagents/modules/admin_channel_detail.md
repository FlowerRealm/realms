# 模块: admin_channel_detail（已移除）

## 概述

历史上（SSR 管理后台）存在渠道详情页：`GET /admin/channels/{channel_id}/detail?window=5m|1h|24h`，用于按 Key/账号聚合近端用量统计。

随着 SSR Web/管理后台全量切换为 **SPA + JSON API**，该页面已删除（2026-01-31），当前仓库不再提供等价页面。

替代入口（当前）：
- 渠道列表与配置：`/admin/channels`（SPA）
  - 渠道聚合用量：`GET /api/channel/page`（按渠道聚合，支持 `start/end` 区间）
  - 渠道健康测试：`GET /api/channel/test/:channel_id`
- 请求级用量明细：`/admin/usage`（SPA）
  - API：`GET /api/admin/usage`、`GET /api/admin/usage/events/:event_id/detail`

如需恢复“按 Key/账号聚合统计”，当前仅保留数据层查询函数：`internal/store/usage.go:GetUsageStatsByCredentialForChannelRange`，可按需新增 API + 前端 UI。

## 相关代码（当前）

- SPA 页面：
  - `web/src/pages/admin/ChannelsPage.tsx`
  - `web/src/pages/admin/UsageAdminPage.tsx`
- API：
  - `router/channels_api_routes.go`
  - `router/admin_usage_api_routes.go`
