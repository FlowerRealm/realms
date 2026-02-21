# 任务清单: remove-probe-claim-singleflight

> **@status:** completed | 2026-02-16 09:23

目录: `helloagents/archive/2026-02/202602160913_remove-probe-claim-singleflight/`

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
总任务: 10
已完成: 10
完成率: 100%
```

---

## 任务列表

### 1. 分析与方案

 - [√] 1.1 梳理 probe claim 单飞相关代码引用点与删除清单
  - 验证: `rg TryClaimChannelProbe` 无残留引用

- [√] 1.2 完成方案包（proposal/tasks）并通过 `validate_package.py`
  - 依赖: 1.1

### 2. 开发实施

- [√] 2.1 删除 `State` 的 probe claim 状态与方法（`channelProbeClaimUntil` / `TryClaimChannelProbe` / `ReleaseChannelProbeClaim`）
  - 验证: 编译通过；无未使用字段/方法

- [√] 2.2 调整 `Scheduler.SelectWithConstraints` 移除 probe claim 单飞逻辑（含 binding 命中路径）
  - 依赖: 2.1

- [√] 2.3 更新 `runtime_stats` 等运行态相关逻辑，移除 probe claim 清理分支
  - 依赖: 2.1

- [√] 2.4 更新/新增单元测试覆盖新行为（probe_due 下多次选择不再单飞跳过）
  - 依赖: 2.2

- [√] 2.5 运行 `go test ./...`
  - 验证: 全部通过

### 3. 文档与归档

- [√] 3.1 同步知识库模块文档（scheduler/openai-api 如涉及）
  - 验证: 文档描述与代码行为一致

- [√] 3.2 迁移方案包到 `helloagents/archive/` 并更新索引
  - 验证: `migrate_package.py` 成功

- [√] 3.3 更新 `helloagents/CHANGELOG.md`（包含归档方案包链接与决策引用）
  - 依赖: 3.2

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 1.1 | completed | 已确认 probe claim 相关引用点仅在 `internal/scheduler/*` 与测试中 |
| 2.1 | completed | 删除 probe claim 状态与方法，保留 `probe_due` 语义（`channelProbeDueAt`） |
| 2.2 | completed | `SelectWithConstraints` 不再做 probe claim 单飞限制（binding/选择路径均移除） |
| 2.4 | completed | 更新测试：probe_due 下多次选择仍会命中 probe channel（无单飞跳过） |
| 2.5 | completed | `go test ./...` 通过 |
| 3.3 | completed | CHANGELOG 新增 `0.10.3` 条目并链接本归档方案包 |
