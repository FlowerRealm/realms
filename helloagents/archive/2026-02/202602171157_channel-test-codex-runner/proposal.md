# 变更提案: channel-test-codex-runner

## 元信息
```yaml
类型: 增强
方案类型: implementation
优先级: P1
状态: ✅完成
创建: 2026-02-17
```

---

## 1. 需求

### 背景
当前管理后台“渠道 → 测试”功能由服务端直接构造 HTTP 请求探测上游（含 SSE 解析/TTFT 统计与 `/v1/responses`→`/v1/chat/completions` 兜底）。该逻辑需要持续维护不同上游的细节差异，并且与项目的“可用性验收口径”不一致：项目 E2E/冒烟统一以 **Codex CLI** 作为客户端发起请求。

你期望将“渠道测试”的执行口径切换为：
- 启动一个 **Docker 常驻容器**，容器内安装 **Codex CLI**；
- 渠道测试通过该容器执行 `codex exec` 来验证可用性（口径与 E2E 一致）。

### 目标
1) 管理后台“测试连接”可选择使用 Codex CLI 作为探测客户端（常驻 Docker 容器）
2) 渠道测试仍保持现有 SSE 进度输出（start/model_start/model_done/summary），前端无需改动协议
3) 将 Codex CLI 执行封装为独立的 runner 服务（可通过 docker-compose 作为 sidecar 常驻）

### 约束条件
```yaml
兼容性: 不要求部署环境安装 codex；通过 runner 容器提供
安全性: runner 不落盘保存上游 key，不在日志打印敏感信息
性能: runner 常驻，避免每次测试重复安装/冷启动
```

### 验收标准
- [√] 新增 codex runner 容器（Dockerfile + 启动方式）
- [√] Realms 支持通过配置将渠道测试委派给 runner
- [√] 未配置 runner 时，渠道测试仍可用（回退为现有 HTTP probe）
- [√] `go test ./...` 通过
- [√] 文档与知识库同步更新（含 CHANGELOG）

---

## 2. 方案

### 技术方案
- 新增一个轻量 **codex-runner**（Node HTTP 服务）：
  - 容器内 `npm install -g @openai/codex`
  - 对外提供 `POST /v1/exec`：接收 `{base_url, api_key, model, prompt, wire_api, organization}`，执行 `codex exec` 并返回 `{ok, latency_ms, output, error}`
- Realms 渠道测试（`GET /api/channel/test/:channel_id`）：
  - 当配置 `REALMS_CHANNEL_TEST_CODEX_RUNNER_URL` 存在时，对 `openai_compatible` 渠道优先走 runner；
  - 其他类型或 runner 不可用时，回退现有 HTTP probe（保持当前行为与单测稳定）。
- 提供 `docker-compose.channel-test.yml` 作为可选叠加文件：
  - 定义 `codex-runner` 服务（常驻）
  - 为 `realms` 服务注入 `REALMS_CHANNEL_TEST_CODEX_RUNNER_URL=http://codex-runner:8787`

### 影响范围
```yaml
后端:
  - router/channels_api_routes.go（渠道测试入口）
  - internal/config/config.go（新增配置项）
  - internal/server/app.go（注入 router options）
  - internal/*（新增 runner client）
容器/部署:
  - tools/codex-runner/*（runner 服务）
  - docker-compose.channel-test.yml（可选 compose 叠加）
文档:
  - docs/USAGE.md（如何启用 runner）
  - .env.example（配置项提示）
  - helloagents/modules/testing.md（测试口径/依赖说明）
  - helloagents/CHANGELOG.md（Unreleased 记录）
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| runner 执行失败导致渠道测试不可用 | 中 | 未配置/不可用时回退现有 probe；返回明确错误信息 |
| runner 处理敏感信息（上游 key）泄露 | 高 | 禁止日志打印 key；响应中不返回 key；限制 output 长度 |
| codex 版本变动导致行为差异 | 中 | runner 容器可固定 @openai/codex 版本（后续按需 pin） |

---

## 3. 核心场景

### 场景: 管理后台渠道测试（启用 runner）
**模块**: admin/channels  
**条件**: `REALMS_CHANNEL_TEST_CODEX_RUNNER_URL` 已配置且 runner 可访问  
**行为**: 点击“测试”→ 后端调用 runner 逐模型执行 `codex exec`  
**结果**: 返回 SSE 进度 + 汇总结果；`last_test_*` 字段更新

### 场景: 管理后台渠道测试（未启用 runner）
**模块**: admin/channels  
**条件**: 未配置 runner 或 runner 不可用  
**行为**: 按当前 HTTP probe 逻辑探测上游  
**结果**: 行为与现状一致

