# Web SPA（Vite + React）

本模块记录 Realms 的前端 SPA（`web/`）组织方式与 UI 风格约定。

---

## 1. 基本信息

- **目录**：`web/`
- **框架**：Vite + React + React Router
- **对齐目标**：new-api 的“后端只提供 API + SPA fallback；页面路由由前端负责”

## 2. 路由范围（当前）

用户侧：
- `/login`
- `/register`
- `/oauth/authorize`
- `/dashboard`
- `/announcements`
- `/tokens`
- `/models`
- `/usage`
- `/account`
- `/subscription`
- `/topup`
- `/pay/:kind/:orderId`（及 success/cancel）
- `/tickets`（含 open/closed/new/detail）

管理侧（root）：
- `/admin`
- `/admin/channels`
- `/admin/channel-groups`
- `/admin/models`
- `/admin/users`
- `/admin/subscriptions`（含 `/admin/subscriptions/:id`）
- `/admin/orders`
- `/admin/payment-channels`
- `/admin/usage`
- `/admin/tickets`（含 open/closed/detail）
- `/admin/announcements`
- `/admin/oauth-apps`（含 `/admin/oauth-apps/:id`）
- `/admin/settings`

## 2.1 Playwright E2E（路由覆盖口径）

为减少回归风险，仓库引入基于 Playwright（Chromium Desktop）的 Web E2E：

- **测试目录**：`web/e2e/`
- **配置**：`web/playwright.config.ts`
- **本地运行**：
  - 一键：`npm --prefix web run test:e2e`
  - 仅跑测试（假设已 build）：`npm --prefix web run test:e2e:ci`
- **初始化/自洽环境**：
  - 测试启动器：`cmd/realms-e2e`（启动时自动创建临时 SQLite 并 seed 必要数据）
  - Playwright 通过 `webServer` 启动该服务并等待 `/healthz`

**“覆盖率=100%”口径（本模块 SSOT）：**
- 本节“路由范围（当前）”中列出的 **每个路由** 必须至少有 1 条 E2E 用例覆盖（smoke case：能打开 + 关键标题可见）。
- 对“含动态路由”的条目（例如 tickets/pay/admin subscriptions/oauth apps），至少覆盖：
  - 列表页/入口页 1 条
  - 详情页（`:id`）至少 1 个代表实体

> 说明：如未来新增/删除/迁移路由，本文件的路由清单与 E2E 用例需同步更新。

## 3. 视觉基线（SSOT）

SPA 的视觉与布局参考 **tag `0.3.3`** 的 SSR 模板（当前仓库已移除 SSR 代码；如需对照请通过 git 查看历史文件）：
- `git show 0.3.3:internal/web/templates/base.html`
- `git show 0.3.3:internal/admin/templates/base.html`

目标是“登录后应用布局（sidebar + top-header + content-scrollable）”与“登录/注册公共布局（simple-header + 居中卡片）”复刻该基线，并在以下点上做统一化处理：

- **侧栏统一（新约定）**：为避免用户侧/管理侧左侧栏观感不一致，`AppLayout` 与 `AdminLayout` 的 `.sidebar-link` spacing 统一采用同一套值（以管理侧紧凑样式为基线），并避免在布局层额外用 `gap` 调整密度。

## 4. 关键文件

- `web/index.html`
  - CDN 引入 Bootstrap 5、Google Fonts（Inter/JetBrains Mono）、Material Symbols
  - favicon 指向 `/assets/realms_icon.svg`
  - 为复刻旧版 Dashboard/Usage，引入 `flatpickr` 与 `chart.js`（CDN）

- `web/src/index.css`
  - 主要参考 tag `0.3.3` 的 SSR 模板样式（已删除，需用 `git show` 对照）：
    - 背景渐变、卡片/表格/表单基调、sidebar/top-header 布局等
    - Dashboard/Usage 的页面级补充：`border-dashed`、`metric-card`、`quick-range-btn`、`rlm-usage-*` 等

