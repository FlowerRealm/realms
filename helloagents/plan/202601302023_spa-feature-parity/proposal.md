# 变更提案: spa-feature-parity

## 元信息
```yaml
类型: 功能
方案类型: implementation
优先级: P0
状态: 草稿
创建: 2026-01-30
基线(tag): 0.3.3
```

---

## 1. 背景

Realms 已完成与 `new-api` 的前后端分离对齐（Gin Router + `web/` React SPA + `/api/*` JSON API + SPA fallback），但当前仅迁移了 MVP（登录/注册、Dashboard、Tokens、Models、Usage、Admin: Channels/Models）。

结果是：旧 SSR（`internal/web/*`、`internal/admin/*`）时代的大量功能在当前 SPA 中缺失，且 SSR 路由已被显式停用（见 `helloagents/plan/202601292015_frontend-backend-separation/` 的任务 3.8/3.9 及 “停用 SSR 路由”）。

本方案目标是：在保持“前后端分离 + SPA 接管页面路由”的前提下，把旧版功能**按既有路由与能力 1:1 补齐**，做到“一个都不能少”。

---

## 2. 目标与范围

### 2.1 总目标

将旧版 SSR 的 Console + Admin 能力迁移到：
- 后端 `/api/*` JSON API（Gin，使用 session + `Realms-User` header 作为 CSRF 防护）；
- 前端 `web/` SPA（React Router，复刻旧 UI 结构与交互）。

### 2.2 用户侧（Console）需要补齐的功能

| 功能 | 旧版参考(SSOT) | 现状 | 缺口（需要补齐的代码部分） |
|---|---|---|---|
| 公告（列表/详情/已读） | `internal/web/templates/announcements.html`、`internal/web/templates/announcement.html`、`internal/web/announcements.go` | 有 API（`router/announcements_api_routes.go`）+ 有列表页组件（`web/src/pages/AnnouncementsPage.tsx`）但未挂路由/缺详情页 | 前端路由：`web/src/App.tsx` + 侧边栏项；补齐详情页 `web/src/pages/AnnouncementDetailPage.tsx`；补齐 API 调用/MarkRead 交互 |
| 账号设置（用户名/邮箱/密码） | `internal/web/templates/account.html`、`internal/web/server.go` 相关 handler | 完全缺失 | 后端：新增 `/api/account/*`（update username/email/password）；前端：新增 `/account` 页 + 表单交互；必要时扩展 `GET /api/user/self` 输出 |
| 订阅管理（套餐列表/购买/我的订阅/订单） | `internal/web/templates/subscription.html`、`internal/web/money.go`、`internal/store/*subscription*` | 完全缺失 | 后端：新增 `/api/billing/subscriptions`、`/api/billing/plans`、`/api/billing/orders`、`/api/billing/purchase`；前端：新增 `/subscription` 页与购买交互 |
| 余额充值（创建充值单/支付入口） | `internal/web/templates/topup.html`、`internal/web/money.go` | 完全缺失 | 后端：新增 `/api/billing/topups`、`/api/billing/topup/create`、`/api/billing/pay/*`；前端：新增 `/topup` 页与创建订单交互 |
| 支付页（订阅/充值支付、取消订单、回跳） | `internal/web/templates/pay.html`、`internal/web/money.go` | 完全缺失 | 需要在 SPA 中实现等价流程；若涉及第三方支付跳转/二维码展示，优先复刻旧页面交互与参数结构 |
| 工单（列表/新建/详情/回复/附件） | `internal/web/templates/tickets.html`、`ticket_new.html`、`ticket_detail.html`、`internal/web/tickets.go`、`internal/tickets/*` | 完全缺失 | 后端：新增 `/api/tickets/*`（含附件上传/下载）；前端：新增 `/tickets`、`/tickets/new`、`/tickets/:id` 等页 |

> 注：`self_mode` 下按旧逻辑 Billing/Tickets 可能被禁用；需在前端导航与后端 API 中一致处理（feature gate / SelfMode）。

### 2.3 管理侧（Admin）需要补齐的功能

