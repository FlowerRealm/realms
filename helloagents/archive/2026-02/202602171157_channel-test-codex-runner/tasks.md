# 任务清单: channel-test-codex-runner

> **@status:** completed | 2026-02-17 12:09

目录: `helloagents/archive/2026-02/202602171157_channel-test-codex-runner/`

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
总任务: 9
已完成: 9
完成率: 100%
```

---

## 任务列表

### 1. 现状梳理与设计收敛

- [√] 1.1 梳理渠道测试现状（router + web SSE 协议）
- [√] 1.2 明确 runner API（请求/响应字段、超时、输出截断）

### 2. Codex runner（常驻容器）

- [√] 2.1 新增 `tools/codex-runner`（Node HTTP 服务 + Dockerfile）
- [√] 2.2 新增 `docker-compose.channel-test.yml`（叠加启动 runner）

### 3. 后端集成（渠道测试委派）

- [√] 3.1 新增 runner client（Go）
- [√] 3.2 配置项接入（config + server 注入 router options）
- [√] 3.3 渠道测试 handler 接入 runner（openai_compatible 优先）
- [√] 3.4 补齐单测（使用 fake runner 覆盖委派逻辑）

### 4. 验证与文档同步

- [√] 4.1 本地运行 `go test ./...` 通过
- [√] 4.2 更新知识库与 CHANGELOG（Unreleased）
- [√] 4.3 归档方案包并更新归档索引

---

## 执行备注

| 任务 | 状态 | 备注 |
|------|------|------|
| 3.3 | completed | runner 仅对 `openai_compatible` 渠道启用；其他类型保持原 probe |

