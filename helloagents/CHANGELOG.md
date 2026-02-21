# 变更日志

## [Unreleased]

### 发布
- **[web/ui]**: 推送前端主题统一与视觉回归基线更新到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`

### 回滚
- **[usability]**: 回滚自 `0.9.3` tag 以来的性能/安全相关提交以恢复可用性（HTTP hardening/body limits、auth cache/invalidation、usage rollup sharding/backfill、probe claim 单飞相关、proxy-aware debug guard 等）
  - 分支: `rollback/0.9.3-usability`
  - 方案: [202602161558_rollback-perf-security-since-093](archive/2026-02/202602161558_rollback-perf-security-since-093/)
  - ⚠️ EHRB: 发布 `master` + 推送 tag `0.10.3` - 用户已确认风险（检测依据: `master(分支)`）

### 重构
- **[admin/channels]**: 移除旧 curl/HTTP probe 测试逻辑，CLI Runner 成为唯一测试方式
  - 方案: [202602181238_remove-curl-test-cli-only](plan/202602181238_remove-curl-test-cli-only/)
  - 移除: 旧 probe 相关函数/类型（15 个函数、3 个类型）、批量测试路由及 `testAllChannelsHandler`
  - 移除: `channelsPageResponse.CLITestAvailable` 字段及前端 `cli_test_available` 类型
  - 变更: `testChannelHandler` 始终委派 `streamChannelCLITestHandler`，未配置 runner URL 时返回明确错误
  - 测试: 删除 `channels_api_routes_probe_test.go`，更新 CLI 单测和 E2E 测试
- **[cleanup]**: 清理未使用代码与前端模板资产
  - 移除: `internal/api/openai/handler.go` 中未使用的 `readLimited`
  - 移除: `tests/e2e/codex_cli_test.go` 中未使用的 `setChannelProbeDueForTest`
  - 移除: `web/src/api/admin/channelGroups.ts` 中未使用的 `listAdminChannelGroupPointerCandidates`
  - 移除: `web/src/format/money.ts` 中未使用的 `formatUSD`
  - 删除: `web/public/vite.svg`、`web/src/assets/react.svg`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险（检测依据: `master(分支)`）
- **[cleanup]**: 删除未被引用的孤儿脚本（保留 `scripts/dev-cli-runner.sh`）
  - 删除: `scripts/dev-mysql.sh`, `scripts/load-curl-responses.sh`, `scripts/mysql-capture-lockwaits.sh`, `scripts/smoke-curl.sh`, `scripts/smoke-curl-sse.sh`, `scripts/update-realms.sh`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险（检测依据: `master(分支)`；提交: `63aaf67`）

### 微调
- **[web]**: `/usage` 页面 1:1 对齐 `/admin/usage`（版式 + 文案 + 字段/表头；补齐"消费排行用户"与明细列；用户侧明细列将"渠道"替换为"Key"）
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/UsagePage.tsx, web/src/pages/usage/*, web/src/index.css
- **[web]**: 登录/注册页右上角导航（nav-pills）激活态从默认蓝色改为主题绿色
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[web/ui]**: 统一“胶囊/标签”淡边框：`.rounded-pill` 默认带淡边框，补齐非 `.badge` 的 pill/标签，避免“有框/无框”混用
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/app-dashboard-linux.png
- **[web/ui]**: 统一“区域外框/按钮”淡边框：`SegmentedFrame` 外框补齐，按钮默认带淡边框（`.btn-link` 例外）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/*
- **[web/ui]**: 统一 tabs/下拉/列表的淡边框：`.nav-tabs`、`.dropdown-item`、`.list-group` 保持与全站一致的细边框与中性文字层级
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/*
- **[web/ui]**: 补齐圆形图标与常见信息块的淡边框：`.rounded-circle` 与 `bg-* + rounded-*` 默认带淡边框，减少“无框漂浮”元素
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/*
- **[web/theme]**: 选定 B 主题并将背景/文字/主题/交互/边界等配色角色化为 tokens（保持鼠尾草绿系不变）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[web/ui]**: 登录后主内容区背景改为白色；顶栏/侧栏与内容区使用淡实线分隔；侧栏导航文字不再使用绿色
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[admin/channels]**: “缓存”统计值不再使用绿色（从 `text-success` 改为中性 `text-muted`，避免把缓存误读为成功态）
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/admin/ChannelsPage.tsx
- **[admin/settings]**: 系统设置页移除非必要绿色文本（tabs 文本改为中性灰；PayGo 状态 badge 不再用 `text-success`），并用淡实线分隔 tabs/header 与内容区域
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/admin/SettingsAdminPage.tsx, web/src/index.css
- **[web/ui]**: 全站增强“圆角实线框”：卡片/表格统一使用淡实线边框并统一圆角，用更清晰的区域框线替代大面积底色分隔
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[web/ui]**: 进一步强化“实线框 + 圆角”一致性（按钮/输入/告警/弹窗/下拉统一圆角；卡片/表格内 `bg-light` 区域回归白底，减少浅绿底色干扰）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[web/ui]**: 边框升级为“更明显更粗的圆滑实线框 + 角处留白”（用 4 条线段围出框线，并保留角部空隙；全站卡片/表格/弹窗/告警/常用 bordered 区块统一 적용）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[web/ui]**: 区域分隔的淡实线加长（减少 `--rlm-segment-sep-inset`，让分隔线两端留白更小）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/*
- **[web/ui]**: 区域框线颜色从灰系改为黑色（更清晰、更有距离感；保留角处留白）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[web/ui]**: 区域框线进一步“更轻 + 更短 + 角留白更大”（降低黑线透明度；增加角留白间距，使线段更短）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css
- **[admin/usage]**: 上游不可用时，仍记录并展示最后一次尝试的渠道（渠道名缺失时回退显示 `#channel_id`）
  - 类型: 微调（无方案包）
  - 文件: internal/api/openai/handler.go, internal/api/openai/handler_test.go, web/src/pages/admin/UsageAdminPage.tsx
