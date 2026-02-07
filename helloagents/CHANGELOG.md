# Changelog

本文件记录项目所有重要变更。
格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/),
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增

- **[GitHub/Collaboration]**: 新增社区协作模板：`Issue` 使用结构化表单（Bug 报告 + Feature 请求 + 安全/文档分流配置），并新增 `PR` 模板统一要求“变更概述、关联 Issue、验证方式、兼容性风险与检查清单”，对齐主流开源项目的提交流程实践（`.github/ISSUE_TEMPLATE/bug_report.yml`、`.github/ISSUE_TEMPLATE/feature_request.yml`、`.github/ISSUE_TEMPLATE/config.yml`、`.github/pull_request_template.md`）

- **[Models/API/Web]**: 模型目录新增 `group_name` 分组能力，并打通“管理端可配置 + 用户端按组可见 + 页面分组/归属方二级展示”全链路；`/api/user/models/detail` 与 `/api/models/` 返回 `group_name`，admin create/update 支持设置分组（空值归一到 `default`）；SQLite schema + MySQL migration 新增字段与索引，导入导出支持 `group_name`，并补充组过滤与默认回退测试（`internal/store/managed_models.go`、`internal/store/schema_sqlite.sql`、`internal/store/migrations/0051_managed_models_group_name.sql`、`router/models_api_routes.go`、`web/src/pages/ModelsPage.tsx`、`web/src/pages/admin/ModelsAdminPage.tsx`、`tests/store/sqlite_grouping_test.go`）
  - 方案: [202602060620_model-visibility-grouping](plan/202602060620_model-visibility-grouping/)

- **[Web/Frontend]**: 用户模型页视觉重做，对齐 `new-api` 交互布局：左侧分组选择、右侧模型单行表格（每个模型仅一行），并优化分组选中态、表头层级与行 hover 反馈；使用 Playwright 登录 `pwadmin` 账号完成 `/models` 页面实测与截图核对（`web/src/pages/ModelsPage.tsx`、`web/src/index.css`、`output/playwright/models-newapi-style-default.png`、`output/playwright/models-newapi-style-selected.png`）
  - 方案: [202602060620_model-visibility-grouping](plan/202602060620_model-visibility-grouping/)

- **[Models/Web/Test]**: 明确并固化模型可见性规则：用户仅可见其所属用户组（`group_name`）内模型；前端展示改为“左侧归属方显示（不暴露 `default` 等原始组名）+ 右侧按 `owned_by` 二级归属方分组”，每个模型保持单行；补充 SQLite 回归测试覆盖多组查询不泄露异组模型，并用 Playwright 截图验证（`internal/store/managed_models.go`、`tests/store/sqlite_grouping_test.go`、`web/src/pages/ModelsPage.tsx`、`web/src/index.css`、`output/playwright/models-owner-grouping-default.png`、`output/playwright/models-owner-grouping-selected.png`、`output/playwright/models-owner-display-name.png`）
  - 方案: [202602060620_model-visibility-grouping](plan/202602060620_model-visibility-grouping/)

- **[CI/Web]**: 新增 Playwright Web E2E（Chromium Desktop）：通过 `cmd/realms-e2e` 自动创建临时 SQLite + seed 数据，自洽覆盖 SPA 路由，并在 CI 增加 `e2e-web` job（`.github/workflows/ci.yml`、`cmd/realms-e2e/main.go`、`web/playwright.config.ts`、`web/e2e/*`）

- **[Web/E2E]**: 新增用量页隐私回归：用户侧 `/usage` 明细不展示上游渠道信息（`web/e2e/usage.spec.ts`）

- **[Web/Frontend]**: 用量统计页“请求明细”下拉详情展示对应 API 令牌（Key）名称，便于按 Key 定位请求来源（`web/src/pages/UsagePage.tsx`、`web/e2e/usage.spec.ts`）

- **[Web/Frontend]**: 用量统计页“请求明细”下拉详情补充费用计算明细（输入/输出/缓存输入/缓存输出 × 各自单价），便于审计每次请求的计费组成（`web/src/pages/UsagePage.tsx`、`web/e2e/usage.spec.ts`）

- **[Docker]**: 增加后端专用镜像（用于前后端分离部署）：在同一仓库名 `flowerrealm/realms` 下发布 `:backend` / `:<TAG>-backend` tag（不 embed 前端产物）；默认镜像仍为前后端同源一体（embed `web/dist`）（`Dockerfile`、`.github/workflows/docker.yml`、`docs/frontend.md`、`.env.example`、`docs/USAGE.md`）

- **[Docs]**: 补充“前后端分离（默认同源部署）”用户文档：说明同源一体/外置前端两种部署方式与关键环境变量（`docs/frontend.md`、`docs/index.md`、`docs/USAGE.md`、`mkdocs.yml`、`README.md`）

- **[Web/Frontend]**: 启动 Liquid Glass（苹果液体玻璃）亮色重设计第一轮：升级全局背景光晕、玻璃表面（card/sidebar/top-header/dropdown）与基础动效（路由内容过渡、按钮高光、下拉弹出），并补齐方案包文档（`web/src/index.css`、`web/src/layout/PublicLayout.tsx`、`web/src/layout/AppLayout.tsx`、`web/src/layout/AdminLayout.tsx`、`helloagents/plan/202602030640_frontend_liquid_glass/*`）

