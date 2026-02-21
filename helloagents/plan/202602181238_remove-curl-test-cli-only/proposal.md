# 变更提案: remove-curl-test-cli-only

## 元信息
```yaml
类型: 重构
方案类型: implementation
优先级: P1
状态: 草稿
创建: 2026-02-18
```

---

## 1. 需求

### 背景
当前渠道测试功能存在两套逻辑：
1. **旧 curl/HTTP probe 逻辑**：后端直接构造 HTTP 请求探测上游 API（`probeUpstream` → `probeSingleModel`），结果会写入数据库（`UpdateUpstreamChannelTest`）
2. **新 CLI runner 逻辑**：委派给外部 CLI runner 容器（Codex/Claude/Gemini CLI），结果纯 SSE 返回前端，不写数据库

两套逻辑通过 `ChannelTestCLIRunnerURL` 配置项切换：配置了 runner URL 时走 CLI 路径，否则回退到旧 curl 逻辑。前端通过 `cli_test_available` 字段控制测试按钮的可见性。

用户希望完全采用 CLI runner 逻辑，移除旧 curl 测试代码，测试功能始终可用（不再通过配置开关控制可见性）。

### 目标
1. **移除旧 curl 测试逻辑**：删除 `probeUpstream`、`probeSingleModel`、`buildChannelProbePlan`、`parseProbeSSE`、`extractSSETextChunk`、`summarizeChannelProbeResults`、`resolveChannelTestModels`、`testChannelOnceDetailed`、`testChannelOnce`、`streamChannelTestHandler`、`wantProbeStream`、`buildUpstreamURL` 等函数及相关类型
2. **移除批量测试**：删除 `testAllChannelsHandler`，移除 `GET /channel/test` 路由
3. **移除可选开关**：删除 `channelsPageResponse.CLITestAvailable` 字段，删除前端 `cli_test_available` 类型定义
4. **CLI runner 始终为唯一测试方式**：`testChannelHandler` 始终委派给 `streamChannelCLITestHandler`，无回退
5. **未配置 runner URL 时给出明确错误**：runner URL 为空时测试返回错误提示而非隐藏按钮
6. **清理相关测试文件**：移除旧 probe 测试（`channels_api_routes_probe_test.go`）

### 约束条件
```yaml
隔离性: CLI 测试结果仍然不影响 scheduler 状态（不写数据库、不调 UpdateUpstreamChannelTest）
兼容性: ChannelTestCLIRunnerURL 配置项保留（runner 仍需要知道服务地址），但不再作为功能开关
部署: cli-runner 容器仍为可选 sidecar，未部署时测试会返回错误提示
```

### 验收标准
- [ ] 旧 curl/HTTP probe 相关函数及类型全部移除
- [ ] `testAllChannelsHandler` 及 `GET /channel/test` 批量路由移除
- [ ] `channelsPageResponse` 中 `CLITestAvailable` 字段移除
- [ ] 前端 `cli_test_available` 类型定义移除
- [ ] `testChannelHandler` 始终走 CLI runner 路径
- [ ] runner URL 为空时返回明确错误（不隐藏按钮）
- [ ] `channels_api_routes_probe_test.go` 删除
- [ ] CLI runner 相关测试更新适配
- [ ] `go test ./...` 通过
- [ ] 知识库同步

---

## 2. 方案

### 技术方案

**后端（router/channels_api_routes.go）：**

1. `testChannelHandler` 移除条件分支，始终调用 `streamChannelCLITestHandler`：
   - 移除 `if opts.ChannelTestCLIRunnerURL != ""` 分支
   - 移除 `wantProbeStream` / `streamChannelTestHandler` / `testChannelOnceDetailed` 回退路径
   - 在 `streamChannelCLITestHandler` 开头增加 runner URL 为空时的错误处理

2. 移除批量测试路由和处理函数：
   - 删除 `r.GET("/channel/test", admin, testAllChannelsHandler(opts))` 路由注册
   - 删除 `testAllChannelsHandler` 函数
   - 删除 `channelTestResult` 类型

3. 移除旧 probe 相关函数和类型：
   - `testChannelOnce`、`testChannelOnceDetailed`
   - `probeUpstream`、`buildChannelProbePlan`、`probeSingleModel`
   - `parseProbeSSE`、`extractSSETextChunk`、`stringFromAny`
   - `summarizeChannelProbeResults`、`resolveChannelTestModels`
   - `wantProbeStream`、`streamChannelTestHandler`、`buildUpstreamURL`
   - `channelProbePlan`、`channelProbeAttempt` 类型

4. 移除 `channelsPageResponse.CLITestAvailable` 字段及赋值逻辑

**前端（web/src/）：**

5. `web/src/api/channels.ts`：移除 `ChannelsPageData` 中的 `cli_test_available` 字段
6. `web/src/pages/admin/ChannelsPage.tsx`：如有 `cli_test_available` 条件渲染逻辑则移除

**测试：**