| 功能 | 旧版参考(SSOT) | 现状 | 缺口（需要补齐的代码部分） |
|---|---|---|---|
| 分组（Channel Groups） | `internal/admin/templates/channel_groups.html`、`channel_group_detail.html`、`internal/admin/channel_groups*.go` | 完全缺失 | 后端：新增 `/api/admin/channel-groups/*`；前端：新增 `/admin/channel-groups`、`/admin/channel-groups/:id` |
| 用户管理 | `internal/admin/templates/users.html`、`internal/admin/users.go` | 完全缺失 | 后端：新增 `/api/admin/users/*`（列表/编辑/禁用/删除等）；前端：新增 `/admin/users` |
| 订阅套餐（Plan 管理） | `internal/admin/templates/subscriptions.html`、`internal/admin/subscriptions.go` | 完全缺失 | 后端：新增 `/api/admin/subscriptions/*`；前端：新增 `/admin/subscriptions`、`/admin/subscriptions/:id` |
| 订单管理（订阅/充值订单） | `internal/admin/templates/orders.html`、`internal/admin/orders.go` | 完全缺失 | 后端：新增 `/api/admin/orders/*`；前端：新增 `/admin/orders` |
| 用量统计（Admin） | `internal/admin/templates/usage.html`、`internal/admin/usage.go` | 完全缺失 | 后端：新增 `/api/admin/usage/*`；前端：新增 `/admin/usage`（含日期筛选、展开明细） |
| 工单管理（Admin） | `internal/admin/templates/tickets.html`、`ticket_detail.html`、`internal/admin/tickets.go` | 完全缺失 | 后端：新增 `/api/admin/tickets/*`（列表/详情/回复/附件）；前端：新增 `/admin/tickets`、`/admin/tickets/:id` |
| 公告管理（Admin） | `internal/admin/templates/announcements.html`、`internal/admin/announcements.go` | 完全缺失 | 后端：新增 `/api/admin/announcements/*`（CRUD/发布）；前端：新增 `/admin/announcements` |
| OAuth Apps（Admin） | `internal/admin/templates/oauth_apps.html`、`oauth_app.html`、`oauth_app_secret.html`、`internal/admin/oauth*.go` | 完全缺失 | 后端：新增 `/api/admin/oauth-apps/*`；前端：新增 `/admin/oauth-apps`、`/admin/oauth-apps/:id`、`rotate-secret` |
| 系统设置 | `internal/admin/templates/settings.html`、`internal/admin/settings.go` | 完全缺失 | 后端：新增 `/api/admin/settings/*`（app_settings 读写）；前端：新增 `/admin/settings` |
| 支付渠道（可选但旧版有） | `internal/admin/templates/payment_channels.html`、`payment_channel.html`、`internal/admin/payment_channels.go` | 完全缺失 | 后端：新增 `/api/admin/payment-channels/*`；前端：新增 `/admin/payment-channels` |

---

## 3. 约束与策略

### 3.1 路由与交付
- **按现有路由**：用户侧继续使用 `/subscription`、`/topup`、`/tickets`、`/account` 等；管理侧继续使用 `/admin/*`。
- **继续保持 SPA fallback**：非 `/api|/v1|/v1beta|/oauth|/assets|/healthz` 的路径都由 SPA 处理。

### 3.2 安全模型
- 延续现有方案：session + `Realms-User` header（API 层统一使用 `requireUserSession()` / `requireRootSession()`）。
- 对 “涉及敏感信息” 的接口（如 OAuth App secret、支付配置）做最小暴露与必要权限校验（root-only）。

### 3.3 实现策略（关键决策）
1) **优先复用现有 store / 业务逻辑**：旧 SSR 的 handler 只作为行为参考，数据操作以 `internal/store` 为准。
2) **先补齐“端到端可用”再做 UI 精修**：每个模块先做到可访问 + CRUD 可用 + 基本样式一致，然后再逐步对齐交互细节。
3) **最小化引入新依赖**：前端继续使用 Bootstrap CDN；后端保持现有依赖集合。

---

## 4. 验收标准

- [ ] 用户侧：公告/账号设置/订阅管理/余额充值/工单 均可在 SPA 中访问并完成核心操作（创建、查看、更新、必要的取消/回复/下载）。
- [ ] 管理侧：分组/用户/订阅套餐/订单/用量/工单/公告/OAuth Apps/系统设置 均可在 SPA 中访问并完成核心操作。
- [ ] `go test ./...` 通过
- [ ] `cd web && npm run lint && npm run build` 通过

