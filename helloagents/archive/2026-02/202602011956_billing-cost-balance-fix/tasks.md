# 任务清单: billing-cost-balance-fix

目录: `helloagents/plan/202602011956_billing-cost-balance-fix/`

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

### 1. 后端：计费策略与扣费链路

- [√] 1.1 修复 quota provider 选择：默认使用 `HybridProvider`；当 `self_mode` 或 `feature_disable_billing` 时使用 `FreeProvider`
  - 位置: `internal/server/app.go`
  - 验证: `internal/server/app_test.go` 覆盖默认/禁用计费/self_mode 三种行为

- [√] 1.2 PayG 结算支持“补扣差额”：`actual_cost_usd > reserved_usd` 时，余额充足则追加扣减差额；余额不足则最多扣到 0
  - 位置: `internal/store/usage_balance.go`
  - 验证: `internal/store/usage_balance_test.go`（SQLite）覆盖：补扣/返还/扣到 0

- [√] 1.3 校验计费口径只依赖 tokens：确认不存在读取上游“费用/金额”字段的路径，并补测试防回归
  - 位置: `internal/api/openai/handler.go`（usage tokens 提取与 quota.Commit 传参）
  - 验证: handler 层测试中构造包含额外 cost 字段的响应，仍以 tokens 计费（不读取 cost）

### 2. 测试与回归

- [√] 2.1 更新/新增单测：quota provider 组装逻辑（默认应为 normal；禁用计费/self_mode 应为 free）
  - 依赖: 1.1

- [√] 2.2 新增 store 层单测：`ReserveUsageAndDebitBalance` + `CommitUsageAndRefundBalance` 的余额变化与 committed_usd 落库正确性
  - 依赖: 1.2

- [√] 2.3 运行 Go 测试（含现有用量/路由相关用例）
  - 验证: `go test ./...`

### 3. 文档与知识库同步

- [√] 3.1 更新数据模型文档：明确 PayG 计费口径（tokens + managed_models 定价）与余额扣减流程
  - 位置: `helloagents/wiki/data.md`

- [√] 3.2 更新变更记录（Unreleased）：记录“余额扣费修复 + 计费策略修复 + PayG 结算补扣”
  - 位置: `helloagents/CHANGELOG.md`

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