- **[admin/usage]**: 上游不可用时，记录并展示“最后一次失败原因”（状态码/上游错误摘要/网络错误）；用户侧 `/usage` 仍仅展示“上游不可用”
  - 类型: 微调（无方案包）
  - 文件: internal/api/openai/handler.go, router/usage_api_routes.go
- **[admin/usage]**: 用量明细展开区的 `Error Message` 改为整行展示并支持换行，避免横向滚动才能看清错误
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/admin/UsageAdminPage.tsx
- **[usage]**: “缓存 tokens（bolt）”提示不再使用绿色，统一为中性灰以避免误读为“成功态”
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/usage/UsageEventsCard.tsx, web/src/pages/admin/UsageAdminPage.tsx
- **[web/ui]**: 引入分段容器 `SegmentedFrame`（外框线段 + 仅在分隔处画线）并推广到所有页面；补齐 Playwright 视觉快照覆盖所有路由页面
  - 方案: [202602201654_segmented-divider-container](plan/202602201654_segmented-divider-container/)
  - 文件: web/src/index.css, web/src/components/SegmentedFrame.tsx, web/src/pages/*, web/src/pages/admin/*, web/e2e/visual-routes.spec.ts, helloagents/modules/web-theme.md
- **[web/ui]**: 分段容器外框线条加粗，并移除最外层左右实线（仅保留上下线段 + 内部分隔线）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, .tmp/ui-theme-spec.md
- **[web/ui]**: 分段容器移除最外层上下线段（外框不绘制）；增大内部区块留白间距（更疏离、更轻盈）
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, .tmp/ui-theme-spec.md, helloagents/modules/web-theme.md
- **[web/ui]**: 分段容器内补充“同段自动分隔”规则：同一 `.row` 里纵向堆叠多个 `col-12` 时自动补分隔线，减少页面分隔不一致
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, .tmp/ui-theme-spec.md, helloagents/modules/web-theme.md
- **[web/ui]**: 修复 `DividedStack` 处理纯空白子节点导致的“空分隔条”；修正多个 admin 页分段改造后的多余闭合标签并更新视觉快照
  - 类型: 微调（无方案包）
  - 文件: web/src/components/DividedStack.tsx, web/src/pages/admin/MainGroupsPage.tsx, web/src/pages/admin/OAuthAppsAdminPage.tsx, web/src/pages/admin/SubscriptionsPage.tsx, web/src/pages/admin/UsersPage.tsx, web/e2e/visual-routes.spec.ts-snapshots/*, .tmp/ui-theme-spec.md, helloagents/modules/web-theme.md
- **[web/ui]**: 增大分隔线（segment/divided/row-stack）的上下留白间距，使横线分隔更“疏离、轻盈”
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/*, .tmp/ui-theme-spec.md
- **[web/ui]**: 统一“文字胶囊/导航项”的淡边框：badge 强制带边框；`nav-pills` tabs 统一描边；侧栏条目不画边框，仅用背景高亮表达当前项
  - 类型: 微调（无方案包）
  - 文件: web/src/index.css, web/e2e/visual-routes.spec.ts-snapshots/*

### 增强
- **[admin/channels]**: 渠道"测试连接"改为可选的 CLI Runner 模式（Codex/Claude/Gemini CLI），测试结果仅在前端展示、不写数据库、不影响渠道可用性调度
  - 方案: [202602171513_cli-channel-test](plan/202602171513_cli-channel-test/)
  - 新增: `tools/cli-runner/`（Node.js HTTP runner 容器，支持三种 CLI）
  - 新增: `docker-compose.channel-test.yml`（可选叠加）
  - 配置: `REALMS_CHANNEL_TEST_CLI_RUNNER_URL` 控制功能开关
  - 前端: 测试按钮和健康徽章列根据配置条件显示/隐藏
  - 安全: 批量测试在 CLI 模式下禁用（405）

### 修复
- **[scheduler/channel-groups]**: 删除 `channel_groups.max_attempts` 与组内 failover 上限，避免多 key/多账号在组路由中被误伤
  - MySQL: 新增迁移 `internal/store/migrations/0061_drop_channel_groups_max_attempts.sql`
  - SQLite: `internal/store/schema_sqlite.sql` 同步移除字段
- **[scheduler]**: 移除 session binding/sticky 逻辑与相关运行态输出，修复多账号“不轮询/不分配使用”与误伤问题
  - 影响范围: `internal/scheduler/*`、`router/channel_runtime_api.go`、`internal/api/openai/*`
- **[codex_oauth]**: 多账号失败分类与标记：区分“余额用尽 / 限流 / 账号不可用”，并在切换到下一个账号前写入持久标记
  - 余额用尽: 写入 `quota_error=余额用尽` + `cooldown_until`（按上游 `resets_at`/`resets_in_seconds`）
  - 限流: 标记 `upstream_throttled` 并短冷却，避免把限流误判为余额耗尽
  - 账号不可用: 401/403 高置信禁用账号（`status=0`）
  - 调度保护: `upstream_exhausted`/`upstream_throttled` 不触发 channel auto-ban
- **[web]**: 全局覆盖 `nav-pills`/`form-switch` 等状态色并调整主题主色为浅绿色，修复 `/admin/settings` tabs/开关仍呈现默认亮蓝色
  - 方案: [202602190938_light-green-theme](archive/2026-02/202602190938_light-green-theme/)
  - 决策: light-green-theme#D001(全局覆盖 `nav-pills` 激活态以避免默认亮蓝色)
- **[e2e]**: 修复 usage Playwright 测试：等待已移除的 `/api/user/models/detail` 改为 `/api/usage/windows`；补回 `rlm-usage-row` / `rlm-usage-detail-row` 类名
  - 类型: 微调（无方案包）
  - 文件: web/e2e/usage.spec.ts, web/src/pages/usage/UsageEventsCard.tsx

### 测试
- **[e2e/codex_oauth]**: 增加虚拟上游回归：多账号在 `usage_limit_reached`/`invalid_token` 下的标记与 failover（余额用尽/禁用账号）
  - 文件: tests/e2e/codex_oauth_multi_account_test.go
- **[playwright/codex_oauth]**: 增加 UI 回归：触发 codex_oauth 多账号 failover 后，`/admin/channels` 账号统计面板展示“余额用尽/已禁用”
  - 文件: web/e2e/codex-oauth-multi-account.spec.ts
- **[e2e]**: 增加回归：`upstream_unavailable` 在管理后台展示“最后一次失败原因”，用户侧仍保持“上游不可用”
  - 文件: web/e2e/upstream-unavailable-details.spec.ts, cmd/realms-e2e/main.go
- **[e2e]**: 增加回归：登录页右上角导航（nav-pills）激活态不使用默认亮蓝色，且命中主题主色
  - 文件: web/e2e/theme-colors.spec.ts
- **[e2e]**: `/models` 用例改为校验“所有已配置的可见模型”都能在模型列表页展示（按 `/api/user/models/detail` 返回为准）；`cmd/realms-e2e` 支持通过 `REALMS_E2E_BILLING_MODELS` 一次 seed 多个模型
  - 文件: web/e2e/routes.spec.ts, cmd/realms-e2e/main.go, web/README.md
- **[playwright/visual]**: 增加全站页面视觉快照回归（逐页截图对比，用于主题/样式改动后的反复修正）
  - 文件: web/e2e/visual-routes.spec.ts, web/package.json
- **[playwright/visual]**: 增加侧栏专用快照与断言，确保 `/dashboard` 与 `/admin` 侧栏导航文字层级不被像素阈值吞掉（默认=muted，激活=heading）
  - 文件: web/e2e/visual-routes.spec.ts, web/e2e/visual-routes.spec.ts-snapshots/*

### 开发体验
- **[dev]**: `make dev` 默认尝试拉起 docker compose `cli-runner` 并设置 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL`，保证管理后台“CLI 渠道测试”本地可用
  - 文件: scripts/dev.sh, scripts/dev-cli-runner.sh, Makefile, README.md
- **[admin/channels]**: “CLI 渠道测试”从只测试第一个绑定模型改为测试该渠道下所有已启用的绑定模型（`test_model` 仍会优先作为首项）
  - 文件: router/channels_api_routes.go

### CI
- **[actions/docker]**: Docker 多架构打包提速：将 `web-build` 固定为 `BUILDPLATFORM` 避免 arm64 下 Node/QEMU；移除 QEMU 初始化；后端镜像构建仅导入缓存以减少重复缓存导出
  - 文件: Dockerfile, .github/workflows/docker.yml
- **[actions]**: 移除 GitHub Actions 的 release 三端打包与 GitHub Pages 文档发布工作流
  - 文件: .github/workflows/release.yml, .github/workflows/pages.yml, README.md, docs/versioning.md, docs/USAGE.md
- **[release]**: 移除本仓库的三端打包/安装包构建脚本与 Debian 打包资源（仅保留 Docker 推镜像链路）
  - 文件: scripts/build-release.sh, scripts/build-deb.sh, packaging/debian/*

### 发布
- **[release]**: 推送 `master` 并发布 tag `0.11.3`
  - ⚠️ EHRB: 发布 `master` + 推送 tag `0.11.3` - 用户已确认风险
  - 检测依据: `master(分支)`、tag 发布
- **[release]**: 推送 `master` 并发布 tag `v0.12.1`
  - ⚠️ EHRB: 发布 `master` + 推送 tag `v0.12.1` - 用户已确认风险
  - 检测依据: `master(分支)`、tag 发布
- **[git]**: 提交并推送“主题浅绿化（tabs/开关去亮蓝）”到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`
  - 文件: web/src/index.css, web/e2e/theme-colors.spec.ts, web/src/pages/DashboardPage.tsx, web/src/pages/admin/AdminHomePage.tsx, web/src/pages/admin/ChannelsPage.tsx, web/src/pages/admin/UsageAdminPage.tsx, web/src/pages/usage/UsageTimeSeriesCard.tsx, helloagents/CHANGELOG.md
- **[git]**: 提交并推送本次变更到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`
  - 文件: helloagents/CHANGELOG.md, web/src/index.css, web/e2e/theme-colors.spec.ts
- **[git]**: 提交并推送本次变更到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`
  - 文件: Makefile, README.md, cmd/realms-e2e/main.go, router/channels_api_routes.go, router/channels_api_routes_cli_test.go, scripts/dev.sh, scripts/dev-cli-runner.sh, web/README.md, web/e2e/routes.spec.ts, helloagents/CHANGELOG.md
- **[git]**: 提交并推送“Docker 多架构构建提速”到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`
  - 文件: Dockerfile, .github/workflows/docker.yml, helloagents/CHANGELOG.md
- **[git]**: 提交并推送“移除 Actions 的 release/pages 发布链路与本地安装包打包脚本”到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`
  - 文件: .github/workflows/release.yml, .github/workflows/pages.yml, .github/workflows/docker.yml, Makefile, README.md, docs/USAGE.md, docs/versioning.md, scripts/build-release.sh, scripts/build-deb.sh, helloagents/CHANGELOG.md
- **[git]**: 提交并推送“多 key/多账号误伤修复 + Codex OAuth 多账号 failover 标记 + 虚拟上游回归”到 `master`
  - ⚠️ EHRB: 推送到 `master` - 用户已确认风险
  - 检测依据: `master(分支)`
  - 提交: `53c50c5`

## [0.10.10] - 2026-02-16

### 微调
- **[admin]**: `/admin/channels` 去除“指针”操作，并将表头“渠道详情”改为“渠道”以避免文案重复
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/admin/ChannelsPage.tsx:933-1510
- **[playwright]**: 指针 e2e 用例改为在渠道组详情页设置指针（不再从渠道列表设置）
  - 类型: 微调（无方案包）
  - 文件: web/e2e/channel-group-pointer.spec.ts:1-118

## [0.10.9] - 2026-02-16

### CI
- **[e2e]**: `TestCodexCLI_E2E` 增加 `REALMS_CI_ENFORCE_E2E` gating，避免 `go test ./...` 在注入 `REALMS_CI_*` 时隐式触发真实上游；真实上游回归由 `scripts/ci-real.sh` 显式执行并可被 retry 包裹
  - 方案: [202602161426_fix-ci-real-e2e-gating](archive/2026-02/202602161426_fix-ci-real-e2e-gating/)

## [0.10.8] - 2026-02-16

### 修复
- **[e2e]**: `cmd/realms-e2e` 内置 fake upstream 支持 Responses SSE（补齐 `usage.total_tokens`），保证 `scripts/smoke-codex.sh` 稳定可用
  - 方案: [202602161127_testing-unify-codex-playwright](archive/2026-02/202602161127_testing-unify-codex-playwright/)

## [0.10.7] - 2026-02-16

### CI
- **[actions]**: 主 CI 统一入口 `make ci`（Codex fake upstream + Playwright seed），新增 `ci-real` 工作流用于真实上游集成回归
  - 方案: [202602161127_testing-unify-codex-playwright](archive/2026-02/202602161127_testing-unify-codex-playwright/)
  - 决策: testing-unify-codex-playwright#D001(主 CI 默认 seed/fake upstream + 可选 ci-real)

### 测试
- **[playwright]**: 新增 Tokens 创建/查看/隐藏用例，覆盖组件交互与页面流程
  - 方案: [202602161127_testing-unify-codex-playwright](archive/2026-02/202602161127_testing-unify-codex-playwright/)

## [0.10.6] - 2026-02-16

### CI
- **[actions]**: 在 GitHub Actions `e2e-codex` 中运行多窗口并发回归测试（Codex CLI -> Realms -> Real Upstream，`TestCodexE2E_ConcurrentWindows_ProbeDueSSE`）

## [0.10.5] - 2026-02-16

### 测试
- **[e2e]**: 增加 probe_due 场景下的并发 SSE 回归测试（Codex/多窗口并发不再被错误收敛为单连接）

## [0.10.4] - 2026-02-16

### 移除
- **[upstream]**: 移除 `REALMS_UPSTREAM_HTTP_MAX_CONNS_PER_HOST` 配置项与对应实现（不再支持按 host 限制上游并发连接数）

## [0.10.3] - 2026-02-16

### 修复
- **[scheduler]**: 移除 probe claim 单飞限制，允许单 upstream/channel 并发多连接（probe_due 下不再因单飞导致调度失败）
  - 方案: [202602160913_remove-probe-claim-singleflight](archive/2026-02/202602160913_remove-probe-claim-singleflight/)
  - 决策: remove-probe-claim-singleflight#D001(完全移除 probe claim 单飞以满足单上游多连接)

## [0.10.2] - 2026-02-16

### 修复
- **[scheduler]**: probe claim busy 时并发请求不再直接返回“上游不可用”（无可用 Selection 时回退尝试被跳过的 probe_due channel）
  - 方案: [202602160859_fix-v1-probe-claim-concurrency](archive/2026-02/202602160859_fix-v1-probe-claim-concurrency/)
  - 决策: fix-v1-probe-claim-concurrency#D001(两轮选择 + 回退保证并发可用性)
