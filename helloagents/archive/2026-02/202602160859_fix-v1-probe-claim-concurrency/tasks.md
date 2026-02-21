# 任务清单: fix-v1-probe-claim-concurrency

> **@status:** completed | 2026-02-16 09:07

目录: `helloagents/archive/2026-02/202602160859_fix-v1-probe-claim-concurrency/`

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
总任务: 8
已完成: 8
完成率: 100%
```

---

## 任务列表

### 1. 分析与设计

- [√] 1.1 定位并确认根因（probe claim 单飞导致并发无 Selection → `502 上游不可用`）
  - 验证: 代码路径分析（`internal/scheduler/scheduler.go` / `internal/api/openai/handler.go`）

- [√] 1.2 形成修复方案与决策记录（两轮选择 + 回退）
  - 依赖: 1.1

### 2. 开发实施

- [√] 2.1 在 `internal/scheduler/scheduler.go` 增加 probe claim busy 回退选择逻辑
  - 验证: 新增单元测试覆盖并通过

- [√] 2.2 在 `internal/scheduler/scheduler_test.go` 增加回归测试（单 channel + 双 channel 回退）
  - 依赖: 2.1

- [√] 2.3 运行 `go test ./...`
  - 验证: 全部通过

### 3. 文档与归档

- [√] 3.1 同步知识库模块文档（scheduler）
  - 验证: `helloagents/modules/scheduler.md` 已更新

- [√] 3.2 迁移方案包到 `helloagents/archive/` 并更新索引
  - 验证: `migrate_package.py` 成功

- [√] 3.3 更新 `helloagents/CHANGELOG.md`（包含方案包归档链接）
  - 依赖: 3.2

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 2.1 | completed | probe claim busy 时记录 skipped，最终无 Selection 时回退尝试被跳过的 channel |
| 2.2 | completed | 覆盖“单 channel 不阻断”与“其它 channel 不可用时回退”两类关键场景 |
| 3.2 | completed | 已迁移至 `helloagents/archive/2026-02/202602160859_fix-v1-probe-claim-concurrency/` |
| 3.3 | completed | 已在 `helloagents/CHANGELOG.md` 记录版本条目并链接归档方案包 |
