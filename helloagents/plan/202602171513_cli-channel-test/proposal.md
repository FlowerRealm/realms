# 变更提案: cli-channel-test

## 元信息
```yaml
类型: 增强
方案类型: implementation
优先级: P1
状态: 草稿
创建: 2026-02-17
```

---

## 1. 需求

### 背景
当前管理后台"渠道 → 测试连接"使用后端内置 HTTP probe（`probeUpstream`）直连上游逐模型探测。该逻辑需持续维护不同上游的请求构造细节，且与真正的端到端客户端体验存在差距。

此前已有 Codex runner 提案（202602171157_channel-test-codex-runner），但仅覆盖 Codex CLI / `openai_compatible` 渠道，且尚未提交代码。

用户希望：
1. 使用 **Codex CLI**、**Claude Code CLI**、**Gemini CLI** 三种客户端工具替代现有 HTTP probe
2. **最小化测试**：仅在用户手动点击时触发，不自动运行
3. **纯信息性**：测试结果不影响指针（pointer）和渠道可用性（scheduler 状态）
4. 渠道可用性完全由真实用户请求结果（`scheduler.Report`）判定
5. CLI 运行在 Docker 持久化容器中

### 目标
1. 新增统一 CLI runner 容器（Codex CLI + Claude Code CLI + Gemini CLI），持久化运行
2. 管理后台"测试连接"按钮调用 runner 执行对应 CLI，结果仅用于展示
3. 不调用 `UpdateUpstreamChannelTest`，不更新 `last_test_*` 字段
4. **测试功能可选：** 未配置 runner 时，前端隐藏测试按钮（不回退 HTTP probe）
5. 部署者通过是否启动 runner 容器 + 是否设置 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL` 来控制测试功能开关

### 约束条件
```yaml
安全性: runner 不落盘保存上游 key，不在日志打印敏感信息
隔离性: CLI 测试结果不影响 scheduler 状态、指针、可用性判定
部署: runner 为可选 sidecar，通过 docker-compose 叠加启用
最小化: 每个渠道仅测试一个模型，简短 prompt，快速返回
```

### 验收标准
- [ ] 新增 CLI runner 容器（Dockerfile + 启动方式）
- [ ] runner 支持 Codex / Claude / Gemini 三种 CLI
- [ ] 后端支持将渠道测试委派给 runner，自动匹配 channel type → CLI
- [ ] 测试结果纯客户端状态（不写数据库、不调 UpdateUpstreamChannelTest、不触发 scheduler.Report）
- [ ] 禁止自动/批量测试（testAllChannelsHandler 在 CLI 模式下禁用）
- [ ] 后端在 channel page 响应中返回 `cli_test_available` 标记
- [ ] 前端根据 `cli_test_available` 控制测试按钮显示/隐藏
- [ ] 未配置 runner 时，前端不显示测试按钮
- [ ] `go test ./...` 通过
- [ ] 文档与知识库同步

---

## 2. 方案

### 技术方案

**统一多 CLI 容器：** 单个 Docker 容器安装三种 CLI（Codex CLI、Claude Code CLI、Gemini CLI），内置轻量 Node.js HTTP 服务，提供统一 `/v1/test` 端点。后端根据渠道类型自动选择对应 CLI。

**渠道类型 → CLI 映射：**

| 渠道类型 | CLI 工具 | 连接方式 |
|---------|---------|---------|
| `openai_compatible` | Codex CLI (`codex exec`) | 直连上游 base_url |
| `anthropic` | Claude Code CLI (`claude -p`) | 直连上游 base_url |
| `openai_compatible`（Gemini 上游） | Gemini CLI (`gemini`) | 直连上游 base_url |
| `codex_oauth` | 不支持测试 | - |

**最小化策略：**
- 仅测试第一个绑定模型（而非全部模型）
- 固定 prompt: `Reply with exactly: OK`
- 超时 30s

**结果隔离（CRITICAL）：**
- CLI 测试结果仅通过 SSE 流式返回前端，为纯客户端会话状态，不写入数据库
- 不调用 `UpdateUpstreamChannelTest`，不更新 `last_test_*` 字段
- 不调用 `scheduler.Report()`，不影响 ban/cool/probe/fail_score
- 不影响指针（pointer）、渠道组路由、调度权重

**禁止自动测试：**
- 仅用户手动点击单个渠道的"测试连接"按钮时触发
- 无定时探测、无批量测试、无后台轮询
- `testAllChannelsHandler`（批量测试）在 `cli_test_available=true` 时禁用或移除，避免批量 CLI 调用

**测试功能开关：**
- 后端在 channel page 响应中增加 `cli_test_available: bool` 字段（当 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL` 已配置时为 true）
- 前端根据 `cli_test_available` 控制测试按钮和健康徽章列的显示/隐藏
- 未配置 runner → 前端完全隐藏测试按钮，不回退 HTTP probe
- 部署者通过 docker-compose 叠加启动 runner 容器，并设置环境变量来启用测试