- `web/src/layout/PublicLayout.tsx`
  - 登录/注册公共布局（simple-header + footer）
  - 通过 `/healthz` 读取 `allow_open_registration` 控制“注册”入口是否展示

- `web/src/layout/AppLayout.tsx`
  - 登录后应用布局：sidebar（含“管理”分组）+ top-header 用户下拉 + 内容区滚动 + footer

- `web/src/layout/ProjectFooter.tsx`
  - 复刻旧版页脚信息（不展示版本号）

- `web/src/auth/AuthContext.tsx`
  - 根据 `user` 状态切换 `documentElement/body` 的 `app-html/app-body` class，使滚动行为与布局对齐旧版

- `web/src/components/BootstrapModal.tsx`
  - SPA 侧统一的 Bootstrap modal 组件封装（用于“新增/编辑/导入”等小窗交互）

- `web/src/components/modal.ts`
  - modal 关闭 helper（用于表单提交后主动关闭弹窗）

## 5. 页面与接口映射（复刻 0.3.3）

- OAuth Authorize
  - 页面：`web/src/pages/OAuthAuthorizePage.tsx`
  - 接口：`GET /api/oauth/authorize`（prepare）、`POST /api/oauth/authorize`（approve/deny）
  - 换取 token：`POST /oauth/token`

- Dashboard
  - 页面：`web/src/pages/DashboardPage.tsx`（UI 参考 tag `0.3.3` Dashboard 结构）
  - 接口：`GET /api/dashboard`

- Tokens（用户侧：API 令牌）
  - 页面：
    - `web/src/pages/TokensPage.tsx`（列表/创建/撤销/删除/重新生成；查看/隐藏/复制；创建/重新生成后用弹窗展示 token）
    - `web/src/pages/TokenCreatedPage.tsx`（legacy：保留路由 `/tokens/created`，但主流程不再跳转）
  - 接口：
    - `GET /api/token`（列表：仅 `token_hint` 预览，不下发明文）
    - `POST /api/token`（创建：返回一次 token 明文）
    - `GET /api/token/:token_id/reveal`（按需查看：返回 token 明文；`Cache-Control: no-store`；撤销态不允许）
    - `POST /api/token/:token_id/rotate`（重新生成：返回一次新 token 明文）
    - `POST /api/token/:token_id/revoke`（撤销：撤销后不可 reveal）
    - `DELETE /api/token/:token_id`（删除）
  - 说明：
    - 为安全起见，列表页默认隐藏 token；点击“查看/复制”时才调用 reveal 接口
    - 升级前创建的旧 token 可能无法 reveal（服务端无明文），需要 rotate 后才能查看/复制

- Usage
  - 页面：`web/src/pages/UsagePage.tsx`（UI 参考 tag `0.3.3` Usage 结构；区间筛选样式对齐 `/admin/channels` 的 row g-2 date 输入）
  - 接口：`GET /api/usage/windows`、`GET /api/usage/events`、`GET /api/usage/events/:event_id/detail`

- Admin Usage（管理后台：全站用量统计）
  - 页面：`web/src/pages/admin/UsageAdminPage.tsx`（区间筛选样式对齐 `/admin/channels`）
  - 接口：`GET /api/admin/usage`、`GET /api/admin/usage/events/:event_id/detail`

- Admin Models（管理后台：模型管理）
  - 页面：`web/src/pages/admin/ModelsAdminPage.tsx`（UI 参考 tag `0.3.3` 旧版管理后台）
  - 接口：
    - `GET /api/models/`（列表）
    - `POST /api/models/library-lookup`（models.dev 填充）
    - `POST /api/models/import-pricing`（导入价格表）

