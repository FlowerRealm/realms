# 任务清单: spa-feature-parity

目录: `helloagents/plan/202601302023_spa-feature-parity/`

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
总任务: 34
已完成: 4
完成率: 12%
```

---

## 任务列表

### 1) 缺口盘点（按功能对齐 SSR）

- [ ] 1.1 生成“旧 SSR 功能 → 新 SPA/API 现状 → 缺口”对照表（以 `proposal.md` 为准细化到文件/路由/接口）
- [ ] 1.2 统一路由清单（用户侧 + 管理侧）并补齐 SPA 侧边栏导航结构

### 2) Console：公告（Announcements）

- [√] 2.1 前端：挂载 `/announcements` 路由并复用 `AnnouncementsPage`
- [√] 2.2 前端：实现公告详情页 `/announcements/:id`（含 MarkRead）
- [√] 2.3 前端：修复 Dashboard 中公告入口/跳转与未读提示一致性
- [√] 2.4 后端：公告 API 回归测试（list/detail/read）

### 3) Console：账号设置（Account）

- [ ] 3.1 后端：新增 `/api/account/username`、`/api/account/email`、`/api/account/password`（复刻 SSR 规则与错误信息）
- [ ] 3.2 前端：新增 `/account` 页面（分区表单：用户名/邮箱/密码）
- [ ] 3.3 前端：完善 `user/self` 输出（如需：self_mode、feature flags、email_verification_enabled 等）
- [ ] 3.4 后端：账号设置 API 单测

### 4) Console：订阅/充值/支付（Billing）

- [ ] 4.1 后端：补齐 Billing API（plans/subscriptions/orders/topup/create/pay/start/cancel 等）并与 store/支付配置对齐
- [ ] 4.2 前端：新增 `/subscription`（我的订阅 + 订单 + 套餐列表 + 下单）
- [ ] 4.3 前端：新增 `/topup`（充值订单 + 创建充值单）
- [ ] 4.4 前端：新增 `/pay/:kind/:orderId`（支付页：展示渠道/二维码/跳转 + 取消订单）
- [ ] 4.5 后端：Billing API 单测（关键写操作 + 权限 + self_mode gating）

### 5) Console：工单（Tickets）

- [ ] 5.1 后端：补齐 Tickets API（列表/新建/详情/回复/附件上传下载）
- [ ] 5.2 前端：新增 `/tickets`（open/closed tabs）
- [ ] 5.3 前端：新增 `/tickets/new`
- [ ] 5.4 前端：新增 `/tickets/:id`（详情 + 回复 + 附件）
- [ ] 5.5 后端：Tickets API 单测（含附件场景）

### 6) Admin：导航与基础壳

- [ ] 6.1 前端：扩展 `/admin/*` 子路由结构（统一入口页 + 各功能菜单）
- [ ] 6.2 前端：AppLayout 侧边栏补齐“管理后台”完整菜单（按旧版分组）

### 7) Admin：Channel Groups（分组）

- [ ] 7.1 后端：新增 `/api/admin/channel-groups/*`（列表/详情/创建/更新/删除/树结构/指针等）
- [ ] 7.2 前端：新增 `/admin/channel-groups`、`/admin/channel-groups/:id`（树/表单/绑定）
- [ ] 7.3 后端：channel-groups API 单测

### 8) Admin：Users（用户管理）

- [ ] 8.1 后端：新增 `/api/admin/users/*`（列表/详情/更新角色/禁用/删除/重置等，按旧逻辑取舍）
- [ ] 8.2 前端：新增 `/admin/users`
- [ ] 8.3 后端：users API 单测

### 9) Admin：Announcements（公告管理）

- [ ] 9.1 后端：新增 `/api/admin/announcements/*`（CRUD/发布/撤回）
- [ ] 9.2 前端：新增 `/admin/announcements`
- [ ] 9.3 后端：admin announcements API 单测

### 10) Admin：Billing（套餐/订单/支付渠道）

- [ ] 10.1 后端：新增 `/api/admin/subscriptions/*`（plan CRUD）
- [ ] 10.2 后端：新增 `/api/admin/orders/*`
- [ ] 10.3 后端：新增 `/api/admin/payment-channels/*`
- [ ] 10.4 前端：新增 `/admin/subscriptions`、`/admin/orders`、`/admin/payment-channels`
- [ ] 10.5 后端：admin billing API 单测

### 11) Admin：Usage / Tickets

- [ ] 11.1 后端：新增 `/api/admin/usage/*`（日期筛选、明细展开数据源）
- [ ] 11.2 后端：新增 `/api/admin/tickets/*`
- [ ] 11.3 前端：新增 `/admin/usage`、`/admin/tickets`、`/admin/tickets/:id`
- [ ] 11.4 后端：admin usage/tickets API 单测

### 12) Admin：OAuth Apps / Settings

- [ ] 12.1 后端：新增 `/api/admin/oauth-apps/*`（list/create/update/rotate-secret）
- [ ] 12.2 后端：新增 `/api/admin/settings/*`（app_settings 读写）
- [ ] 12.3 前端：新增 `/admin/oauth-apps`、`/admin/oauth-apps/:id`、`/admin/settings`
- [ ] 12.4 后端：oauth-apps/settings API 单测

### 13) 质量门禁与回归

- [ ] 13.1 运行后端测试：`go test ./...`
- [ ] 13.2 运行前端门禁：`cd web && npm run lint && npm run build`
- [ ] 13.3 关键链路回归清单（手工）：登录 → Console 全功能；root 登录 → Admin 全功能
