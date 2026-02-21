# testing

> 测试与 CI 约定（SSOT）

## 1. 目标与原则

- **同口径**：本地与 CI 使用同一套检查集
- **可用性（E2E/冒烟）**：统一以 **Codex CLI** 作为客户端发起请求
- **前端问题**：统一使用 **Playwright**
- **默认稳定**：无 Secrets 时默认 seed/fake upstream；当 `REALMS_CI_*` 配置齐全时自动切换为 real-upstream 集成回归（并由脚本加入 retry）

## 2. 统一入口（推荐）

主入口（seed/fake upstream；若 `REALMS_CI_*` 齐全则自动切换为 real-upstream）：
- `scripts/ci.sh`
- `make ci`（内部调用 `scripts/ci.sh`）
- GitHub Actions：`.github/workflows/ci.yml`

可选真实上游集成回归：
- `scripts/ci-real.sh`
- GitHub Actions：`.github/workflows/ci-real.yml`（`workflow_dispatch`，可选 schedule）

## 3. Codex 可用性（E2E/冒烟）

### 3.1 Go E2E（fake upstream，主线口径）

- 入口：`go test ./tests/e2e -run TestCodexCLI_E2E_FakeUpstream_Cache -count=1`
- 客户端：测试内部使用 `codex exec`（Codex CLI）验证 `/v1/responses` 可用性与 usage_events 落库口径
- 特点：不依赖真实上游 Secrets，可稳定运行

### 3.2 Go E2E（real upstream，可选）

- 入口：`scripts/ci-real.sh`（内部调用 `TestCodexE2E_ConcurrentWindows_ProbeDueSSE` 与 `TestCodexCLI_E2E`）
- 依赖环境变量：`REALMS_CI_UPSTREAM_BASE_URL` / `REALMS_CI_UPSTREAM_API_KEY` / `REALMS_CI_MODEL`
- gating：`TestCodexCLI_E2E` 仅在设置 `REALMS_CI_ENFORCE_E2E=1` 时运行（避免 `go test ./...` 在注入 `REALMS_CI_*` 时隐式触发真实上游）

### 3.3 快速冒烟（推荐）

- `scripts/smoke-codex.sh`
  - 启动 `cmd/realms-e2e`
  - 通过 Codex CLI 发起最小请求验证链路

### 3.4 管理后台渠道测试（CLI Runner）

管理后台「渠道 → 测试连接」功能使用 CLI Runner 容器执行真实 CLI 测试。

**环境变量：** `REALMS_CHANNEL_TEST_CLI_RUNNER_URL`（如 `http://cli-runner:3100`）

**行为：**
- **已配置**：点击测试时委派给 CLI Runner（Codex/Claude/Gemini CLI），通过 SSE 返回结果
- **未配置**：测试按钮始终可见，点击时返回错误提示「CLI runner 未配置，请设置 REALMS_CHANNEL_TEST_CLI_RUNNER_URL」

**渠道类型 → CLI 映射：**
| 渠道类型 | CLI | cli_type |
|---------|-----|----------|
| openai_compatible | Codex CLI | codex |
| anthropic | Claude Code CLI | claude |

**结果隔离（CRITICAL）：**
- CLI 测试结果仅通过 SSE 返回前端，为纯客户端会话状态
- **不调用** `UpdateUpstreamChannelTest`，不写 `last_test_*` 字段
- **不调用** `scheduler.Report()`，不影响 ban/cool/probe/fail_score
- 不影响指针、渠道组路由、调度权重

**使用限制：**
- 仅用户手动点击单个渠道的"测试连接"按钮时触发
- 无批量测试、无定时探测、无后台轮询

**启动 runner（Docker Compose 叠加）：**
```bash
docker compose -f docker-compose.yml -f docker-compose.channel-test.yml up -d --build
```

