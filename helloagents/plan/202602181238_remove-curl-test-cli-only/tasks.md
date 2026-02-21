# 任务清单: remove-curl-test-cli-only

目录: `helloagents/plan/202602181238_remove-curl-test-cli-only/`

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
总任务: 10
已完成: 10
完成率: 100%
```

---

## 任务列表

### 1. 后端：移除旧测试逻辑

- [√] 1.1 在 `router/channels_api_routes.go` 中移除旧 curl/HTTP probe 函数
  - 删除函数: `testChannelOnce`, `testChannelOnceDetailed`, `probeUpstream`, `buildChannelProbePlan`, `probeSingleModel`, `parseProbeSSE`, `extractSSETextChunk`, `stringFromAny`, `summarizeChannelProbeResults`, `resolveChannelTestModels`, `wantProbeStream`, `streamChannelTestHandler`, `buildUpstreamURL`
  - 删除类型: `channelProbePlan`, `channelProbeAttempt`, `channelTestResult`
  - 清理未使用的 import（`bufio`, `io`, `net/url`）
  - 验证: `go build ./router/...` ✅

- [√] 1.2 在 `router/channels_api_routes.go` 中移除批量测试路由和函数
  - 删除路由注册: `r.GET("/channel/test", admin, testAllChannelsHandler(opts))`
  - 删除函数: `testAllChannelsHandler`
  - 验证: `go build ./router/...` ✅

- [√] 1.3 简化 `testChannelHandler`，移除条件分支
  - 移除 `if opts.ChannelTestCLIRunnerURL != ""` 条件判断
  - 移除 `wantProbeStream` 和 `testChannelOnceDetailed` 回退路径
  - 始终调用 `streamChannelCLITestHandler`
  - 验证: `go build ./router/...` ✅

- [√] 1.4 在 `streamChannelCLITestHandler` 开头增加 runner URL 为空校验
  - runner URL 为空时返回 JSON 错误: `{"success": false, "message": "CLI runner 未配置，请设置 REALMS_CHANNEL_TEST_CLI_RUNNER_URL"}`
  - 验证: `go build ./router/...` ✅

### 2. 后端：移除可选开关

- [√] 2.1 移除 `channelsPageResponse.CLITestAvailable` 字段
  - 删除 `channelsPageResponse` 中的 `CLITestAvailable bool` 字段
  - 删除 `channelsPageHandler` 中的 `CLITestAvailable: opts.ChannelTestCLIRunnerURL != ""` 赋值
  - 验证: `go build ./router/...` ✅

### 3. 前端适配

- [√] 3.1 在 `web/src/api/channels.ts` 中移除 `cli_test_available` 字段
  - 从 `ChannelsPageResponse` 类型定义中删除 `cli_test_available: boolean`

- [√] 3.2 在 `web/src/pages/admin/ChannelsPage.tsx` 中移除 `cli_test_available` 相关条件逻辑（如有）
  - 确认无残留引用（前次提交 8dbd5df 已移除条件渲染）

### 4. 测试清理与更新

- [√] 4.1 删除 `router/channels_api_routes_probe_test.go`（整文件）

- [√] 4.2 更新 `router/channels_api_routes_cli_test.go`
  - 移除 `TestChannelsPage_CLITestAvailable` 测试
  - 移除 `TestTestAllChannelsHandler_DisabledInCLIMode` 测试
  - 新增 `TestCLITestRunnerURLEmpty` 测试（验证 runner URL 为空时返回错误）
  - 从已删除的 probe test 迁移 `openTestStore` 和 `createOpenAIChannelWithCredential` 辅助函数
  - 验证: `go test ./router/...` ✅

- [√] 4.3 更新 `tests/e2e/cli_channel_test_e2e_test.go`
  - 移除对 `cli_test_available` 字段的断言（Test 1: channel_page_cli_test_available）
  - 移除批量测试 405 断言（Test 4: batch_test_disabled）
  - 更新测试编号和函数注释
  - 验证: `go build ./tests/e2e/...` ✅

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 1.1 | completed | 移除了 15 个函数、3 个类型，清理了 `bufio`/`io`/`net/url` 三个 import |
| 1.2 | completed | 移除批量测试路由和 `testAllChannelsHandler` |
| 1.3 | completed | `testChannelHandler` 简化为始终调用 `streamChannelCLITestHandler` |
| 1.4 | completed | 增加 runner URL 为空校验，返回明确错误信息 |
| 2.1 | completed | 移除 `CLITestAvailable` 字段及赋值 |
| 3.1 | completed | 移除前端 `cli_test_available` 类型定义 |
| 3.2 | completed | ChannelsPage.tsx 无残留引用（前次提交已清理） |
| 4.1 | completed | 删除整个 probe test 文件 |
| 4.2 | completed | 迁移辅助函数，新增 runner URL 为空测试，移除 2 个过时测试 |
| 4.3 | completed | 移除 2 个过时子测试（cli_test_available 和 batch_test_disabled） |