### 影响范围
```yaml
后端:
  - router/channels_api_routes.go（渠道测试入口，增加 runner 委派 + cli_test_available 标记）
  - router/options.go（Options 增加 ChannelTestCLIRunnerURL 字段）
  - internal/config/config.go（新增 REALMS_CHANNEL_TEST_CLI_RUNNER_URL）
  - internal/server/app.go（注入 router options）
前端:
  - web/src/api/channels.ts（ChannelsPageResponse 增加 cli_test_available）
  - web/src/pages/admin/ChannelsPage.tsx（测试按钮和健康徽章列的条件渲染）
容器/部署:
  - tools/cli-runner/（runner 服务 + Dockerfile）
  - docker-compose.channel-test.yml（可选 compose 叠加）
文档:
  - helloagents/modules/testing.md
  - helloagents/CHANGELOG.md
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| runner 不可用导致测试失败 | 低 | 前端显示明确错误信息 |
| CLI 版本变动导致命令语法变化 | 中 | runner 容器可固定 CLI 版本 |
| 敏感信息（API key）泄露 | 高 | 禁止日志打印 key；响应中不返回 key；限制输出长度 |
| Gemini CLI 包名/命令语法不确定 | 中 | 实现时确认最新 CLI 包名和用法 |

---

## 3. 技术设计

### Runner API 设计

#### POST /v1/test
- **请求**:
```json
{
  "cli_type": "codex | claude | gemini",
  "base_url": "https://api.openai.com",
  "api_key": "sk-...",
  "model": "gpt-5",
  "prompt": "Reply with exactly: OK",
  "timeout_seconds": 30
}
```
- **响应**:
```json
{
  "ok": true,
  "latency_ms": 234,
  "output": "OK",
  "error": ""
}
```

#### GET /healthz
- **响应**: `{"status": "ok", "cli": {"codex": true, "claude": true, "gemini": true}}`

### Runner 内部实现

每个 CLI 的执行方式：

```yaml
codex:
  命令: codex exec --skip-git-repo-check "{prompt}"
  环境变量: OPENAI_API_KEY, HOME(临时)
  配置: ~/.codex/config.toml (model_provider.base_url)

claude:
  命令: claude -p "{prompt}" --output-format text --model {model}
  环境变量: ANTHROPIC_API_KEY 或 OPENAI_API_KEY
  配置: 通过 --api-base 参数或环境变量指定 base_url

gemini:
  命令: gemini -p "{prompt}" --model {model}
  环境变量: GEMINI_API_KEY 或对应环境变量
  配置: 通过环境变量或参数指定 base_url
```

---

## 4. 核心场景

### 场景: 管理后台渠道测试（CLI runner 可用）
**模块**: admin/channels
**条件**: `REALMS_CHANNEL_TEST_CLI_RUNNER_URL` 已配置且 runner 可访问
**行为**: 点击"测试连接" → 后端调用 runner（自动选择 CLI 类型） → 返回 SSE 进度 + 结果
**结果**: 测试结果仅在前端展示，不更新 `last_test_*` 字段，不影响 scheduler

### 场景: 管理后台渠道测试（runner 未配置）
**模块**: admin/channels
**条件**: 未配置 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL`
**行为**: channel page 返回 `cli_test_available: false`；前端隐藏测试按钮
**结果**: 测试功能完全不可见，不回退 HTTP probe

### 场景: 渠道可用性判定
**模块**: scheduler
**条件**: 真实用户请求到达
**行为**: 通过 `scheduler.Report()` 记录成功/失败，触发冷却/封禁/探测
**结果**: 渠道可用性完全由真实请求结果决定，与 CLI 测试无关

---

## 5. 技术决策

### cli-channel-test#D001: 统一容器 vs 独立容器
**日期**: 2026-02-17
**状态**: ✅采纳
**背景**: 需要运行三种 CLI（Codex/Claude/Gemini），需决定容器部署架构
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 统一容器（单容器三 CLI） | 部署简单、统一 API、单一配置 | 镜像较大、CLI 共享资源 |
| B: 独立容器（每 CLI 一个） | 完全隔离、独立扩展 | 需管理三个容器、三个配置 URL |
**决策**: 选择方案 A（统一容器）
**理由**: 用户要求"最小化"，测试频率低（手动触发），资源竞争风险小；单一容器大幅简化部署
**影响**: docker-compose.channel-test.yml 仅定义一个 cli-runner 服务

### cli-channel-test#D002: CLI 测试结果不更新 last_test_* 字段
**日期**: 2026-02-17
**状态**: ✅采纳
**背景**: 用户要求测试结果"不影响实际的指针，不影响渠道可用性"
**决策**: CLI 测试路径中不调用 `UpdateUpstreamChannelTest`，结果仅通过 SSE 返回前端展示
**理由**: `last_test_*` 字段在 UI 中显示为健康徽章，CLI 测试是按需的人工验证，不应覆盖该状态
**影响**: 健康徽章仅反映 HTTP probe 结果（回退模式）或保持"未测试"状态