- **[Router/API]**: 新增 `/api/user/login`、`/api/user/logout`、`/api/user/self`（cookie session），并支持 `SESSION_SECRET`（未设置时运行期随机生成，重启会导致会话失效）（`router/user_api_routes.go`、`internal/server/app.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Router/API]**: 新增 `/api/user/register`，按 `allow_open_registration` 与邮箱验证码开关（email verification）执行注册与自动登录（`router/user_api_routes.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Router/API]**: 新增 `/api/token`（创建/列表/重新生成/撤销/删除），用于前端 Token 管理（`router/tokens_api_routes.go`）
  - 说明: session user id 对齐 new-api，key 从 `user_id` 改为 `id`（`router/session.go`、`router/user_api_routes.go`、`internal/codexoauth/flow.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Web/Frontend]**: 新增 `web/`（Vite + React）前端工程，提供 `/login`、`/register`、`/dashboard`、`/tokens`、`/models`、`/usage` 页面（`web/*`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Web/Frontend]**: 补齐 SPA 控制台/管理后台缺失页面，恢复 tag `0.3.3` 功能完整性：用户侧增加公告/账号/订阅/充值/支付/工单入口；管理侧补齐分组/用户/订阅套餐/订单/支付渠道/用量统计/工单/公告/OAuth Apps/系统设置等页面（`web/src/pages/*`、`web/src/pages/admin/*`、`web/src/api/admin/*`、`web/src/layout/AdminLayout.tsx`、`README.md`）
  - 方案: [202601310120_restore-console-admin-features](plan/202601310120_restore-console-admin-features/)

- **[Router/API]**: 新增 `/api/usage/windows`、`/api/usage/events`（session user），用于前端用量查询（`router/usage_api_routes.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Router/API]**: 为复刻 tag `0.3.3` 控制台页面，新增/补齐接口：`/api/dashboard`、`/api/user/models/detail`、`/api/usage/events/:event_id/detail`，并扩展用量返回字段（`router/dashboard_api_routes.go`、`router/models_api_routes.go`、`router/usage_api_routes.go`、`internal/store/usage.go`）
  - 方案: [202601301534_spa-style-restore](plan/202601301534_spa-style-restore/)

- **[Router/API]**: 新增模型目录与渠道模型绑定相关接口：`/api/models`、`/api/user/models`、`/api/channel/:channel_id/models...` 等（root/session 分层）（`router/models_api_routes.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Router/API]**: 新增 `/api/channel`（root session）用于渠道管理最小闭环（列表/创建/查询/删除），并在前端补齐 `/admin/channels`（`router/channels_api_routes.go`、`web/src/pages/admin/ChannelsPage.tsx`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Docker]**: Dockerfile 增加前端构建 stage（npm），并在 Go 构建阶段以 `-tags embed_web` embed `web/dist`，实现单镜像同源部署（`Dockerfile`、`webdist_embed.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Release]**: 增加非 Docker 发布产物：Debian/Ubuntu `.deb`（systemd 服务 + `/etc/realms/realms.env` 配置）、Windows zip（`realms.exe`）、macOS tar.gz（darwin/amd64、darwin/arm64），并在 tag push 时通过 GitHub Actions 自动构建并上传到 GitHub Release（`scripts/build-release.sh`、`scripts/build-deb.sh`、`packaging/debian/*`、`.github/workflows/release.yml`、`README.md`、`docs/USAGE.md`）
  - 方案: [202601292030_packaging-deb-exe](plan/202601292030_packaging-deb-exe/)

### 修复

- **[Tests/E2E]**: 新增倍率回归端到端测试：覆盖按量计费（多用户分组倍率连乘）与订阅计费（仅按用户分组倍率计费，订阅分组仅用于购买权限）两条真实 `/v1/responses` 请求链路，验证 `usage_events.committed_usd` 与余额/订阅扣费结果符合预期（`tests/e2e/billing_multiplier_test.go`）

- **[Usage/Billing]**: 请求详情新增“完整金额计算流程”输出：后端 `/api/usage/events/:event_id/detail` 与 `/api/admin/usage/events/:event_id/detail` 现在返回可还原计费过程的 `pricing_breakdown`（token 拆分、单价分项、基础费用、用户分组倍率、订阅分组信息、生效倍率、最终费用与差值）；前端用户侧与管理侧详情面板同步展示完整公式链路（`router/usage_pricing_breakdown.go`、`router/usage_api_routes.go`、`router/admin_usage_api_routes.go`、`web/src/api/usage.ts`、`web/src/api/admin/usage.ts`、`web/src/pages/UsagePage.tsx`、`web/src/pages/admin/UsageAdminPage.tsx`、`router/usage_api_routes_test.go`）

- **[Billing/Quota]**: 修复“用户分组倍率”在实际计费时未正确叠加的问题；新增按用户分组聚合倍率逻辑，并接入订阅与按量计费 `Reserve/Commit` 流程；订阅分组仅用于购买权限校验，不参与计费倍率；补充配额层回归测试覆盖叠加行为（`internal/quota/group_multiplier.go`、`internal/quota/subscription.go`、`internal/quota/hybrid.go`、`internal/quota/quota.go`、`internal/quota/group_multiplier_test.go`）

- **[Usage/API]**: 下线 `pricing_breakdown.subscription_group_multiplier` 与 `pricing_breakdown.subscription_group_applied` 字段，避免“订阅分组参与倍率”歧义；保留 `subscription_group` 仅作套餐权限信息展示（`router/usage_pricing_breakdown.go`、`web/src/api/usage.ts`、`web/src/api/admin/usage.ts`）

- **[Dev/SPA]**: 修复 `make dev` 运行中前端重新构建后白屏问题：SPA fallback 不再只使用启动时缓存的 `index.html`，改为请求时优先读取最新 `web/dist/index.html`（或 embed FS 中的 `index.html`），避免旧 hash 资源 404 导致页面空白；新增回归测试覆盖“运行时更新 dist/index.html 后应立即生效”（`internal/server/app.go`、`router/web_spa_routes.go`、`router/web_spa_routes_test.go`、`router/options.go`）

- **[Dev]**: `make dev` 升级为前后端双热更新：在后端 air 热重载之外，自动启动前端 `npm run build -- --watch` 持续写入 `web/dist`，避免 `web/src` 变更后同源页面不更新（`scripts/dev.sh`、`Makefile`、`.env.example`、`README.md`、`docs/frontend.md`）

- **[Docs]**: 精简 `README` 开发说明，合并“同源联调 / 前端独立 dev server”两种模式，减少 `make dev` 重复描述并统一入口（`README.md`）

- **[Router/Dev]**: 修复 `make dev`（Gin Debug + gzip NoRoute）下 API 未命中路由时的日志噪声：`/api/*` 的 SPA 兜底由 `c.Status(404)` 改为 `c.AbortWithStatus(404)`，避免 `serveError` 在 gzip writer 关闭后再次写入触发 `flate: closed writer`（`router/web_spa_routes.go`、`router/web_spa_routes_test.go`）

- **[Usage/Privacy]**: 用户侧用量“请求明细”彻底移除上游标识与上游明细 body：`/api/usage/events` 不再返回 `upstream_endpoint_id/upstream_credential_id`，`/api/usage/events/:event_id/detail` 不下发 `upstream_request_body/upstream_response_body`；前端在对应区域提示“仅管理员可查看”，并增强 Playwright 回归覆盖失败请求（`router/usage_api_routes.go`、`web/src/pages/UsagePage.tsx`、`cmd/realms-e2e/main.go`、`web/e2e/usage.spec.ts`、`router/usage_api_routes_test.go`）
  - 方案: [202602042228_usage-privacy-hide-upstream-detail](archive/2026-02/202602042228_usage-privacy-hide-upstream-detail/)

- **[Config]**: 同步 `.env` 与 `.env.example`：为缺失的环境变量补齐占位（不改动现有配置），便于升级后补全配置（`.env`）

- **[Dev]**: `air` 热重载排除 `web/node_modules`/`web/dist`/`site`/`dist` 等目录，避免 `make dev` 监听文件过多导致性能问题或触发系统文件句柄上限（`.air.toml`）

- **[Lint]**: 清理 staticcheck 警告：移除 `net.Error.Temporary()` 使用；简化 `TrimSuffix/len(nil map)`；删除无效初始赋值（`internal/codexoauth/*`、`internal/api/openai/param_override_engine.go`、`router/billing_api_routes.go`）

- **[Billing]**: 修复配额 provider 组装：默认使用“订阅优先 + 余额兜底”的 `HybridProvider`；当 `self_mode` 或 `feature_disable_billing` 时切换到 `FreeProvider`，避免余额永不扣减（`internal/server/app.go`、`internal/server/app_test.go`）
  - 方案: [202602011956_billing-cost-balance-fix](archive/2026-02/202602011956_billing-cost-balance-fix/)

- **[Billing]**: PayG 结算支持“补扣差额”：当实际成本 `> reserved_usd` 时尝试追加扣减余额（不足则最多扣到 `0`），并将实际扣到的金额写入 `committed_usd`；新增 SQLite 单测覆盖补扣/返还/扣到 0（`internal/store/usage_balance.go`、`internal/store/usage_balance_test.go`）
  - 方案: [202602011956_billing-cost-balance-fix](archive/2026-02/202602011956_billing-cost-balance-fix/)

- **[Billing/OpenAI]**: 明确计费口径仅依赖上游返回的 tokens（含 cached tokens），忽略响应中的“金额/费用”等字段；新增回归测试防止误读 cost（`internal/api/openai/handler_test.go`）
  - 方案: [202602011956_billing-cost-balance-fix](archive/2026-02/202602011956_billing-cost-balance-fix/)

- **[Tests/E2E]**: 新增 PayG 余额扣费 E2E：验证 `committed_usd > reserved_usd` 时可补扣差额，且忽略上游 `cost` 字段（`tests/e2e/billing_balance_test.go`）

- **[Tests/E2E]**: 新增“余额不足=402(Payment Required)”回归：Go E2E 验证预留阶段直接拒绝且不触发上游请求；Playwright Web E2E 验证“用户管理”余额随一次 `/v1/responses` 调用扣减，以及余额不足时返回 402（`tests/e2e/billing_insufficient_balance_test.go`、`cmd/realms-e2e/main.go`、`web/e2e/seed.ts`、`web/e2e/billing.spec.ts`、`web/playwright.config.ts`）

- **[Web/Frontend]**: 全站移除页面级“返回/返回列表”按钮（含 `arrow_back`），统一依赖左侧栏/面包屑导航；并将 `/admin/channels` 统计区间改为修改日期后自动更新（无需“查询/更新统计”按钮）（`web/src/pages/*`、`web/src/pages/admin/*`、`web/src/pages/admin/ChannelsPage.tsx`）
  - 方案: [202601311927_remove-page-back-buttons](plan/202601311927_remove-page-back-buttons/)

- **[Web/Frontend]**: 管理后台“模型管理”列表不再展示内部 `id`（仅展示对外 `public_id`）（`web/src/pages/admin/ModelsAdminPage.tsx`）

- **[Web/Frontend]**: 恢复 `/admin/channels` 渠道“更多设置项”并将配置入口收敛到“设置”弹窗：移除独立的“模型绑定/查看 Key”弹窗，按 SSR 分区补齐“渠道属性/请求处理设置”与 overrides/filters/mapping；新增渠道凭证管理与设置更新相关 API；并将“模型绑定”交互改为“模型选择 + 模型重定向”（`router/channels_api_routes.go`、`web/src/pages/admin/ChannelsPage.tsx`、`web/src/api/channels.ts`、`web/src/api/channelModels.ts`）
  - 方案: [202601311705_admin-channel-settings-restore](plan/202601311705_admin-channel-settings-restore/)

- **[Web/Frontend]**: 恢复 SPA 前端样式为 tag `0.3.3` 视觉基线：迁移 `base.html/dashboard.html/usage.html` 样式到 `web/src/index.css`，并 1:1 复刻 Dashboard/Models/Usage 页面结构（含 flatpickr + Chart.js CDN 依赖）（`web/index.html`、`web/src/*`）
  - 方案: [202601301534_spa-style-restore](plan/202601301534_spa-style-restore/)
  - 决策: spa-style-restore#D001(样式依赖使用 CDN), spa-style-restore#D002(布局完全复刻 0.3.3 且仅覆盖现有路由)

- **[Web/Frontend]**: 修复 SPA 管理后台“内容缺失/样式不对/交互不一致”：恢复“新增/编辑=弹窗小窗（Bootstrap modal）”交互并补齐中文文案；模型管理新增“从模型库填充（models.dev）/导入价格表（含导入结果明细）”；渠道页补齐“智能调度：渠道指针”展示与“设为指针”，并对齐 SSR 的“按日期区间统计/运行态封禁&fail score/拖拽排序”（`web/src/pages/admin/PaymentChannelsPage.tsx`、`web/src/pages/admin/SubscriptionsPage.tsx`、`web/src/pages/admin/OAuthAppsAdminPage.tsx`、`web/src/pages/admin/OAuthAppDetailPage.tsx`、`web/src/pages/admin/ModelsAdminPage.tsx`、`web/src/pages/admin/ChannelsPage.tsx`、`web/src/api/models.ts`、`web/src/api/channels.ts`）
  - 方案: [202601302328_spa-admin-pages-restore-modals](plan/202601302328_spa-admin-pages-restore-modals/)

- **[Router/API]**: 补齐管理侧配套接口：模型库查询 `/api/models/library-lookup`、定价导入 `/api/models/import-pricing`；渠道指针查询 `/api/channel/pinned` 与设为指针 `/api/channel/:channel_id/promote`；并新增渠道页聚合接口 `/api/channel/page` 与拖拽排序 `/api/channel/reorder`（`router/models_api_routes.go`、`router/models_admin_tools_api_routes.go`、`router/channels_api_routes.go`、`router/channel_pinned_api_routes.go`、`internal/admin/pinned_api.go`、`internal/admin/runtime_api.go`）
  - 方案: [202601302328_spa-admin-pages-restore-modals](plan/202601302328_spa-admin-pages-restore-modals/)

- **[Web/Frontend]**: 修复工单页面样式缺失：管理端 `/admin/tickets` 与 `/admin/tickets/:id` 对齐 SSR 视觉基线（页头按钮组容器、列表卡片阴影、消息气泡/附件 badge、回复区样式等）（`web/src/pages/admin/TicketsAdminPage.tsx`、`web/src/pages/admin/TicketAdminDetailPage.tsx`）
  - 方案: [202601311507_ticket-style-restore](plan/202601311507_ticket-style-restore/)

- **[Web/Frontend]**: 统一用户侧与管理侧左侧栏样式：移除用户侧侧栏列表的额外 `gap`，并统一 `.sidebar-link` 的 padding/margin/radius 与 hover 文字颜色，避免两套界面观感不一致（`web/src/layout/AppLayout.tsx`、`web/src/index.css`）
  - 方案: [202601311546_sidebar-style-align](plan/202601311546_sidebar-style-align/)

- **[Web/Frontend]**: 优化管理后台“上游渠道”页面体验：统计区间控件样式对齐 SSR（放回页头下方 row g-2，始终展开且不显示时区文本）；渠道详情区移除不必要的“文字外框”（code/badge）以节省空间；并将 `BootstrapModal` 通过 Portal 挂到 `document.body`，修复打开弹窗后出现灰幕遮罩但弹窗层级异常的问题（`web/src/pages/admin/ChannelsPage.tsx`、`web/src/components/BootstrapModal.tsx`）
  - 方案: [202601311556_admin-channels-ui-polish](plan/202601311556_admin-channels-ui-polish/)

- **[Web/Frontend]**: 用量统计页区间筛选样式对齐“上游渠道”页：`/usage` 与 `/admin/usage` 改为 row g-2 date 小表单（`type=date` + `form-control-sm`），避免胶囊式筛选造成观感不一致；并移除 `/admin/usage` 右上角“返回”按钮（`web/src/pages/UsagePage.tsx`、`web/src/pages/admin/UsageAdminPage.tsx`）

- **[Web/Frontend]**: 恢复“老的时区选择样式”：系统设置页的时区输入改为 `input[list] + datalist`（带候选提示），并抽成 `TimeZoneInput` 组件供全项目复用（`web/src/pages/admin/SettingsAdminPage.tsx`、`web/src/components/TimeZoneInput.tsx`）
  - 方案: [202601311604_timezone-picker-legacy-style](plan/202601311604_timezone-picker-legacy-style/)

- **[Web/Frontend]**: 统一字体与字号：移除 SPA 中与基线不一致的 admin 专用 `code` 样式；收敛多处页面内手写 `fontSize/fontFamily`，优先使用 `small/smaller/font-monospace` 等统一策略（`web/src/index.css`、`web/src/pages/DashboardPage.tsx`、`web/src/pages/TokensPage.tsx`、`web/src/pages/UsagePage.tsx`、`web/src/pages/admin/*`、`web/src/layout/*`）
  - 方案: [202601311619_typography-unify](plan/202601311619_typography-unify/)

- **[Web/Frontend]**: 修复管理后台“概览/仪表盘”页面内容缺失：补齐统计卡片（用户/渠道/端点）与“今日概览/快捷操作/系统信息”；并按 `admin_base` 细化管理后台样式（链接颜色、侧边栏间距、表格表头颜色、`code` 样式），对齐 tag `0.3.3`（`router/admin_home_api_routes.go`、`router/admin_api_routes.go`、`web/src/pages/admin/AdminHomePage.tsx`、`web/src/api/admin/home.ts`、`web/src/pages/AdminPage.tsx`、`web/src/index.css`）

- **[Router/API]**: 修复 `/api/channel` 相关构建问题：补齐渠道更新基础字段写入（`UpdateUpstreamChannelBasics`）与渠道连通性测试实现，并清理重复/缺失 import 导致的编译错误（`router/channels_api_routes.go`、`internal/store/upstreams.go`）

- **[Scheduler]**: failover 时为同一渠道提供有限重试（当前允许 1 次重试），超过后再切换到“下一个”渠道；当 B 不可用但 A 可用时可更稳定地回归使用 A，同时降低组内 `max_attempts` 被单渠道耗尽的概率（`internal/scheduler/group_router.go`、`internal/scheduler/group_router_test.go`）
  - 方案: [202601282059_channel-failover-fix](plan/202601282059_channel-failover-fix/)

- **[Scheduler/Admin]**: 将“5 分钟最高优先级”升级为“渠道指针（树级 SSOT）”：指针作为“应该使用什么渠道”的唯一标定，作用域覆盖整棵 `default` 渠道树；指针开启后按 Channel Ring（树展开序列）从指针位置开始遍历一圈（到底从头再来），并在指针渠道进入 ban 时自动轮转到下一个渠道（跳过仍处于 ban 的渠道）；同时覆盖会话粘性绑定/亲和；管理后台移除剩余时间展示并改为“设为指针”按钮（`internal/scheduler/channel_ring.go`、`internal/scheduler/group_router.go`、`internal/scheduler/scheduler.go`、`internal/scheduler/state.go`、`internal/admin/templates/channels.html`、`internal/admin/channel_intelligence.go`、`internal/server/app.go`）

- **[Scheduler]**: 渠道指针指向不在默认 Channel Ring 的渠道时，允许运行态将该渠道追加到 ring（尾部）以确保“设为指针”立即生效；避免指针被误判为 invalid 而回退到 ring[0]（`internal/scheduler/state.go`、`internal/scheduler/group_router.go`、`internal/scheduler/scheduler_test.go`）

- **[Admin/UI]**: 上游渠道管理页移除“属性”列，避免右侧按钮文字换行；页头仅展示“渠道指针”，不再展示“最近成功”概览，避免重复概念（`internal/admin/templates/channels.html`、`internal/admin/server.go`、`internal/scheduler/scheduler.go`、`internal/scheduler/state.go`、`internal/scheduler/scheduler_test.go`）

### 变更

- **[CI/Pages]**: GitHub Pages 工作流改为仅在 `push tags` 时触发，移除 `master` 分支 push 与 `workflow_dispatch` 触发，避免每次提交都自动发布文档（`.github/workflows/pages.yml`）

- **[Auth/DB]**: `users.username` 收敛为不可更改 + 唯一 + 区分大小写 + 仅字母/数字：MySQL 增加迁移 `0050_users_username_immutable.sql`（case-sensitive collation + CHECK + trigger），SQLite schema/启动期 ensure 补齐约束；同时禁用用户侧/管理员侧修改账号名接口，并将登录流程改为账号名大小写敏感（`internal/store/migrations/0050_users_username_immutable.sql`、`internal/store/schema_sqlite.sql`、`internal/store/sqlite_schema.go`、`internal/store/username.go`、`router/user_api_routes.go`、`router/account_api_routes.go`、`router/admin_users_api_routes.go`、`web/src/pages/AccountPage.tsx`、`web/src/pages/admin/UsersPage.tsx`）
  - 方案: [202602032206_username_immutable](plan/202602032206_username_immutable/)

- **[Token/Web][Router/API][Store]**: 用户 Token 支持在列表页查看/隐藏/复制，并在创建/重新生成后以弹窗展示 token（不再跳转 `/tokens/created`）：新增 `GET /api/token/:token_id/reveal`（`no-store`）；DB 增加 `user_tokens.token_plain`（撤销时清空）；Token 前缀统一为 `sk_`（`web/src/pages/TokensPage.tsx`、`web/src/pages/TokenCreatedPage.tsx`、`web/src/api/tokens.ts`、`router/tokens_api_routes.go`、`router/oauth_token_public.go`、`internal/store/store.go`、`internal/store/migrations/0049_user_tokens_plain.sql`、`internal/store/schema_sqlite.sql`）
  - ⚠️ EHRB: 敏感数据 - 用户已确认风险
  - 检测依据: 需求涉及 token 明文落库（`token_plain`）与 reveal 接口（`/api/token/:token_id/reveal`）

- **[Router/API]**: 对齐 new-api 的 session API 防 CSRF 策略：移除 `/api/token`、`/api/channel` 的 `X-CSRF-Token` 校验，改为要求 `Realms-User: <user_id>` header 与会话用户一致；前端登录态同步写入 localStorage 并自动附带该 header（`router/tokens_api_routes.go`、`router/channels_api_routes.go`、`router/tokens_api_routes_test.go`、`web/src/api/client.ts`、`web/src/auth/AuthContext.tsx`、`README.md`）

- **[Server/Router]**: HTTP 路由迁移到 Gin：新增仓库根 `router/` 分层组装，并将 Web 路由切换为 SPA fallback（支持 `FRONTEND_BASE_URL` / `FRONTEND_DIST_DIR`）（`internal/server/app.go`、`router/*`、`internal/server/app_test.go`）
  - 方案: [202601292015_frontend-backend-separation](plan/202601292015_frontend-backend-separation/)

- **[Dev]**: `make dev` 不再自动启动 Docker（包含 self_mode 与 MySQL 自动拉起），仅启动本地 air（8080）（`scripts/dev.sh`、`Makefile`、`README.md`、`helloagents/project.md`、`helloagents/wiki/modules/realms.md`）

- **[Codex OAuth]**: Codex OAuth OAuth 客户端参数改为内置默认值：不再读取 `REALMS_CODEX_OAUTH_*`，也不再在渠道 setting 中保存/编辑 `client_id/authorize_url/token_url/scope/prompt`；管理后台仅保留“渠道/端点/账号”相关配置（`internal/config/config.go`、`internal/codexoauth/*`、`internal/server/app.go`、`internal/upstream/executor.go`、`internal/store/admin_export_import.go`、`router/admin_settings_api_routes.go`、`router/channels_api_routes.go`、`.env.example`、`tests/e2e/*`、`web/src/pages/admin/ChannelsPage.tsx`）

### 优化

- **[Scheduler/Admin]**: 渠道指针补齐可解释性信息：记录并展示指针最近一次变更的“更新时间/原因”（手动设置/因封禁轮转/指针无效修正），管理后台在渠道页头通过 hover 提示展示（`internal/scheduler/state.go`、`internal/scheduler/scheduler.go`、`internal/admin/server.go`、`internal/admin/templates/channels.html`、`internal/scheduler/scheduler_test.go`）
  - 方案: [202601291724_channel-pointer-meta](plan/202601291724_channel-pointer-meta/)

- **[Admin/Upstream]**: 上游渠道设置对齐 new-api：新增渠道属性 `openai_organization/test_model/tag/remark/weight/auto_ban` 与 `setting`（`force_format/thinking_to_content/pass_through_body_enabled/proxy/system_prompt/system_prompt_override`），并在转发链路生效：按渠道代理、OpenAI-Organization 注入、system_prompt 注入、pass_through 直透、thinking_to_content SSE 转换、auto_ban 控制封禁；配置导出/导入版本升级到 `6`（兼容 `1/2/3/4/5/6`；MySQL 迁移 `0048_upstream_channels_newapi_settings.sql`；SQLite schema 与启动期自动补齐列）。

- **[Limits]**: 移除所有限流与时长/大小限制：取消 per-token/per-credential 并发护栏与 SSE 连接上限；配额不再拦截请求；HTTP 服务不再设置读写/空闲超时；SSE 不再限制单行长度；工单附件不再限制大小/数量与过期时间；Codex OAuth 不再设置 HTTP/TLS 超时；proxy log 不再限制单条体积与文件数量（见 `internal/api/openai/handler.go`、`internal/server/app.go`、`cmd/realms/main.go`、`internal/upstream/sse.go`、`internal/tickets/storage.go`、`internal/web/tickets.go`、`internal/admin/tickets.go`、`internal/codexoauth/client.go`、`internal/proxylog/proxylog.go`、`internal/config/config.go`、`.env.example`）

- **[Admin/UI]**: 渠道详情页：支持按 `5m/1h/24h` 查看各 Key/账号的请求数、成功/失败、输入/输出 Token、最近使用时间；Codex OAuth 账号展示余额（自定义 URL 渠道余额占位）（`internal/admin/channel_detail.go`、`internal/admin/templates/channel_detail.html`、`internal/admin/templates/channels.html`、`internal/store/usage.go`、`internal/server/app.go`）
  - 方案: [202601281601_channel-key-usage-detail](archive/2026-01/202601281601_channel-key-usage-detail/)

- **[CI]**: Codex CLI E2E 增强断言：真实上游链路执行两次请求并要求第二次命中缓存 Token（`cached_input_tokens>0`），同时保留 fake upstream 缓存用例用于更稳定地覆盖 `cached_tokens` 解析与落库；统一使用“生成最小 Go 程序”的 prompt（`tests/e2e/codex_cli_test.go`）

- **[Web/UI]**: Web 控制台（SSR）URL 自动略去 Query：对 GET/HEAD 含 Query 的请求 302 到无 Query URL，并将 `msg/err/next`、`/usage` 筛选/分页、`/tickets` 状态筛选迁移到 cookie/path；同时支付回跳改为 `/pay/{kind}/{order_id}/success|cancel`，站内不再生成 `?msg/?err/?next`（见 `internal/middleware/strip_web_query.go`、`internal/middleware/flash.go`、`internal/server/app.go`、`internal/web/server.go`、`internal/web/tickets.go`、`internal/web/templates/usage.html`、`internal/web/templates/tickets.html`）

- **[Admin/UI]**: 统一渠道管理界面风格，使用卡片布局替代手风琴，优化 JSON 配置项展示（见 `internal/admin/templates/channels.html`）

  - 方案: [202601271953_unify-channel-ui](archive/2026-01/202601271953_unify-channel-ui/)

- **[Admin/UI]**: 上游渠道列表：将行内图标/独立页面操作整合为“设置”弹窗，在弹窗内完成 base_url、密钥、授权账号、模型绑定，以及分 组/请求字段策略/黑白名单/改写/状态码映射；旧 `/admin/channels/{id}/endpoints` 与 `/admin/channels/{id}/models*` 改为重定向兼容（ 见 `internal/admin/templates/channels.html`、`internal/admin/server.go`、`internal/admin/channel_models.go`）
- **[Admin/UI]**: 渠道设置弹窗移除 Accordion 下拉栏，改为所有设置项默认展开、无分区标题（`internal/admin/templates/channels.html`）
- **[UI]**: 订单列表优化：已取消订单整行变灰（`opacity: 0.5`）并禁用“批准/不批准”按钮
- **[Web/Frontend]**: Web 控制台与管理后台页脚不再展示版本信息，并移除对应的版本信息接口（详见方案包）
  - 方案: [202602011407_remove-footer-version](archive/2026-02/202602011407_remove-footer-version/)
- **[UI]**: 用量统计“请求明细”支持点击展开查看详细信息，并按需加载“转发请求体/上游响应体”（仅失败请求保存），便于排障（见 `internal/web/templates/usage.html`、`internal/admin/templates/usage.html`、`internal/store/migrations/0045_usage_event_details.sql`）
- **[Build]**: Docker 构建支持注入版本信息（`REALMS_VERSION/REALMS_BUILD_DATE`）并在 `/healthz` 输出
- **[Build]**: 版本号不再依赖仓库内写文件：移除 `internal/version/version.txt` 的 embed 兜底，本地默认 `dev`，release 继续由构建注入版本信息（见 `internal/version/version.go`、`.github/workflows/docker.yml`）
- **[Build]**: Dockerfile 支持 buildx 多架构构建（基于 `TARGETOS/TARGETARCH`），用于 `linux/amd64` + `linux/arm64` 镜像发布（见 `Dockerfile`）
- **[Deploy]**: Docker Compose 默认拉取 Docker Hub 镜像 `flowerrealm/realms`（支持 `REALMS_IMAGE` 覆盖/固定 tag），并同步更新部署文档与更新脚本（`docker-compose.yml`、`docs/USAGE.md`、`.env.example`、`scripts/update-realms.sh`）
  - 方案: [202601281400_compose-pull-flowerrealm-realms](archive/2026-01/202601281400_compose-pull-flowerrealm-realms/)
  - 决策: compose_pull_flowerrealm_realms#D001(使用 REALMS_IMAGE 覆盖镜像)

- **[Deploy]**: Docker Compose MySQL 默认暴露端口到宿主机（对外监听），并支持 `MYSQL_HOST_PORT/MYSQL_BIND_IP` 配置；同步更新文档与 `make dev` 自动拉起逻辑（`docker-compose.yml`、`.env.example`、`docs/USAGE.md`、`README.md`、`scripts/dev-mysql.sh`）
  - 方案: [202601281424_mysql-port-expose-configurable](archive/2026-01/202601281424_mysql-port-expose-configurable/)
  - 决策: mysql_port_expose_configurable#D001(使用 MYSQL_HOST_PORT 控制宿主端口映射)

- **[Dev]**: `make dev` 同时启动本地(8080)正常模式 + Docker(7080) self_mode（独立 docker compose project，数据库隔离），并提供跳过/端口覆盖选项（`scripts/dev.sh`、`Makefile`、`README.md`）
  - 方案: [202601281445_make-dev-dual-local-docker-self](archive/2026-01/202601281445_make-dev-dual-local-docker-self/)
  - 决策: make_dev_dual_local_docker_self#D001(docker self 使用独立 compose project)
- **[Upstream]**: 流式首包超时（`limits.stream_first_byte_timeout` / `REALMS_LIMITS_STREAM_FIRST_BYTE_TIMEOUT`）：SSE 在未写回前等待上游首包，超时触发 failover（重新选渠/换 key/换账号），缓解“卡住不出字”问题（见 `internal/api/openai/handler.go`、`internal/api/openai/stream_first_byte.go`）

### 修复
- **[CI]**: 修复 Codex CLI E2E fake upstream 的 Responses SSE：补齐 `usage.total_tokens`，兼容 `codex-cli 0.92.0` 解析要求（`tests/e2e/codex_cli_test.go`、`internal/api/openai/handler_test.go`）
- **[Billing]**: 计费改为完全本地计算 tokens：不再依赖上游返回 `usage`（避免费用回退到预留值导致“固定 `0.014336`”），流式/非流式统一从请求/响应内容解析并用内置 tokenizer 计数；同时预留金额估算加入 `input_tokens`，减少按量计费被低预留额钳制的情况（`internal/api/openai/handler.go`、`internal/api/openai/token_count_local.go`、`internal/quota/*`、`internal/tokenizer/*`）
- **[Web/Auth]**: 修复本机同时运行正常模式与 self_mode 时 Web 会话互相“踢下线”：self_mode 默认使用独立会话 Cookie `realms_session_self`，避免与正常模式共享 `realms_session`（`internal/web/server.go`、`internal/server/app.go`）
- **[Docs]**: 修复 GitHub Pages Pages workflow 的 MkDocs 构建命令：移除无效的 `mkdocs build --site-url` 参数，改为通过临时 config 注入 `site_url`（`.github/workflows/pages.yml`）
- **[UI]**: 修复 Web 控制台 Dashboard 的 RPM/TPM 显示：按“请求/Token ÷ 时间（分钟）”计算（近 1 分钟窗口），并以小数展示，避免低调用量被整型截断为 0（见 `internal/web/server.go`、`internal/web/templates/dashboard.html`）
- **[DB/Time]**: 修复 MySQL 会话时区不一致导致的时间换算与统计偏移：MySQL 连接强制 `time_zone='+00:00'` 且驱动以 `loc=UTC` 解析/编码 DATETIME，避免 `CURRENT_TIMESTAMP/NOW()` 与应用侧 UTC 逻辑混用（见 `internal/store/db.go`；并同步更新 DSN 示例：`.env.example`、`docker-compose.yml`、`docs/USAGE.md`、`internal/config/config.go`）
- **[Usage/UI]**: 修复用量统计“请求明细”中的渠道展示：管理后台显示渠道名称（悬停可见 ID），用户侧仅显示渠道名称（见 `internal/admin/usage.go`、`internal/admin/templates/usage.html`、`internal/web/server.go`、`internal/web/templates/usage.html`）
- **[Usage/Privacy]**: 用户侧用量“请求明细”不再返回/展示上游渠道信息，仅管理后台可见（`router/usage_api_routes.go`、`web/src/pages/UsagePage.tsx`、`web/src/api/usage.ts`）
- **[Billing]**: SQLite 下 `user_balances.usd` 的加减使用 `ROUND(..., 6)` 保持 6 位小数精度，避免浮点误差导致 `0.000001` 级别变动被截断（见 `internal/store/user_balances.go`、`internal/store/usage_balance.go`、`internal/store/topup_orders.go`）
- **[Admin/UI]**: 修复 `/admin/channels` 渠道设置弹窗导致的表格结构错乱与“渲染到外层页面”问题：将 modal 包装进合法的 `<tr><td>` 容器，并限制拖拽排序仅作用于 `tr[data-id]`（`internal/admin/templates/channels.html`）
- **[UI]**: 修复管理后台订阅套餐编辑页表单边框过近问题 (`internal/admin/templates/subscriptions.html`)
- **[Upstream]**: 修复 OpenAI 兼容上游 tokens 字段兼容性：当上游返回 `Unsupported parameter: max_output_tokens` 或 `Unsupported parameter: max_tokens` 时，自动做一次无损改写重试（`max_output_tokens` ↔ `max_tokens`），提升自定义 base_url 渠道可用性（见 `internal/upstream/executor.go`）
- **[Upstream]**: 修复 tokens 字段兼容重试的误改写：仅在请求体确实包含被提示为 unsupported 的字段时才触发互斥改写，并在改写失败时保留原始错误响应（避免“越改越错”导致错误信息被覆盖）（见 `internal/upstream/executor.go`、`internal/upstream/executor_test.go`）
- **[Usage]**: 修复用量事件错误信息记录：当上游返回非 2xx 时，`usage_events.error_message` 记录解析后的错误摘要（优先 `detail` / `error.message` / `message`），用量统计页面可直接看到真实失败原因（见 `internal/api/openai/handler.go`）
- **[Upstream]**: 修复 `Unsupported parameter: max_tokens` 仍出现的问题：在 `/v1/responses` 渠道 `param_override` 后再次规范化 tokens 字段（`max_tokens/max_completion_tokens → max_output_tokens`），并增强上游错误解析避免“建议文案”导致的误改写；同时过滤 `/v1/responses`/`/v1/messages` 的 tokens 查询参数（见 `internal/api/openai/token_alias.go`、`internal/upstream/executor.go`）
- **[Scheduler]**: 修复渠道封禁到期后“难以回归使用”的问题：封禁到期自动进入半开探测（单飞），并在**无流量**场景下由后台自动触发一次渠道测试；探测成功重置渠道失败分值并恢复正常调度；同时将渠道封禁时长上限收敛到 10 分钟（见 `internal/scheduler/state.go`、`internal/scheduler/scheduler.go`、`internal/scheduler/group_router.go`、`internal/server/app.go`、`internal/admin/channel_health.go`）

### 移除
- **[Config]**: 移除对 `config.yaml` 的依赖：服务启动自动加载 `.env`（若存在），仅通过环境变量完成配置（`cmd/realms/main.go`、`internal/config/config.go`、`README.md`、`docs/index.md`、`scripts/dev.sh`、`.env.example`）
- **[Legacy/SSR]**: 移除 SSR Web 控制台与管理后台实现（`internal/web/*`、`internal/admin/*`）及对应路由（`router/web_routes.go`、`router/admin_routes.go`），统一切换为 SPA（`web/`）+ JSON API（`router/*_api_routes.go`）；同时清理 SSR 专用中间件/脚本/测试与未使用的 `internal/limits`，并执行 `go mod tidy`。
- **[Billing/Payment]**: 完全移除旧版 Payment 配置（`REALMS_PAYMENT_*` 环境变量、Admin Settings 旧版 Stripe/EPay 开关与接口），计费/支付链路仅保留“支付渠道（payment_channels）”方案（`internal/config/config.go`、`internal/store/app_settings.go`、`router/admin_settings_api_routes.go`、`router/billing_api_routes.go`、`web/src/api/billing.ts`、`web/src/pages/TopupPage.tsx`、`.env.example`）
- **[Cleanup/Legacy]**: 清理静态检查（staticcheck `U1000`）与 SPA 路由遗留的未使用 helper：移除 `spaStaticMiddleware`、`sessionUserRole`、`setSessionUserUpdatedAt`、`formatTimePtrIn`、user_groups CSV helper、Codex OAuth `claimString`、OpenAI handler response copy helper、token_alias 未使用的 payload/compat normalize，以及已废弃的自定义 dotenv 解析实现（已改用 `github.com/joho/godotenv`）（`router/web_spa_routes.go`、`router/session.go`、`router/auth_middleware.go`、`router/admin_time.go`、`internal/store/user_groups.go`、`internal/codexoauth/flow.go`、`internal/api/openai/handler.go`、`internal/api/openai/token_alias.go`、`internal/config/dotenv.go`、`internal/config/dotenv_test.go`）
- **[Codex OAuth]**: 移除独立回调监听端口与旧兼容开关：删除 `callback_listen_addr` / `REALMS_CODEX_OAUTH_CALLBACK_LISTEN_ADDR` 与 `request_passthrough` / `REALMS_CODEX_OAUTH_REQUEST_PASSTHROUGH`；Codex OAuth 回调仅通过主服务端口的 `/auth/callback`；`redirect_uri` 固定为 `http://localhost:{port}/auth/callback`（从 `server.addr` 推导，不再支持配置）；固定直通 `/v1/responses`（不再兜底 `/responses`）（`cmd/realms/main.go`、`internal/config/config.go`、`router/admin_settings_api_routes.go`、`.env.example`、`internal/codexoauth/codexoauth_test.go`、`internal/upstream/executor.go`、`internal/upstream/executor_test.go`、`internal/upstream/codex_instructions.go`）

### 新增
- **[Docs]**: 新增 GitHub Pages 文档站点（MkDocs Material），并发布 `version.json` / `version.txt` 作为 latest 版本查询入口（见 `.github/workflows/pages.yml`、`mkdocs.yml`、`docs/*`）
- **[CI]**: 新增 GitHub Actions：每次 push 运行 `go test ./...`，并基于 secrets 执行 Codex CLI E2E（Codex CLI → Realms → 上游），同时运行 fake upstream 缓存用例（见 `.github/workflows/ci.yml`、`tests/e2e/codex_cli_test.go`）
- **[CI]**: 新增 GitHub Actions：push tag 时构建并推送 Docker Hub 镜像 `flowerrealm/realms`（`linux/amd64` + `linux/arm64`），并同时推送 `latest`（见 `.github/workflows/docker.yml`）
- **[Admin]**: 用户管理支持手动为用户增加余额（USD）：`/admin/users` 展示余额并提供“加余额”入口（`POST /admin/users/{user_id}/balance`）
- **[API]**: 新增 `POST /v1/chat/completions`（OpenAI Chat Completions 兼容入口）：内部将 Chat 请求转换为 `/v1/responses` 转发，并将上游 Responses 的 JSON/SSE 转换回 Chat 格式返回；复用现有的模型目录、分组路由与计费/用量口径（见 `internal/api/openai/chat_completions.go`、`internal/server/app.go`）。
- **[API]**: 新增 Gemini 兼容接口：`GET /v1beta/models`、`GET /v1beta/openai/models`、`POST /v1beta/models/{path...}`（见 `internal/api/openai/gemini.go`、`internal/server/app.go`）。
- **[API]**: 北向请求体 sanitize：`/v1/responses`/`/v1/chat/completions`/`/v1/messages` 仅保留白名单字段，未知字段静默丢弃，并对 tokens 参数做别名/补齐（参考 new-api）。
- **[Upstream]**: 渠道请求字段策略：按渠道控制 `service_tier` / `store` / `safety_identifier` 的透传与过滤（默认过滤 `service_tier`、`safety_identifier`；可选禁用 `store`），并在 `/v1/responses` 与 `/v1/messages` 转发前生效；管理后台新增配置弹窗；导出/导入版本升级到 `5`（导入兼容 `1/2/3/4/5`；MySQL 迁移 `0041_upstream_channels_request_policy.sql`；SQLite 启动期自动补齐列）。
- **[Upstream]**: 渠道参数改写（`param_override`）：按渠道配置请求体 JSON 路径操作（new-api `operations` 兼容），在 `/v1/responses` 与 `/v1/messages` 每次 selection 转发前应用；failover 到其他渠道时按新渠道重新应用；执行顺序：模型 alias rewrite →（Responses 模型后缀解析）→ 请求字段策略 → 请求体黑白名单 → `param_override`；管理后台新增编辑弹窗；导出/导入版本升级到 `5`（导入兼容 `1/2/3/4/5`；MySQL 迁移 `0042_upstream_channels_param_override.sql`；SQLite 启动期自动补齐列）。
- **[Upstream]**: 渠道请求头覆盖（`header_override`）：按渠道配置上游请求头 JSON 覆盖，支持 `{api_key}` 变量替换；在上游请求构造时生效（默认鉴权最后注入，无法覆盖 `Authorization`）；管理后台新增编辑弹窗；导出/导入版本升级到 `5`（导入兼容 `1/2/3/4/5`；MySQL 迁移 `0043_upstream_channels_header_overrides.sql`；SQLite 启动期自动补齐列）。
- **[Upstream]**: 渠道状态码映射（`status_code_mapping`）：按渠道配置上游错误状态码到对外状态码的映射（JSON 对象），用于兼容部分客户端；仅改写对外 HTTP status code，不影响 failover 判定与用量/审计口径；管理后台新增编辑弹窗；导出/导入版本升级到 `5`（导入兼容 `1/2/3/4/5`；MySQL 迁移 `0043_upstream_channels_header_overrides.sql`；SQLite 启动期自动补齐列）。
- **[Upstream]**: 渠道字段转换与请求体黑白名单：参考 new-api
  - `/v1/responses`：支持将 `model` 的 `-low/-medium/-high/-minimal/-none/-xhigh` 后缀解析为 `reasoning.effort` 并去掉后缀；支持 `model_suffix_preserve` 保护名单跳过解析；并将 `max_tokens/max_completion_tokens` 规范化为 `max_output_tokens`
  - `/v1/messages`：将 `max_output_tokens/max_completion_tokens` 规范化为 `max_tokens`
  - 通用：新增 `request_body_whitelist/request_body_blacklist`（JSON path 数组），并在每次 selection 转发前按渠道应用；failover 时无跨渠道串扰
  - 管理后台 `/admin/channels` 新增 3 个配置弹窗；MySQL 迁移 `0044_upstream_channels_body_filters.sql`；SQLite schema 与启动期自动补齐列；导出/导入版本升级到 `5`（导入兼容 `1/2/3/4/5`）；并补齐单测覆盖
- 文档补全：新增 `LICENSE`、`SECURITY.md`、`CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`，并补齐 `README.md` 与知识库索引入口。
- 开发热重载：增加 `air` 配置、`Makefile` 与 `scripts/dev.sh`，支持文件变更后自动重新编译并重启进程。
- `make dev` 自动尝试启动 MySQL 容器（docker compose：当本机 `127.0.0.1:3306` 未监听时自动 `up -d mysql`）。
- Docker 部署：补全 `docker-compose.yml`，支持 `mysql + realms` 一键启动（环境变量覆盖配置），并新增 `.env.example`。
- 可选构建：新增 build tag `no_webchat`，可在编译期剔除 Web 对话页面（`/chat`）与 `/api/chat/*`；Docker 构建支持 `REALMS_BUILD_TAGS=no_webchat`。
- OAuth Apps：新增 OAuth Provider（`/oauth/authorize`、`/oauth/token`）与管理后台应用管理（`/admin/oauth-apps*`），外部应用可通过授权码交换得到 `rlm_...` 并调用 `/v1/*`。
- Web 控制台补齐导航与页面：模型列表（/models）、订阅/用量（/subscription），并在管理员角色下展示管理后台入口。
- 管理后台「系统设置」新增“对话/搜索”配置：可配置 SearXNG 联网搜索（enable/base_url/timeout/max_results/user_agent），覆盖 `config.yaml` 默认值且无需重启生效。
- 用户体系增强：`users` 新增 `username`（账号名，可用于登录；必填、唯一、区分大小写、不可修改；仅字母/数字）；登录页支持“邮箱或账号名”。
- Web 控制台新增账号设置页面：`GET /account`，支持修改邮箱（验证码）、修改密码（旧密码校验）；账号名只读展示；变更成功后强制登出。
- 自用模式：新增 `self_mode.enable`（或 `REALMS_SELF_MODE_ENABLE`），开启后禁用订阅/订单/充值/支付/工单相关路由与页面，并将数据面配额切换为“仅记录用量，不要求订阅/余额”。
- 管理后台增加上游配置“导出/导入”：新增 `/admin/backup` 页面与 `/admin/export`、`/admin/import` JSON 接口（默认不包含敏感字段）。
- 管理后台端点页补齐上游可解释性信息：展示 credential/account 的运行态 RPM/TPM/Sessions、冷却状态与失败计分，并提示超限原因。
- 管理后台渠道页补齐调度运行态：展示封禁渠道；支持一键将某个渠道设置为“渠道指针”；并在页头展示“渠道指针”概览。
- 管理后台系统设置：当计费功能被禁用（含 self_mode 强制禁用）时隐藏“计费支付”标签页，减少自用形态暴露面与误配置。
- 管理后台「系统设置」新增“功能禁用（Feature Bans）”：可按功能禁用 Web/管理后台部分页面；禁用后隐藏 UI 入口并对对应路由返回 404（对所有用户生效，且不允许禁用系统设置页本身）。
- 配置文件新增 `app_settings_defaults`：为管理后台系统设置提供配置文件默认值（`site_base_url`、`admin_time_zone`、`chat_group_name`、`feature_disable_*`），仅当数据库未覆盖对应键时生效。
- 管理后台「系统设置」Base 标签新增“启动期配置（只读）”清单，展示 config-only 的键名集合，便于排障（避免在后台写回配置文件）。
- 增加设置项同步校验脚本：`scripts/check_settings_sync.py`，用于防止 `config.example.yaml` 与管理后台系统设置键名漂移。
- 管理后台「系统设置」支持分页（tab）分区展示，并为多数字段提供自动保存（AJAX 防抖提交）；保留手动保存与恢复默认。
- Web 控制台与管理后台的模型列表引入模型图标库：基于 `@lobehub/icons-static-svg`（CDN），按 `owned_by/model_id` 映射展示品牌图标。
- 模型管理（SSOT）：新增 `managed_models` 表与管理后台页面 `/admin/models`（白名单/展示元信息）。
- 管理后台模型管理：新增“从模型库填充”（OpenRouter），在新增模型弹窗中按 `model_id` 远程查询并自动填充 `owned_by` 与价格字段（含图标预览）。
- 管理后台模型管理：新增 OpenRouter 远程搜索下拉提示（对外 ID 输入框），默认返回不带前缀的 model_id；同名冲突时提供 `provider/model_id` 选项，避免手动记忆/输入错误。
- 模型缓存定价拆分：`managed_models` 从单一 `cache_usd_per_1m` 拆分为 `cache_input_usd_per_1m` + `cache_output_usd_per_1m`；计费按 cached tokens 子集裁剪后分别结算（含 `cached_output_tokens`），管理后台表单/导入/导出同步升级（`export.version=2`）。
- 修复管理后台模型管理：删除模型按钮在部分页面结构下可能误提交到 `/admin/models`（创建接口），导致报错 `public_id 不能为空`。
- 渠道绑定模型：新增 `channel_models` 表，并在管理后台 `/admin/channels` 的渠道设置弹窗（`?open_channel_settings={id}#models`）维护绑定（channel → models；alias/upstream_model 在此维护）。
- 管理后台补齐用户管理页面：`/admin/users`。
- 管理后台用户管理增强：展示账号名，并支持修改用户邮箱（无需验证码）、账号名与重置密码；变更成功后强制登出目标用户。
- 分组：新增 `channel_groups` 分组字典（`/admin/channel-groups` 管理）；用户分组升级为 `user_groups`（多选；强制包含 `default`），渠道分组为 `upstream_channels.groups`（下拉多选/CSV）；数据面调度按用户组集合筛选可用渠道。
- 分组倍率：`channel_groups.price_multiplier`（默认 1.0），计费按“模型单价 × 分组倍率”；管理后台分组页支持配置倍率。
- 管理后台用户管理支持硬删除用户（仅 root），并级联清理关联数据（Token/Session/订阅/用量/审计等）。
- 管理后台 Channels 支持一键测试上游可用性与延迟，并记录最近一次测试结果（`last_test_*`）。
- 管理后台 Channels 测试增强：使用对话式 prompt 发起 **流式（SSE）** 请求，展示 TTFT 与示例输出（便于排查“像是随机延迟/未真实请求”的错觉）。
- 管理后台 Channels 测试升级：按该渠道的模型绑定（`channel_models`）逐个测试，并支持在“模型绑定”分区手动选择单个模型测试。
- Claude Code / Anthropic Messages：新增 `/v1/messages` 数据面端点与 `anthropic` 上游类型，支持 SSE 透传、failover、运行态 RPM/TPM/Sessions 统计与管理后台配置/测试。
- 管理后台 Channels 列表增加“按渠道用量统计（区间）”：总消耗（USD）、总 Token（含缓存）与缓存命中率；支持 `start/end` 查询，默认今天（按管理后台时区）。
- 管理后台 Channels 页新增“按上游供应商实时统计”：窗口下拉选择（1m/5m/15m/1h/6h/24h）展示请求数与成功率，并提供 `GET /admin/upstream-vendors/stats`。
- 管理后台 Channels 列表：新增“渠道活跃度柱形背景”（按时间分段展示请求量与失败情况），数据来自 `GET /admin/channels/activity`（DB 聚合，支持多实例汇总）。
- 管理后台「系统设置」新增时区配置 `admin_time_zone`（默认 `Asia/Shanghai`），用于控制管理后台时间展示与统计区间解析。
- Codex OAuth Accounts 列表展示订阅状态字段（从 `id_token` 的 `https://api.openai.com/auth` 提取 `chatgpt_plan_type/chatgpt_subscription_active_*`）。
- Codex OAuth Accounts 列表展示订阅额度/速率限制（best-effort：调用 Codex 后端 `GET /wham/usage` 或 `/api/codex/usage`，展示 credits 与 primary/secondary 窗口），并在服务端后台每 10 分钟自动刷新，失败时回显错误信息。
- 管理后台 Codex OAuth Accounts 支持手动刷新单个账号/整个 Endpoint 的订阅额度（立即触发一次刷新，并落库显示）。
- 管理后台 Codex OAuth 账号列表右上角新增 `+` 按钮：弹窗内完成快捷授权/手工录入，收敛添加入口。
- Codex OAuth 支持在管理后台发起授权并换取 token 入库：`redirect_uri` 固定为 `http://localhost:{port}/auth/callback`（从 `server.addr` 推导，不需要也不支持用户配置）。
- 渠道组树形路由：新增 `channel_group_members` 与 `channel_groups.max_attempts`，数据面从 `default` 根组递归选择叶子渠道并 failover；scheduler 增加渠道级自动 ban（连续可重试失败后跳过）；管理后台分组页支持进入组详情、创建子组/添加渠道/拖拽排序。
- Codex OAuth 支持在管理后台粘贴回调 URL 完成授权（远程/端口不可达兜底）。
- Codex OAuth 授权 state 持久化：新增 `codex_oauth_pending` 表缓存 `state/code_verifier`，避免进程重启或多实例导致“state 无效或已过期”。
- 默认会 seed 一个 `codex_oauth` Channel：启动时自动初始化（可手动删除/可新建），管理后台不再限制其创建/删除入口。
- Codex OAuth 上游在 access_token 临近过期时自动使用 refresh_token 刷新并更新入库。
- Codex OAuth 请求策略调整：移除 `request_passthrough` 开关与 legacy `/responses` 兼容改写，固定直通 `/v1/responses` 转发给上游。
- 订阅系统：新增默认套餐（¥12/月；5h=$6，7d=$20，30d=$80），支持控制台下单（购买先创建订单；支付后自动生效；管理后台订单支持批准/不批准；订单状态更新并保留记录；生效后创建订阅记录；多订阅同时有效时按 end_at 最早到期优先扣费），并在数据面按滚动窗口限额拒绝请求。
- 支付与按量计费：新增余额充值与支付页（`/topup`、`/pay/{kind}/{order_id}`），支持 EPay 与 Stripe 支付跳转与回调入账/生效（验签 + 幂等），并在无订阅或订阅额度不足时使用余额按量计费。
- 支付渠道化：新增 `payment_channels` 与管理后台 `/admin/payment-channels`；支付页按渠道选择（每个渠道独立配置），并新增按渠道 Stripe/EPay 回调路由（记录 `paid_channel_id`，修复订阅订单入账不会覆盖已写入的渠道）。
- 支付页支持用户关闭“待支付”订单：标记为“已取消”，若仍完成支付则不自动入账/生效，需人工退款处理。
- 管理后台新增订阅套餐管理：`/admin/subscriptions` 支持新增/编辑套餐，并可配置 5h/1d/7d/30d 额度窗口（留空表示该窗口不限额）。
- 管理后台订阅套餐列表支持强制删除（会级联删除该套餐下的订阅/订单，并解绑相关 `usage_events.subscription_id`）。
- 订阅套餐支持配置分组：`subscription_plans.group_name`（下拉选择）；订阅页仅展示用户所属组内套餐，购买时服务端二次校验。
- 订阅额度窗口新增 1d：`subscription_plans.limit_1d_usd`，并纳入数据面配额校验与 Web 控制台展示。
- 管理后台上游删除升级为硬删除：Channel/Endpoint/Credential/Account 支持彻底删除（带级联删除），并从调度中移除。
- 新增用量查询 API：`GET /api/usage/windows`、`GET /api/usage/events`（TokenAuth）。
- 用量统计增强：新增请求数、输入/输出 Token、缓存输入/输出与缓存比统计，并扩展 `/api/usage/windows`、`/api/usage/events` 与管理后台用量展示。
- 用量统计请求明细：`usage_events` 增加 `endpoint/status_code/latency_ms/error_class/error_message/is_stream/request_bytes/response_bytes` 等字段；用户 `/usage` 与管理员 `/admin/usage` 新增“按请求”明细表。
- 管理后台新增全局用量统计页面：`GET /admin/usage`（仅 root）。
- Web 控制台新增 `API Tokens` 页面：`GET /tokens`（Token 列表/创建/撤销/一键重新生成）；Token 生成页增加一键复制。
- 新增邮箱验证码：SMTP 发送 HTML 邮件，提供 `POST /api/email/verification/send`；并在管理后台提供 `/admin/settings` 可切换开关与 SMTP 配置（DB/UI 覆盖配置文件默认；注册强制校验）。
- 反向代理部署支持：新增 `server.public_base_url` 与 `security.trust_proxy_headers/security.trusted_proxy_cidrs`，并在管理后台「系统设置」新增 `site_base_url`（优先覆盖），用于页面展示的 Base URL、支付回调/返回地址与 Codex OAuth 回跳链接生成。
- 新增 CORS 配置：`security.cors.*`（含预检 OPTIONS），用于浏览器跨域调用 `/v1/*` 与 `/api/*`。
- 新增工单系统（用户支持）：用户控制台 `/tickets`（创建/查看/回复），管理后台 `/admin/tickets`（全量列表/回复/关闭/恢复）；支持附件本地存储（默认 7 天过期清理）与邮件通知（复用 SMTP 配置）。
- 新增公告系统：管理后台 `/admin/announcements` 支持创建/发布/撤回/删除公告；用户控制台 `/announcements` 可查看列表/详情；未读公告在登录进入 `/dashboard` 时自动弹窗提示并标记已读。
- 新增对话功能：用户控制台新增 `/chat` 对话页面（localStorage 保存对话记录）；自动创建名为 `chat` 的数据面 Token；对话功能受“对话分组”控制——启用后对话请求通过 `X-Realms-Chat: 1` 仅使用“对话分组集合”内渠道（组内可 failover）；未启用（未配置/全部禁用）则对话功能关闭（对话请求会被拒绝）（管理后台 `/admin/channel-groups` 支持多选设置/关闭）。
- 对话页新增本地会话管理：会话列表（新建/切换/重命名/删除/搜索）+ 导出（JSON）/分享（Markdown），存储于 localStorage（`realms_chat_sessions_v1`）。
- 对话页增强：支持 Markdown 渲染、代码高亮与代码块复制、附件（图片/文本文件）注入、会话级参数（system prompt/temperature/top_p/max_output_tokens），并新增联网搜索 API（`POST /api/chat/search`；可配置 `search.searxng.*`）。
- 模型管理：`/admin/models` 支持导入价格表（上传/粘贴 JSON），批量 upsert `managed_models` 定价字段（新建默认禁用；已存在仅更新价格）。
- 运维能力：健康检查输出构建版本信息（`GET /healthz`）。
- dev 排障：新增 `debug.proxy_log.*`，可选在 `env=dev` 下对代理失败写入脱敏落盘日志（默认关闭；限大小/限数量）。

### 变更
- **[Upstream]**: 移除渠道/密钥/账号的 `Sessions/RPM/TPM` 限额能力：管理后台不再提供配置入口；调度不再因超限跳过候选；数据库字段已删除（MySQL 迁移 `0047_remove_upstream_limits.sql`；SQLite 初始化 schema 同步更新）。
- 管理后台：将“导出/导入”页面合并到「系统设置」页面（`/admin/settings#backup`），并将 `/admin/backup` 调整为重定向（移除原独立页面模板与左侧导航入口）。
- 计费金额与价格/倍率统一改用小数：数据库与代码字段更新，充值/订阅/用量/模型定价等链路不再使用整数小单位；billing 配置项改为 `min_topup_cny` 与 `credit_usd_per_cny`。
- 用量统计页：将时间筛选与每页数量控件移入“请求明细”列表内部（对齐 new-api）。
- 移除 `group`/租户概念：删除 `groups` 表与所有 `group_id` 字段；上游渠道与定价改为全局配置；移除 `/admin/groups` 与相关 UI。
- 移除 `group_admin`（admin）角色：升级时会将存量 `group_admin/admin` 用户自动降级为 `user`，管理后台仅 `root` 可访问。
- 移除应用层加密：不再需要 `REALMS_MASTER_KEY_BASE64`；上游 API key / OAuth token 明文入库；存量加密凭据在迁移中被禁用；数据面 Token/Web Session 改为 SHA256 hash 并撤销/清理存量会话。
- 清理遗留配置：删除示例 `config.yaml` 中无效的 `security.master_key_base64` 配置项（该服务已不再使用 master key）。
- 订阅扣费语义调整：支持同一用户多条有效订阅同时生效；扣费按 end_at 最早到期优先，依次类推。
- 用户体系：`username` 由可选改为必填（注册/账号设置/管理后台均强制校验）；迁移回填存量 `NULL` 并设置为 `NOT NULL`。
- 前端不再展示/依赖订阅套餐 code 或 ID：用户侧购买按 `plan_id` 提交（隐藏字段）；管理后台订阅套餐页不展示 code/id（不再编辑 code）；`GET /api/usage/windows` 响应不再返回 `plan_code`。
- 数据面增加模型强制策略：`POST /v1/responses` 仅允许已启用且存在可用渠道绑定的模型；在允许的渠道集合内调度，并按选中渠道的 `upstream_model` 执行 alias 重写与上游路由。
- 功能禁用语义收敛：移除 `policy_*`；`feature_disable_billing=true` 表示 free mode；`feature_disable_models=true` 表示模型穿透并关闭 `/models`、`/admin/models*`、`/v1/models`；并自动迁移 legacy keys。
- `GET /v1/models` 不再上游透传，改为从模型目录输出；仅返回存在可用渠道绑定的模型；Web `/models` 同步改为展示模型目录。
- `/admin/models` 改为仅维护模型元信息；模型绑定迁移至 `/admin/channels` 的渠道设置弹窗（`?open_channel_settings={id}#models`）。
- `managed_models` 新增按模型定价（input/output/cache，USD / 1M Token），计费优先使用该定价，并对 prompt cache 命中按 cache 单价结算。
- 移除独立定价规则管理：下线 `/admin/pricing-models` 并移除 `pricing_models` 表，定价仅与模型绑定。
- 管理后台模型定价输入单位改为 USD（支持最多 6 位小数），并以美元图标展示单位。
- Web 控制台 `/models` 与管理后台 `/admin/models` 列表增加模型定价展示（Input/Output/Cache；USD / 1M Token）。
- 品牌改名：服务/控制台/构建产物统一为 `Realms`（入口迁移至 `cmd/realms`，Docker/air 输出 `realms`）。
- 破坏性改名：环境变量前缀从 `CODEX_*` 改为 `REALMS_*`；Web Session Cookie 从 `codex_session` 改为 `realms_session`；示例数据库名从 `codex` 改为 `realms`；新生成 Token 前缀从 `cx_` 改为 `rlm_`。
- 上游渠道配置简化：每个 Channel 固定 1 个 Endpoint；Codex OAuth 渠道支持同一 Endpoint 绑定多个账号；自定义 URL 渠道支持同一 Endpoint 绑定多个 Key。
- Codex OAuth 回调端口策略调整：移除独立回调监听端口（原 `callback_listen_addr`）；`/auth/callback` 仅在主服务端口提供。
- 管理后台收敛：将 `openai_compatible` 的 Keys 管理合并到 `/admin/channels` 渠道设置弹窗（`?open_channel_settings={id}#keys`），移除独立的 OpenAI Credentials 页面入口。
- 管理后台收敛：将 `codex_oauth` 的授权账号管理合并到 `/admin/channels` 渠道设置弹窗（`?open_channel_settings={id}#accounts`），保留旧 `/admin/endpoints/{endpoint_id}/codex-accounts` 为重定向入口。
- `codex_oauth` 的 base_url 回归为普通 Endpoint 字段：创建时默认预填 `https://chatgpt.com/backend-api/codex`，且允许在管理后台编辑/保存。
- base_url 校验策略调整：仅做协议/Host/DNS 校验。
- Web 控制台调整：Token 管理由 `/dashboard` 迁移至 `/tokens`。
- 用户可见文案中文化：Web 控制台、管理后台与 `README.md` 的固定文案统一为中文，并统一“渠道/端点/凭证/令牌”等术语口径；计费单位展示统一为 1M Token。
- 文档/控制台示例调整：移除 HTTP 调用示例，统一改为 Codex CLI 配置模板（`OPENAI_BASE_URL/OPENAI_API_KEY` + `~/.codex/config.toml`）。
- 用量统计口径调整：移除固定 5h/7d/30d 统计窗口，改为用户选择时间区间汇总（默认今天：用户页 `/usage` 与 API 仍为 UTC；管理后台 `/admin/usage` 按管理后台时区），并同步更新 `GET /api/usage/windows`。
- Web 控制台订阅页补齐渠道分组展示（用于区分不同分组的订阅套餐与已购订阅）。
- 站点图标统一：内置 `realms_icon.svg`，新增 `/assets/realms_icon.svg`，并在 Web 控制台/管理后台全站替换品牌图标与 favicon（`/favicon.ico` 永久重定向）。

### 移除
- 移除对话能力：下线 Web 对话页面（`/chat`、`/api/chat/*`）与数据面 `POST /v1/chat/completions`，统一使用 `POST /v1/responses`。
- 管理后台：移除 Proxy 状态页 `GET /admin/proxy-status` 与轮询 API `GET /admin/api/proxy-status`。
- 移除 `managed_models.description` 字段（仅展示用途），Web 控制台 `/models` 不再展示 Description 列。
- 移除 `managed_models.updated_at` 字段（仅展示用途），管理后台 `/admin/models` 不再展示 Updated 列。
- 支付渠道：不再使用 `payment_channels.priority`（管理后台不再展示/提交；列表排序仅按 `id` 倒序）。
- 移除 base_url 地址范围限制相关开关。
- 移除弹窗类提示：管理后台禁用浏览器 `alert/confirm/prompt`，Web `/dashboard` 不再自动弹出未读公告。

### 修复
- Token 管理：重新生成改为覆盖更新（不再新增记录）；已撤销令牌支持删除。
- 修复模型价格表导入：兼容历史字段 `cache_usd_per_1m`；支持 `cache_write_cost_per_token`（用于区分缓存读/写，例如 Claude）；并在缺少 `cache_input_usd_per_1m/cache_output_usd_per_1m` 时自动回填为 `cache_usd_per_1m` 或对应 input/output 价格，避免导入后缓存价格意外变为 0。
- 修复模型库价格读取：使用 OpenRouter 的 `pricing.prompt/pricing.completion`（USD/token）自动换算为 `*_usd_per_1m`；若返回 `input_cache_read/input_cache_write` 则同步换算并填充缓存单价，缺失时缓存单价填 `0`。
- 修复管理后台侧边栏溢出：菜单项过多时导航区域支持滚动，避免底部入口不可见。
- 修复管理后台渠道组详情页渲染失败：成员状态字段为 `*int` 时模板比较导致执行报错。
- 修复上游转发的安全边界：禁止透传 Cookie 与 RFC 7230 hop-by-hop 头；Base URL 推断仅在请求来自 `trusted_proxy_cidrs` 时信任 `X-Forwarded-*` 且校验 proto/host；`request_id` 在 `crypto/rand` 失败时退化为“时间+计数器”避免碰撞。
- Codex OAuth 上游路径策略调整：移除 `/responses` 兜底重试；上游不支持 `/v1/responses` 时直接返回错误（避免“自动切换路径”导致的行为不透明）。
- 修复管理后台用户编辑：修改用户邮箱不再强制验证码校验，且不受邮箱验证码开关影响。
- 修复用量事件在客户端断开/请求 context 取消时可能无法结算/作废：数据面在 commit/void 与请求明细落库时使用后台短超时 context，避免用量长期停留 reserved 或口径不准。
- 用量请求明细展示：不再对用户展示 `client_disconnect`，且默认仅展示已完成请求（过滤 `state=reserved` 的进行中记录）。
- 修复用量统计页快捷区间按钮不自动刷新：点击“今天/昨天/7天/30天”切换日期后会自动提交筛选表单并刷新数据。
- 管理后台用量统计页：不再在页面中展示具体时区名称（如 `Asia/Shanghai`），避免干扰阅读。
- 修复 Web 控制台/管理后台：`msg/err` query 参数污染地址栏与浏览器历史（页面加载后自动移除）。
- 修复管理后台 PRG 跳转干扰：对标记 `data-ajax="1"` 的表单走 AJAX(JSON) 分支，不再通过 URL 携带提示信息，避免影响浏览器前进/后退。
- 修复 `base_url` 校验误拒绝：移除禁用逻辑，避免再次出现误导结论。
- 修复数据库迁移 `0016_managed_models_drop_updated_at.sql`：当目标列已不存在时跳过执行，避免启动阶段迁移失败。
- 修复数据库迁移 `0019_channel_grouping.sql`：当目标列已存在时跳过执行，避免启动阶段迁移失败（Duplicate column name）。
- 修复数据库迁移：新库初始化时计费字段直接使用小数（DECIMAL），不再创建 `*_usd_micros/*_cny_fen` 中间列；旧库升级继续由兼容迁移处理。
- 修复 `codex_oauth` 渠道端点页缺少分组配置入口，导致无法通过 UI 调整该渠道可用用户组。
- 修复分组删除兼容性：不再依赖 MySQL 的 `FIND_IN_SET`；SQLite 下删除分组会正确清理 `user_groups` / `upstream_channels.groups` 引用并重建 `channel_group_members`。
- 修复 SQLite 分组自举：`EnsureSQLiteSchema` 会根据 `upstream_channels.groups` 回填 `channel_group_members`（幂等），避免 default 根组无成员导致路由不可用。
- 修复新建分组的默认挂载：在 `/admin/channel-groups` 新建的“根分组”会自动作为子组挂到 `default` 根组下，保证分组树可达。
- 修复渠道组详情页成员链接/移除按钮：模板中 `*int64` 直接渲染会输出指针地址，导致访问 `/admin/channel-groups/0xc000...` 返回 400；现已改为格式化输出真实 ID。
- 修复管理后台分组删除误提交：移除对 `button.formAction` 的错误兜底，避免删除等表单在 AJAX 提交时误打到当前页面（如 `/admin/channel-groups`）从而提示 `name 不能为空`。
- 修复模型目录按分组筛选：MySQL 引用 `ch.\`groups\``，SQLite 使用 `INSTR` 做 CSV 精确匹配，避免 `FIND_IN_SET` 不可用导致查询失败。
- 修复管理后台分组保存兼容性：当分组表单误提交到 `/admin/channels/{id}/endpoints` 时，自动回退为分组更新，避免错误提示“codex_oauth 不允许修改 base_url”。
- 修复 Channels 分组弹窗保存偶发 400「参数错误」：补齐 `channel_id`，并在 `POST /admin/channels` 兼容按“更新分组”处理。
- 修复渠道测速按钮误提示“已保存”：为测速表单增加 `_intent=test_channel` 并在 `POST /admin/channels`、`POST /admin/channels/{channel_id}` 兜底转交测试逻辑；同时将保存类提示细化为“分组已保存/限额已保存”。
- 修复支付渠道管理“配置详情”页面层级过深：改为在列表页弹窗编辑；`/admin/settings/payment-channels/{id}`（及兼容 `/admin/payment-channels/{id}`）会重定向回列表并自动打开编辑弹窗。
- 修复 `no_webchat` 构建下系统设置页的“对话（Web）”开关误导与污染：标记为“编译期剔除”且不可编辑，并跳过写入 `feature_disable_web_chat`。
- 修复 OAuth Apps 迁移在 MySQL 下索引过长导致的启动失败：`redirect_uri` 改为 hash 唯一索引（`redirect_uri_hash`）。
- 修复服务端构建：`internal/server/app.go` 调用 `web.NewServer` 缺少 `selfMode` 参数导致的编译失败。
- 修复封禁/禁用用户后已登录 Web Session 仍可继续访问的问题：会话鉴权增加 `user.status` 校验，禁用则强制登出。
- dev 环境下 MySQL 目标数据库不存在时，启动阶段自动创建数据库并继续迁移（需账号具备 CREATE DATABASE 权限）。
- 修复数据面 SSE 流式随机断联：引入 StreamPump（大事件行 buffer、idle-timeout，可选 ping），并拆分流式/非流式超时策略，避免固定 2m deadline 与 64KB 单行限制导致的误断联。
- 修复流式 token 统计口径：best-effort 从 SSE `data:` JSON 事件中提取 `usage`（含缓存 tokens）用于结算与请求明细；上游未返回 usage 时保持 reserved 兜底。
- dev 环境下启动阶段等待 MySQL 就绪（最多 30s），降低容器启动竞态导致的连接失败概率。
- 修复 MySQL 迁移执行：迁移文件按语句拆分逐条执行，不再依赖 DSN 的 `multiStatements`。
- 修复 SSR 模板渲染：移除动态模板名调用，改为先渲染内容再注入 base 模板。
- 修复 Codex OAuth 账号额度查询：改为调用 Codex 后端 usage 接口（ChatGPT 上游为 `.../wham/usage`），避免错误走 platform 的 `credit_grants`。
- 修复管理后台 Codex OAuth 展示：额度窗口将“主/次额度”改为“5 小时/每周”，并显示剩余金额（Team：$6/5h、$20/week）与重置时间；订阅进度条显示到期时间与剩余天数。
- 修复 `internal/store/app_settings.go` SQL 字符串包含反引号导致的构建失败（改为普通字符串拼接）。
- 修复 Web 控制台的 Token/Logout 动作错误使用管理员权限链路，导致普通用户无法创建/撤销 Token 或登出。
- 修复 OpenAI 兼容上游 base_url 携带 `/v1` 时的路径拼接（避免 `/v1/v1/*`）。
- 修复 Codex OAuth 上游兼容性：`codex_oauth` 请求强制 `stream=true`、注入 Codex CLI 官方 `instructions`（解决 `Instructions are required/not valid`）、规范化 `input`，删除 Codex 后端不接受的字段（`max_output_tokens/max_completion_tokens/temperature/top_p/service_tier/previous_response_id/prompt_cache_retention`），并补齐 Codex 风格 Header。
- 修复 Codex OAuth 换取 token 偶发超时：新增 `codex_oauth.http_timeout`/`codex_oauth.tls_handshake_timeout`，并对拨号失败/TLS 握手超时做一次安全重试；超时类错误文案提示检查网络/代理。
- 修复渠道健康检查的 SSE 流式测试：当上游缺失 `Content-Type: text/event-stream` 但实际返回 SSE 事件体时，仍可正确解析并判定成功。
- 修复 `internal/server/app.go` 未使用 import 导致的构建失败。
- 修复 managed_models 定价迁移后仍引用 legacy 上游字段导致的构建失败：统一以 `channel_models` 绑定作为调度依据。
- 修复订阅用量/额度口径：为 `usage_events` 增加 `subscription_id` 归属字段，扣费按 end_at 最早到期优先，并避免用量在多个订阅卡片重复统计。
- 修复 Codex OAuth 授权体验：回调页完成后自动刷新原管理页面以显示新账号，并尝试自动关闭回调窗口。
- 修复数据面调度抖动：对上游 `429` 的可重试失败应用更长的凭证冷却窗口，减少短时间内反复撞限流。
- 修复 Codex OAuth 账号余额/订阅异常（`402 Payment Required`）未触发 failover：将 `402` 纳入可重试状态码并自动切换到下一个 credential。
- 补齐数据面测试：覆盖 `prompt_cache_key`（body）优先级与审计事件不记录上游错误 body 的约束。
- 修复 Web 控制台订阅页文案与展示：统一“订阅”用词，并在页面顶部展示当前订阅名称与有效期。
- 修复 CSRF 中间件对 multipart 表单（文件上传）不解析导致的误拒绝：上传附件时可正确读取表单 `_csrf` 字段完成校验。
- 修复对话页轮换对话 Token 的 403（CSRF 校验失败）：`chat.html` 内 CSRF token 注入移除 `printf "%q"`（避免在 JS 语境被 html/template 二次转义成 `"\"csrf_...\""`），并对 `POST /api/chat/token` 同时携带 `X-CSRF-Token` 与表单 `_csrf`。
- 调整对话页输入快捷键：Enter 发送，Ctrl+Enter 换行。
- 修复订阅分组字段兼容性：Web 控制台订阅页展示字段对齐 `subscription_plans.group_name`（迁移后替代旧 `channel_group`）。
- 修复管理后台分组删除失败：统计 `upstream_channels.groups` 时缺少反引号引用，MySQL 8 下会触发语法错误。
- 调整管理后台分组删除逻辑：允许删除仍被用户/渠道引用的分组；删除时自动解绑 `user_groups` 与 `upstream_channels.groups`，若渠道仅属于该分组则自动禁用并回退到 `default`。
- 修复管理后台渠道“测试连接”（闪电按钮）在部分场景返回“参数错误”：测试接口兼容从表单读取 `channel_id`，并增加 `POST /admin/channels/test` 兼容路由。
- 优化管理后台 AJAX 错误提示：统一裁剪长度、规整空白字符；当返回非 JSON（如登录页 HTML）时不再直接弹出原始响应体。
- 修复渠道“测试连接”提示“已保存”的误导：闪电测试表单统一提交到 `POST /admin/channels/test`（携带 `channel_id`），避免部分环境 `/admin/channels/{id}/test` 被改写/误匹配。

### 移除
- 管理后台移除配置文件页（`/admin/config`）与侧边栏入口；移除 YAML 配置文件接口（不再支持 `config.yaml` / `-config`），启动期配置统一通过 `.env`/环境变量维护（修改后重启）。

### 微调
- **[Admin/UI]**: 渠道管理页在使用 `open_channel_settings/msg/err/oauth` 等 query 完成一次性行为后自动清理地址栏（保留统计筛选 `start/end`），避免 URL 长期携带 `?`
  - 类型: 微调（无方案包）
  - 文件: internal/admin/templates/channels.html:1394-1428
- **[Admin/UI]**: 渠道列表页操作列增强：直接展示“测试”、“5分钟最高”、“删除”按钮（常驻显示），减少点击层级；调整表格列宽以适应新按钮；移除“常用设置”中重复的按钮
  - 类型: 微调（无方案包）
  - 文件: internal/admin/templates/channels.html
- **[Web 控制台/管理后台]**: 底栏增加 GitHub 项目申明，并在运行时监测/自动恢复被移除的项目声明
  - 类型: 微调（无方案包）
- **[Web 控制台]**: 固定左侧导航栏，滚动条仅作用于页面主体内容
  - 类型: 微调（无方案包）
  - 文件: internal/web/templates/base.html:3-217, internal/admin/templates/base.html:3-232
- **[Web 控制台]**: 修复控制台图表在无用量数据时 `model_stats=null` 导致前端 `.map()` 报错
  - 类型: 微调（无方案包）
  - 文件: internal/web/server.go:754-758
- **[Web 控制台/管理后台]**: 品牌图标默认无背景（移除白底容器），恢复透明展示
  - 类型: 微调（无方案包）
  - 文件: internal/web/templates/base.html:276-414, internal/admin/templates/base.html:244-246
- **[管理后台模型管理]**: OpenRouter 填充成功后不再在模型列表页顶部显示“已填充”提示
  - 类型: 微调（无方案包）
  - 文件: internal/admin/templates/models.html:415-430
- **[Web 控制台]**: 修复 Token 创建成功页“复制令牌”按钮无效（修正 JS 语法错误并增加剪贴板降级复制）
  - 类型: 微调（无方案包）
  - 文件: internal/web/templates/token_created.html
- **[文档]**: README 聚焦配置并补充 `.env` 变量说明与“从 0 开始（含 git clone）”的部署命令；将部署命令整理到 `docs/USAGE.md`
  - 类型: 微调（无方案包）
  - 文件: README.md, docs/USAGE.md
- **[Build]**: 升级版本号到 `0.1.4`
  - 类型: 微调（无方案包）
  - 文件: internal/version/version.txt:1-1

## [0.1.0] - 2026-01-13

### 新增
- 新增 `codex` Go 单体服务（`cmd/codex/main.go`），支持 `GET /healthz`。
- 新增 MySQL 迁移与核心表结构（用户/分组/Token/会话/上游/审计/定价/用量事件）。
- 新增 Web 控制台（SSR）：注册/登录/登出/会话、Token 自助创建与撤销。
- 新增管理后台（SSR）：上游 channel/endpoint/credential/account 的基础配置（含 SSRF 校验与加密入库）。
- 新增数据面 OpenAI 兼容接口：`/v1/responses`、`/v1/chat/completions`、`/v1/models`（含 SSE 逐事件透传与 failover 边界）。
- 新增最小护栏：per-token 并发、SSE 连接上限、per-credential 并发、默认最大输出 tokens、请求超时。
- 新增测试：调度选择顺序、failover、SSE relay、日志不泄漏 Token。
- 新增运行与部署示例：`config.example.yaml`、`README.md`、`Dockerfile`、`docker-compose.yml`。

### 变更
- 同步更新知识库：`overview/arch/api/data/modules`。
