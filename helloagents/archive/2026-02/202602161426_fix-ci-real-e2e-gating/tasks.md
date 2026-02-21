# 任务清单: fix-ci-real-e2e-gating

> **@status:** completed | 2026-02-16 14:31

目录: `helloagents/archive/2026-02/202602161426_fix-ci-real-e2e-gating/`

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
总任务: 5
已完成: 5
完成率: 100%
```

---

## 任务列表

### 1. 定位问题

- [√] 1.1 查看 CI 失败日志，确认 `go test ./...` 隐式触发 `TestCodexCLI_E2E` 的根因

### 2. 修复与稳定性

- [√] 2.1 为 `TestCodexCLI_E2E` 增加 `REALMS_CI_ENFORCE_E2E` gating（未开启则跳过）
- [√] 2.2 本地验证：`go test ./...` + `REALMS_CI_ENFORCE_E2E=1 go test ./tests/e2e -run TestCodexCLI_E2E_FakeUpstream_Cache -count=1`

### 3. 文档与知识库同步

- [√] 3.1 更新 `helloagents/modules/testing.md`（真实上游用例执行口径与 gating 说明）
- [√] 3.2 更新 `helloagents/CHANGELOG.md`，归档方案包并更新 `helloagents/archive/_index.md`
