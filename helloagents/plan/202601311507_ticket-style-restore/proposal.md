# 方案提案：工单页面样式缺失修复（对齐 SSR 基线）

## 背景

在技术栈迁移到 SPA（`web/`）后，工单页面出现“部分样式缺失/观感不一致”的问题。以 SSR 模板为真实性基准（SSOT），需要将 SPA 的工单页面结构与样式对齐到原来的管理后台/用户后台实现。

## 页面差异计划（SSR 基线 vs SPA 现状）

### 页面：`/tickets`（用户：工单列表）

- 原来的内容：`internal/web/templates/tickets.html`
  - 顶部标题「工单」+ 描述 + 「创建工单」按钮
  - 选项卡：全部/进行中/已关闭（`btn btn-sm btn-white border`）
  - 列表表格 + 状态 badge + 「查看」按钮
  - 页尾提示：附件 7 天清理
- 现在的内容：`web/src/pages/TicketsPage.tsx`
- 差异：
  - 页面结构与样式基本一致（无需修改）。

### 页面：`/tickets/new`（用户：创建工单）

- 原来的内容：`internal/web/templates/ticket_new.html`
- 现在的内容：`web/src/pages/TicketNewPage.tsx`
- 差异：
  - 页面结构与样式基本一致（无需修改）。

### 页面：`/tickets/:id`（用户：工单详情）

- 原来的内容：`internal/web/templates/ticket_detail.html`
- 现在的内容：`web/src/pages/TicketDetailPage.tsx`
- 差异：
  - 页面结构与样式基本一致（无需修改）。

### 页面：`/admin/tickets`（管理：工单列表）

- 原来的内容：`internal/admin/templates/tickets.html`
  - 页头：标题/说明 + 右侧「全部/待处理/已关闭」按钮组（包裹在 `card shadow-sm border-0` 内）
  - 列表卡片：`card shadow-sm border-0 overflow-hidden`
  - 表格列：ID / 用户（含头像 icon + code）/ 标题 / 状态 / 最后更新 / 操作
  - 空状态 icon：`ri-customer-service-2-line`
- 现在的内容：`web/src/pages/admin/TicketsAdminPage.tsx`
  - 迁移后顶部结构、表格列与细节样式偏“简化版”，导致观感与 SSR 不一致（被感知为“样式缺失”）。
- 差异（目标修复点）：
  - 对齐页头按钮组容器样式（`shadow-sm`、`btn-white`）。
  - 对齐表格列布局与「用户」列的 icon/代码样式。
  - 对齐空状态 icon 与整体卡片阴影/边框。

### 页面：`/admin/tickets/:id`（管理：工单详情）

- 原来的内容：`internal/admin/templates/ticket_detail.html`
  - 页头：返回按钮（`btn-sm btn-white ... shadow-sm`）+ 关闭/恢复按钮（带 `shadow-sm`）
  - 消息气泡：管理员消息为右侧蓝底卡片（`border-0 shadow-sm`），用户消息为左侧白底卡片（`border shadow-sm`）
  - 附件 badge：管理员侧附件带 `shadow-sm`，使用 `ri-attachment-2` icon
  - 回复区：`card shadow-sm border-0 bg-light`，textarea `shadow-sm`，按钮 `ri-send-plane-fill`
- 现在的内容：`web/src/pages/admin/TicketAdminDetailPage.tsx`
  - 迁移后按钮/卡片/气泡阴影与 icon 不完全对齐（被感知为“样式缺失”）。
- 差异（目标修复点）：
  - 对齐页头返回/关闭/恢复按钮样式与 icon。
  - 对齐消息气泡与附件 badge 的 `shadow-sm`、边框与 icon。
  - 对齐回复区的 card/textarea/button 样式。

## 实施要点

- 以 SSR 模板为“原来的内容”基准，优先调整 SPA 页面结构与 Bootstrap class（而非新增复杂 CSS）。
- 保持页面中文文案；图标与 SSR 对齐（Remix Icon）。

