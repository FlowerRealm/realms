# 方案提案：全站移除页面级“返回/返回列表”按钮 + 渠道统计区间自动更新

## 背景

技术栈迁移到 SPA（`web/`）后，多个页面在右上角/页头新增了“返回/返回列表”按钮（含 `arrow_back` 图标）。用户明确要求**每个页面都不需要返回按钮**，统一依赖左侧栏/面包屑导航。

同时，`/admin/channels` 的统计区间属于“可选信息”，不应出现“查询/更新统计”的强入口（用户反馈：渠道页面不需要“查询”概念）。

## 页面差异计划（SSR 基线 vs SPA 现状）

页面：`/admin/channels`（管理后台：上游渠道）
  原来的内容：`internal/admin/templates/channels.html`
    - 统计区间为页头下方 `row g-2` 的 date 输入
    - 存在“查询统计”按钮（用户要求移除“查询”概念）
  现在的内容：`web/src/pages/admin/ChannelsPage.tsx`
    - 统计区间存在“更新统计”按钮
  差异：
    - 移除“更新统计/查询”按钮，改为**修改日期后自动更新**

页面：`/pay/:kind/:orderId`（用户：支付）
  原来的内容：`internal/web/templates/pay.html`
    - 右侧存在“返回订阅/返回充值”按钮
  现在的内容：`web/src/pages/PayPage.tsx`
    - 右侧存在“返回*”按钮
  差异：
    - 移除页面内返回按钮，依赖侧栏导航

页面：`/tickets/:id`（用户：工单详情）
  原来的内容：`internal/web/templates/ticket_detail.html`
    - 右侧存在“返回”按钮
  现在的内容：`web/src/pages/TicketDetailPage.tsx`
    - 右侧存在“返回”按钮
  差异：
    - 移除页面内返回按钮，依赖侧栏导航

页面：`/announcements/:id`（用户：公告详情）
  原来的内容：`internal/web/templates/announcement.html`
    - 右侧存在“返回”按钮
  现在的内容：`web/src/pages/AnnouncementDetailPage.tsx`
    - 右侧存在“返回”按钮
  差异：
    - 移除页面内返回按钮，依赖侧栏导航

页面：`/tokens/created`（用户：令牌已生成）
  原来的内容：`internal/web/templates/token_created.html`
    - 底部存在“我已保存，返回列表”按钮
  现在的内容：`web/src/pages/TokenCreatedPage.tsx`
    - 底部存在“我已保存，返回列表”按钮
  差异：
    - 移除页面内返回按钮；错误提示不再使用“返回列表”措辞

页面：`/models`（用户：可用模型列表）
  原来的内容：`internal/web/templates/models.html`
    - 右侧存在“返回”按钮
  现在的内容：`web/src/pages/ModelsPage.tsx`
    - 右侧存在“返回”按钮
  差异：
    - 移除页面内返回按钮

页面：`/admin/*`（管理后台：列表页）
  原来的内容：`internal/admin/templates/*`
    - 列表页通常依赖侧栏导航，不提供“返回”按钮
  现在的内容：`web/src/pages/admin/*`
    - 多个列表页右侧新增了“返回”按钮（返回到 `/admin`）
  差异：
    - 移除这些“返回”按钮，避免重复导航与迁移风格不一致

页面：`/admin/tickets/:id`（管理后台：工单详情）
  原来的内容：`internal/admin/templates/ticket_detail.html`
    - 右侧存在“返回”按钮
  现在的内容：`web/src/pages/admin/TicketAdminDetailPage.tsx`
    - 右侧存在“返回”按钮
  差异：
    - 移除页面内返回按钮（保留“关闭/恢复工单”等业务按钮）

页面：`/admin/oauth-apps/:id`（管理后台：OAuth 应用详情）
  原来的内容：`internal/admin/templates/oauth_app.html`
    - 右侧存在“返回列表”按钮
  现在的内容：`web/src/pages/admin/OAuthAppDetailPage.tsx`
    - 右侧存在“返回列表”按钮
  差异：
    - 移除页面内返回按钮

页面：`/admin/subscriptions/:id`（管理后台：套餐编辑）
  原来的内容：`internal/admin/templates/subscriptions.html`
    - 编辑入口在列表页/弹窗交互内（无需返回按钮）
  现在的内容：`web/src/pages/admin/SubscriptionEditPage.tsx`
    - 右侧存在“返回列表”按钮
  差异：
    - 移除页面内返回按钮

页面：`/admin/channel-groups/:id`（管理后台：分组详情）
  原来的内容：`internal/admin/templates/channel_group_detail.html`
    - 依赖 breadcrumb/侧栏导航
  现在的内容：`web/src/pages/admin/ChannelGroupDetailPage.tsx`
    - 右侧存在“返回”按钮
  差异：
    - 移除页面内返回按钮（保留 breadcrumb）

页面：`*`（404）
  原来的内容：无 SSR 对应（或由服务端处理）
  现在的内容：`web/src/pages/NotFoundPage.tsx`
    - 使用“返回控制台/返回登录”
  差异：
    - 将文案改为“前往控制台/前往登录”（避免与“不要返回按钮”的交互约定冲突）

