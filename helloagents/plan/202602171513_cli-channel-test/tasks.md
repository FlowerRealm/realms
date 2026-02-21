# 任务清单: cli-channel-test

目录: `helloagents/plan/202602171513_cli-channel-test/`

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
总任务: 13
已完成: 13
完成率: 100%
```

---

## 任务列表

### 1. CLI Runner 容器

- [√] 1.1 新增 `tools/cli-runner/server.js`（Node.js HTTP 服务，`POST /v1/test` + `GET /healthz`）
  - 支持 codex / claude / gemini 三种 cli_type
  - 每种 CLI 创建临时 HOME 目录、写入配置、设置环境变量、执行命令、解析输出
  - 安全：不落盘保存 key，不在日志打印 key，限制输出长度

- [√] 1.2 新增 `tools/cli-runner/package.json`（依赖声明）

- [√] 1.3 新增 `tools/cli-runner/Dockerfile`（基于 node:20-slim，安装三种 CLI）

- [√] 1.4 新增 `docker-compose.channel-test.yml`（可选叠加，定义 cli-runner 服务 + 为 realms 注入 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL`）

### 2. 后端集成

- [√] 2.1 在 `internal/config/config.go` 中新增 `ChannelTestCLIRunnerURL` 配置项（环境变量 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL`）

- [√] 2.2 在 `router/options.go` 的 `Options` 结构体中增加 `ChannelTestCLIRunnerURL string` 字段；在 `internal/server/app.go` 中将配置注入

- [√] 2.3 在 `router/channels_api_routes.go` 中：
  - channel page 响应增加 `cli_test_available: bool` 字段（`ChannelTestCLIRunnerURL != ""`）
  - 实现 CLI runner 委派逻辑：根据 channel type 选择 cli_type，调用 runner `/v1/test`
  - 最小化：仅取第一个绑定模型
  - 结果隔离：不调用 `UpdateUpstreamChannelTest`，不写数据库，纯 SSE 返回
  - `testAllChannelsHandler` 在 CLI 模式下（runner URL 已配置）返回 405 或禁用
  - runner 配置但不可达时：返回明确错误信息
  - 保持 SSE 进度协议不变（start → model_done → summary）

- [√] 2.4 补齐单测（fake runner server 覆盖委派逻辑 + cli_test_available 标记）

### 3. 前端适配

- [√] 3.1 在 `web/src/api/channels.ts` 中为 `ChannelsPageResponse` 增加 `cli_test_available: boolean` 字段

- [√] 3.2 在 `web/src/pages/admin/ChannelsPage.tsx` 中：
  - 根据 `cli_test_available` 条件渲染测试按钮（false 时隐藏）
  - 根据 `cli_test_available` 条件渲染健康徽章列（false 时隐藏或显示"-"）

### 4. 验证与文档

- [√] 4.1 运行 `go test ./...` 确认通过

- [√] 4.2 更新 `helloagents/modules/testing.md`（CLI runner 章节、测试功能开关说明）

- [√] 4.3 更新 `helloagents/CHANGELOG.md`（Unreleased 记录）

---

## 执行备注

| 任务 | 状态 | 备注 |
|------|------|------|
| 1.3 | completed | Gemini CLI 包名确认为 `@google/gemini-cli` |
| 2.3 | completed | Gemini 暂无独立 channel type，当前仅映射 openai_compatible→codex 和 anthropic→claude |
| 3.2 | completed | 测试按钮和健康徽章列均受 cli_test_available 控制；未配置时整列隐藏，表格 colSpan 动态调整 |