7. 删除 `router/channels_api_routes_probe_test.go`（旧 probe 测试）
8. 更新 `router/channels_api_routes_cli_test.go`：
   - `TestChannelsPage_CLITestAvailable` → 移除或改为验证无 `cli_test_available` 字段
   - `TestTestAllChannelsHandler_DisabledInCLIMode` → 删除（路由已移除）
9. 更新 `tests/e2e/cli_channel_test_e2e_test.go`：移除对 `cli_test_available` 的断言和批量测试 405 断言

### 影响范围
```yaml
涉及模块:
  - router/channels_api_routes.go: 移除旧测试函数、类型、路由，简化 testChannelHandler
  - router/options.go: 无变更（ChannelTestCLIRunnerURL 保留）
  - internal/config/config.go: 无变更（配置项保留）
  - web/src/api/channels.ts: 移除 cli_test_available 字段
  - web/src/pages/admin/ChannelsPage.tsx: 移除条件渲染（如有）
  - router/channels_api_routes_probe_test.go: 整文件删除
  - router/channels_api_routes_cli_test.go: 移除相关测试用例
  - tests/e2e/cli_channel_test_e2e_test.go: 移除相关断言
预计变更文件: 6-7
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 未部署 CLI runner 时测试功能不可用 | 低 | 测试时返回明确错误提示"CLI runner 未配置" |
| 移除旧 probe 后遗漏引用导致编译失败 | 低 | 删除后执行 `go build ./...` 验证 |
| 移除 `bufio` 等 import 后可能导致 unused import | 低 | 编译时自动检测 |

---

## 3. 技术设计

### testChannelHandler 重构

重构前：
```go
func testChannelHandler(opts Options) gin.HandlerFunc {
    return func(c *gin.Context) {
        // ... 参数校验 ...
        if opts.ChannelTestCLIRunnerURL != "" {
            streamChannelCLITestHandler(c, opts, channelID)
            return
        }
        if wantProbeStream(c) {
            streamChannelTestHandler(c, opts.Store, channelID)
            return
        }
        ok, latency, msg, probe := testChannelOnceDetailed(...)
        c.JSON(...)
    }
}
```

重构后：
```go
func testChannelHandler(opts Options) gin.HandlerFunc {
    return func(c *gin.Context) {
        // ... 参数校验 ...
        streamChannelCLITestHandler(c, opts, channelID)
    }
}
```

### streamChannelCLITestHandler 增加 runner URL 校验

```go
func streamChannelCLITestHandler(c *gin.Context, opts Options, channelID int64) {
    if opts.ChannelTestCLIRunnerURL == "" {
        c.JSON(http.StatusOK, gin.H{"success": false, "message": "CLI runner 未配置，请设置 REALMS_CHANNEL_TEST_CLI_RUNNER_URL"})
        return
    }
    // ... 现有逻辑 ...
}
```

### channelsPageResponse 变更

移除 `CLITestAvailable` 字段：
```go
type channelsPageResponse struct {
    AdminTimeZone string                   `json:"admin_time_zone"`
    Start         string                   `json:"start"`
    End           string                   `json:"end"`
    Overview      channelUsageOverviewView `json:"overview"`
    Channels      []channelAdminListItem   `json:"channels"`
}
```

---

## 4. 核心场景

### 场景: 渠道测试（CLI runner 已配置）
**模块**: admin/channels
**条件**: `REALMS_CHANNEL_TEST_CLI_RUNNER_URL` 已配置且 runner 可访问
**行为**: 点击"测试连接" → 后端调用 runner → SSE 返回结果
**结果**: 测试结果仅在前端展示，不写数据库

### 场景: 渠道测试（CLI runner 未配置）
**模块**: admin/channels
**条件**: 未配置 `REALMS_CHANNEL_TEST_CLI_RUNNER_URL`
**行为**: 点击"测试连接" → 后端返回 `{"success": false, "message": "CLI runner 未配置..."}`
**结果**: 前端显示错误提示

### 场景: 渠道测试（CLI runner 不可达）
**模块**: admin/channels
**条件**: 已配置但 runner 服务不可达
**行为**: 点击"测试连接" → SSE 返回 summary 事件包含错误信息
**结果**: 前端显示"CLI runner 不可达"

---

## 5. 技术决策

### remove-curl-test-cli-only#D001: runner URL 未配置时的行为
**日期**: 2026-02-18
**状态**: ✅采纳
**背景**: 移除旧 probe 回退后，runner URL 未配置时需要决定行为
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 返回 JSON 错误 | 简单明确，用户知道原因 | 需要前端处理非 SSE 响应 |
| B: 返回 SSE 错误事件 | 与正常流程格式一致 | runner 未配置属于配置错误，不应进入 SSE 流 |
**决策**: 选择方案 A（返回 JSON 错误）
**理由**: runner 未配置是部署层面的配置缺失，返回 JSON 错误更直接；前端已有对非成功响应的 catch 处理
**影响**: `streamChannelCLITestHandler` 增加前置校验
