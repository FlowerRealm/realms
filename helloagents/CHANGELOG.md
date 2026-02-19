# 变更日志

## [Unreleased]

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

### 微调
- **[web]**: `/usage` 页面 1:1 对齐 `/admin/usage`（版式 + 文案 + 字段/表头；补齐"消费排行用户"与明细列；用户侧明细列将"渠道"替换为"Key"）
  - 类型: 微调（无方案包）
  - 文件: web/src/pages/UsagePage.tsx, web/src/pages/usage/*, web/src/index.css
- **[web]**: 登录/注册页右上角导航（nav-pills）激活态从默认蓝色改为主题绿色
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

### 增强
- **[admin/channels]**: 渠道"测试连接"改为可选的 CLI Runner 模式（Codex/Claude/Gemini CLI），测试结果仅在前端展示、不写数据库、不影响渠道可用性调度
  - 方案: [202602171513_cli-channel-test](plan/202602171513_cli-channel-test/)
  - 新增: `tools/cli-runner/`（Node.js HTTP runner 容器，支持三种 CLI）
  - 新增: `docker-compose.channel-test.yml`（可选叠加）
  - 配置: `REALMS_CHANNEL_TEST_CLI_RUNNER_URL` 控制功能开关
  - 前端: 测试按钮和健康徽章列根据配置条件显示/隐藏
  - 安全: 批量测试在 CLI 模式下禁用（405）

### 修复
- **[web]**: 全局覆盖 `nav-pills`/`form-switch` 等状态色并调整主题主色为浅绿色，修复 `/admin/settings` tabs/开关仍呈现默认亮蓝色
  - 方案: [202602190938_light-green-theme](archive/2026-02/202602190938_light-green-theme/)
  - 决策: light-green-theme#D001(全局覆盖 `nav-pills` 激活态以避免默认亮蓝色)
- **[e2e]**: 修复 usage Playwright 测试：等待已移除的 `/api/user/models/detail` 改为 `/api/usage/windows`；补回 `rlm-usage-row` / `rlm-usage-detail-row` 类名
  - 类型: 微调（无方案包）
  - 文件: web/e2e/usage.spec.ts, web/src/pages/usage/UsageEventsCard.tsx

### 测试
- **[e2e]**: 增加回归：`upstream_unavailable` 在管理后台展示“最后一次失败原因”，用户侧仍保持“上游不可用”
  - 文件: web/e2e/upstream-unavailable-details.spec.ts, cmd/realms-e2e/main.go
- **[e2e]**: 增加回归：登录页右上角导航（nav-pills）激活态不使用默认亮蓝色，且命中主题主色
  - 文件: web/e2e/theme-colors.spec.ts
- **[e2e]**: `/models` 用例改为校验“所有已配置的可见模型”都能在模型列表页展示（按 `/api/user/models/detail` 返回为准）；`cmd/realms-e2e` 支持通过 `REALMS_E2E_BILLING_MODELS` 一次 seed 多个模型
  - 文件: web/e2e/routes.spec.ts, cmd/realms-e2e/main.go, web/README.md

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
