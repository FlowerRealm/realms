# 任务清单: restore-console-admin-features

目录: `helloagents/archive/2026-01/202601310120_restore-console-admin-features/`

> **@status:** completed | 2026-01-31 01:58

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 21
已完成: 21
完成率: 100%
```

---

## 任务列表

### 1) 缺口盘点（功能 & 页面）

- [√] 1.1 对照缺失清单：用户控制台（订阅/充值/工单/账号设置）与管理后台（分组/用户/订阅套餐/订单/用量/工单/公告/OAuth Apps/系统设置）
- [√] 1.2 以现有 `/api/admin/*` 路由为准，确认 SPA 缺失页面与需要补齐的 API client

### 2) Admin：页面补齐（按侧边栏入口对齐）

- [√] 2.1 分组：新增 `/admin/channel-groups`、`/admin/channel-groups/:id`（列表/详情/成员管理/排序）
- [√] 2.2 用户管理：新增 `/admin/users`（创建/编辑/加余额/重置密码/删除）
- [√] 2.3 订阅套餐：新增 `/admin/subscriptions`、`/admin/subscriptions/:id`（列表/创建/编辑/删除）
- [√] 2.4 订单：新增 `/admin/orders`（列表/批准/拒绝）
- [√] 2.5 支付渠道：新增 `/admin/payment-channels`（列表/创建/编辑/删除）
- [√] 2.6 用量统计：新增 `/admin/usage`（汇总/Top 用户/事件列表/事件详情）
- [√] 2.7 工单：新增 `/admin/tickets`、`/admin/tickets/:id`（列表/详情/回复/关闭/恢复）
- [√] 2.8 公告：新增 `/admin/announcements`（创建/发布切换/删除）
- [√] 2.9 OAuth Apps：新增 `/admin/oauth-apps`、`/admin/oauth-apps/:id`（创建/编辑/轮换 secret）
- [√] 2.10 系统设置：新增 `/admin/settings`（对接 `/api/admin/settings`）

### 3) Admin：路由与导航

- [√] 3.1 更新 `web/src/pages/AdminPage.tsx`：补齐 `/admin/*` 子路由映射
- [√] 3.2 更新 `web/src/layout/AdminLayout.tsx`：补齐侧边栏入口（含支付渠道）

### 4) API client 补齐

- [√] 4.1 新增 `web/src/api/admin/tickets.ts`
- [√] 4.2 新增 `web/src/api/admin/paymentChannels.ts`

### 5) 文档同步（知识库）

- [√] 5.1 更新 `README.md`（前端入口覆盖说明）
- [√] 5.2 更新 `helloagents/modules/web_spa.md`（路由范围同步）
- [√] 5.3 更新 `helloagents/CHANGELOG.md`（记录本次补齐）

### 6) 验证与回归

- [√] 6.1 前端构建通过：`cd web && npm run build`
- [√] 6.2 后端测试通过：`go test ./...`

---

## 验证记录

```bash
cd web && npm run build
go test ./...
```