**Runner 容器：**
- 镜像：`tools/cli-runner/Dockerfile`（基于 node:20-slim，全局安装 @openai/codex、@anthropic-ai/claude-code、@google/gemini-cli）
- API：`POST /v1/test`（执行 CLI 测试）、`GET /healthz`（健康检查）
- 安全：临时 HOME 目录、不落盘保存 key、输出长度限制 1024 字符

**E2E 测试覆盖：**

| 测试 | 类型 | 依赖 | 入口 |
|------|------|------|------|
| `TestCLIChannelTest_E2E` | fake runner（主线口径） | 无外部依赖 | `go test ./tests/e2e -run TestCLIChannelTest_E2E` |
| `TestCLIChannelTest_RealUpstream_E2E` | real runner + real upstream | CLI Runner 容器 + 上游 Secrets | `go test ./tests/e2e -run TestCLIChannelTest_RealUpstream_E2E` |

- Fake runner 测试：验证 SSE 协议（start/model_done/summary）、结果隔离（`last_test_at` 不更新）
- Real upstream 测试：gating `REALMS_CI_ENFORCE_E2E=1` + `REALMS_CI_CLI_RUNNER_URL`；额外依赖 `REALMS_CI_UPSTREAM_BASE_URL` / `REALMS_CI_UPSTREAM_API_KEY` / `REALMS_CI_MODEL`；由 `scripts/ci-real.sh` 在 `REALMS_CI_CLI_RUNNER_URL` 存在时自动执行

## 4. Playwright（组件级/交互级）

### 4.1 默认 seed 模式（主线口径）

- 测试目录：`web/e2e/`
- 配置：`web/playwright.config.ts`
- 默认会启动 `cmd/realms-e2e`：
  - 自动创建临时 SQLite 并 seed 最小数据集
  - Playwright 通过 `global-setup.ts` 获取 root 登录态（storageState）

### 4.2 真实数据 / 真实上游

- 真实数据（外部服务）：`REALMS_E2E_PROFILE=real` + `REALMS_E2E_EXTERNAL_SERVER=1`
- seed + 真实上游（ci-real）：`REALMS_E2E_ENFORCE_REAL_UPSTREAM=1` + 上游 base_url/api_key/model

### 4.3 视觉快照回归（全站页面）

用于“主题/样式改动”后进行**逐页截图**对比，辅助反复修正 UI（默认 seed 数据集）：

- 运行：`npm -C web run test:visual`
- 更新基线：`npm -C web run test:visual:update`
- 用例：`web/e2e/visual-routes.spec.ts`

## 5. 常用命令

```bash
# 默认同口径检查集（本地/CI）
make ci

# 仅后端单测
go test ./...

# 仅 Codex 可用性（fake upstream）
REALMS_CI_ENFORCE_E2E=1 go test ./tests/e2e -run TestCodexCLI_E2E_FakeUpstream_Cache -count=1

# 仅 Codex 可用性（real upstream）
REALMS_CI_ENFORCE_E2E=1 \
  REALMS_CI_UPSTREAM_BASE_URL="https://api.openai.com" \
  REALMS_CI_UPSTREAM_API_KEY="sk-***" \
  REALMS_CI_MODEL="gpt-5.2" \
  go test ./tests/e2e -run TestCodexCLI_E2E -count=1

# 仅 CLI 渠道测试（fake runner，主线口径）
go test ./tests/e2e -run TestCLIChannelTest_E2E -count=1

# 仅 CLI 渠道测试（real runner + real upstream）
REALMS_CI_ENFORCE_E2E=1 \
  REALMS_CI_CLI_RUNNER_URL="http://localhost:3100" \
  REALMS_CI_UPSTREAM_BASE_URL="https://api.openai.com" \
  REALMS_CI_UPSTREAM_API_KEY="sk-***" \
  REALMS_CI_MODEL="gpt-5.2" \
  go test ./tests/e2e -run TestCLIChannelTest_RealUpstream_E2E -count=1 -timeout=120s

# 仅 Playwright（seed）
npm --prefix web ci
npm --prefix web run build
npm --prefix web run test:e2e:ci
```