- Admin Channels（管理后台：上游渠道）
  - 页面：`web/src/pages/admin/ChannelsPage.tsx`（UI 参考 tag `0.3.3` 旧版管理后台）
  - 接口：
    - `GET /api/channel/page`（列表 + 区间统计 + 运行态：默认今天）
    - `POST /api/channel/reorder`（拖拽排序持久化）
    - `GET /api/channel/pinned`（渠道指针信息）
    - `POST /api/channel/:channel_id/promote`（设为指针）
    - `GET /api/channel/:channel_id`（渠道详情：包含“渠道属性/请求处理设置” + overrides/filters/mapping）
    - `GET/POST/DELETE /api/channel/:channel_id/credentials`（密钥管理：列表/新增/删除）
    - `PUT /api/channel/:channel_id/meta`（渠道属性：组织 ID/默认测试模型/Tag/权重/自动封禁/备注）
    - `PUT /api/channel/:channel_id/setting`（请求处理设置：推理合并/透传/代理/系统提示词）
    - `PUT /api/channel/:channel_id/param_override`
    - `PUT /api/channel/:channel_id/header_override`
    - `PUT /api/channel/:channel_id/model_suffix_preserve`
    - `PUT /api/channel/:channel_id/request_body_whitelist`
    - `PUT /api/channel/:channel_id/request_body_blacklist`
    - `PUT /api/channel/:channel_id/status_code_mapping`
    - `GET /api/channel/:channel_id/models`（模型绑定：列表）
    - `POST /api/channel/:channel_id/models`（模型绑定：新增）
    - `PUT /api/channel/:channel_id/models`（模型绑定：更新 `status/upstream_model`）
    - `DELETE /api/channel/:channel_id/models/:binding_id`（模型绑定：删除）

## 6. 页面开发约定（UI）

- 页面组件尽量只输出“内容区”部分；布局统一由 `PublicLayout/AppLayout` 承担
- 优先使用 Bootstrap 结构类（`card`/`table`/`form-control`/`btn`/`alert` 等）
- 图标统一使用 Material Symbols（`material-symbols-rounded`）
- **Modal 约定（重要）**：为对齐 SSR 并避免 backdrop/层级异常，SPA 的 Bootstrap modal 应通过 `BootstrapModal` 渲染，并使用 Portal 挂到 `document.body`（不要将 `.modal` 长期挂在滚动容器/卡片内部）
- **Admin Channels 约定**：`/admin/channels` 的渠道配置项必须收敛到“设置”弹窗（`editChannelModal`，tab：常用/密钥/模型绑定/高级），不要拆成独立弹窗/独立入口（例如 `channelModelsModal` / `channelKeyModal` 这类模式）
- **渠道模型绑定约定**：在“模型绑定”tab 采用“模型选择 + 模型重定向（对外模型 → 上游模型）”交互。取消选择会将绑定置为禁用（`status=0`）但保留记录，重选时可恢复重定向，避免来回切换丢失配置
- **用量筛选约定**：`/usage` 与 `/admin/usage` 的区间筛选统一采用与 `/admin/channels` 一致的 row g-2 小表单（`type=date` + `form-control-sm`），避免胶囊式筛选造成不一致观感
- **时区输入约定**：需要“时区选择”的输入框统一使用 `TimeZoneInput`（`input[list] + datalist`），复刻 SSR 的候选提示体验（如 `Asia/Shanghai`、`UTC` 等），避免退化为纯文本输入
- **返回按钮约定（新）**：页面不在右上角提供“返回/返回列表”按钮（含 `arrow_back`）。导航依赖左侧栏与局部 breadcrumb；404 等异常页可提供“前往…”按钮，但避免使用“返回”文案
- **Typography 约定**：字体与字号优先由 `web/src/index.css` 与 Bootstrap 统一控制；避免在 JSX 中随意写 `style={{ fontSize }}`/`style={{ fontFamily }}`。需要更小字号时优先使用 `small/smaller`；技术字符串使用 `font-monospace`，不要引入随机的 monospace 字体名
