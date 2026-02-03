# 任务清单: e2e_usage_metrics

目录: `helloagents/plan/202601281234_e2e-usage-metrics/`

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
总任务: 4
已完成: 4
完成率: 100%
```

---

## 任务列表

### 1. E2E 测试增强

- [√] 1.1 调整 Codex prompt 为“生成最小 Go 程序”，并增强输出断言
  - 文件: `tests/e2e/codex_cli_test.go`

- [√] 1.2 增加 fake upstream 与用量事件断言：请求数（2 次）、Token 数、第二次缓存 Token 命中与口径检查
  - 文件: `tests/e2e/codex_cli_test.go`
  - 备注: 真实上游用例与 fake upstream 用例均执行两次请求，并要求第二次缓存 Token 命中（`cached_input_tokens>0`）

### 2. 知识库同步

- [√] 2.1 更新模块文档：补齐 E2E 的 usage_events 断言说明
  - 文件: `helloagents/modules/ci_github_actions.md`

- [√] 2.2 更新 CHANGELOG 记录本次 CI/E2E 变更
  - 文件: `helloagents/CHANGELOG.md`
