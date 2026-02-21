# 任务清单: rollback-perf-security-since-093

> **@status:** completed | 2026-02-16 16:03

目录: `helloagents/archive/2026-02/202602161558_rollback-perf-security-since-093/`

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

### 1. 回滚准备
- [√] 1.1 确认 tag `0.9.3` 存在并列出 `0.9.3..HEAD` 提交
- [√] 1.2 识别“性能/安全相关”提交并确定回滚列表（按提交信息 + 变更范围）

### 2. 开发实施
- [√] 2.1 新建分支 `rollback/0.9.3-usability`
- [√] 2.2 依次 revert 目标提交（自新到旧）：`59106f2`、`d9ff862`、`30d65b2`、`b46db9b`、`75273a1`、`7fb13cf`
- [√] 2.3 运行 `go test ./...`

### 3. 文档与归档
- [√] 3.1 更新 `helloagents/CHANGELOG.md`（Unreleased 回滚条目）
- [√] 3.2 更新 `helloagents/modules/scheduler.md`（probe claim 单飞行为）
- [√] 3.3 新增归档方案包并更新索引（`helloagents/archive/_index.md`、`helloagents/INDEX.md`）

---

## 执行备注

| 任务 | 状态 | 备注 |
|------|------|------|
| 2.2 | completed | 回滚均无冲突；回滚提交在同一分支上线性排列，便于 merge/cherry-pick |
| 2.3 | completed | `go test ./...` 通过 |
