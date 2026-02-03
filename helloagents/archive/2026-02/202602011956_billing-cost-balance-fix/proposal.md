# 变更提案: billing-cost-balance-fix

## 元信息
```yaml
类型: 修复
方案类型: implementation
优先级: P0
状态: 草稿
创建: 2026-02-01
```

---

## 1. 需求

### 背景
当前项目存在两类计费问题：
1) **请求费用口径受“上游倍率”影响**：现状疑似从上游侧拿“费用/金额”或受上游倍率影响的口径，导致平台侧显示费用与预期不一致；期望改为：**仅使用上游返回的 tokens（usage）**，在本项目侧缓存 tokens，并按本地定价表计算费用。
2) **用户余额未扣减**：Web UI「用户管理」中用户余额（USD）在实际请求消耗后未发生变化，导致按量计费失效。

项目分析结论（关键发现）：
- 服务端 `quotaProvider` 当前固定使用 `FreeProvider`（free mode），会记录 `usage_events` 但不会更新 `user_balances`，直接导致“余额不变”。
- 按量计费的 reserve→commit 结算逻辑在 `CommitUsageAndRefundBalance` 中对 `committed_usd > reserved_usd` 采取“直接截断为 reserved”的保守策略，可能导致实际成本无法完整扣减（即使用户余额充足）。

### 目标
- **费用计算口径**：基于上游响应中的 tokens（`usage.{input_tokens/output_tokens}` 或 `usage.{prompt_tokens/completion_tokens}`），将 tokens 写入 `usage_events`（含 cached tokens），并用本地 `managed_models` 定价计算 `committed_usd`。
- **余额扣减正确**：在启用计费（非 self_mode 且未禁用 billing）且启用按量计费时，请求完成后用户 `user_balances.usd` 按实际 `committed_usd` 发生扣减（并在 Web UI 可见）。

### 约束条件
```yaml
时间约束: 尽快恢复按量计费可用性（P0）
性能约束: 不引入高开销的全量 tokenization；优先复用现有“上游 usage 提取”逻辑
兼容性约束: 兼容所有上游类型（OpenAI-compatible / Anthropic / Gemini / Codex OAuth），以现有 usage 字段提取为准
业务约束: 余额不得扣成负数；计费禁用（feature_disable_billing/self_mode）时保持 free mode 行为
```

### 验收标准
- [ ] **按量计费启用时**：单次请求结束后，`usage_events` 记录 input/output/cached tokens，且 `committed_usd > 0`（存在模型定价时），用户余额按 `committed_usd` 扣减。
- [ ] **余额不足**：预留阶段余额不足时请求返回 `402 Payment Required`（保持现有语义）；结算阶段不允许余额变为负数（允许“扣到 0”）。
- [ ] **free mode**（`self_mode.enable=true` 或 `feature_disable_billing=true`）下：请求不扣用户余额，但仍记录 `usage_events`（用于可观测性）。
- [ ] **不依赖上游金额字段**：即使上游响应包含“费用/金额”扩展字段，本项目计费仍以 tokens + 本地定价为准（不读取/不信任上游费用）。

---

## 2. 方案

### 技术方案
- **Provider 选择修复**：在 `internal/server/app.go` 注入“功能开关驱动的 quota provider”：
  - normal：`HybridProvider`（订阅优先 + 余额兜底）
  - free：`FreeProvider`（仅记录 usage_events，不扣余额）
  - 运行时依据 `self_mode` 与 `feature_disable_billing` 选择（复用现有 Feature Bans 逻辑）。
- **按量计费结算修复**：在 `internal/store/usage_balance.go` 的 `CommitUsageAndRefundBalance` 中支持“结算补扣”：
  - 若 `actual_cost > reserved_usd`：在余额充足时追加扣减差额；余额不足时最多扣到 0，并将实际扣减值落到 `committed_usd`（不允许负数）。
  - 若 `actual_cost < reserved_usd`：按现有逻辑返还差额。
- **费用计算口径**：保持（并补充测试确认）现有 `estimateCostUSD` 逻辑：`managed_models` 定价 + tokens 计算 `committed_usd`；不从上游读取费用字段。

### 影响范围
```yaml
涉及模块:
  - internal/server: quota provider 组装与配置注入
  - internal/quota: payg/订阅/free provider 行为验证（主要复用现有实现）
  - internal/store: payg 余额结算逻辑（reserved/commit/refund）
  - internal/api/openai: usage tokens 提取与入账链路（以校验/补测试为主）
  - router/web: 管理后台用户余额展示（用于验收）
预计变更文件: 6-10
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 余额扣减口径变更导致账务不一致 | 高 | 增加单测覆盖 reserve/commit/refund/补扣；仅修复“未扣/少扣”方向，不引入负余额 |
| 上游返回 usage 不完整导致成本为 0 或回退口径 | 中 | 保持现有回退：缺 token 时用 reserved 兜底；并在测试中覆盖典型响应形态 |
| free mode 与 normal mode 行为混淆 | 中 | provider 选择用 Feature Bans 统一判定；增加 server 层单测防回归 |

---

## 3. 技术设计（可选）

> 涉及架构变更、API设计、数据模型变更时填写

### 架构设计
```mermaid
flowchart TD
    R[API Handler] -->|Reserve| Q[Quota Provider]
    R -->|Commit(tokens)| Q
    Q -->|write usage_events| DB[(DB)]
    Q -->|debit/refund user_balances| DB
```

### API设计
本变更不新增 API；仅修复后端计费/扣费实现。

### 数据模型
| 字段 | 类型 | 说明 |
|------|------|------|
| usage_events.input_tokens/output_tokens/cached_* | BIGINT | 记录 tokens（来自上游 usage） |
| usage_events.committed_usd | DECIMAL(20,6) | 本地按 tokens + 定价计算后的实际扣费金额 |
| user_balances.usd | DECIMAL(20,6) | 用户按量计费余额（结算补扣/返还更新） |

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: 按量计费请求扣费（Pay-As-You-Go）
**模块**: `internal/quota` + `internal/store`
**条件**: `self_mode=false` 且 `feature_disable_billing=false` 且 `billing_enable_pay_as_you_go=true`
**行为**:
- 请求开始：Reserve 预留并扣减 `reserved_usd`
- 请求完成：从上游响应提取 tokens → 计算 `actual_cost_usd` → Commit 结算（差额返还/必要时补扣）
**结果**:
- `usage_events` 记录 tokens 与 `committed_usd`
- `user_balances.usd` 减少（不为负）

### 场景: free mode（自用/禁用计费）
**模块**: `internal/quota`
**条件**: `self_mode=true` 或 `feature_disable_billing=true`
**行为**: 仅记录 `usage_events`，不做余额扣减/返还
**结果**: 用户余额保持不变，但用量可观测

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### billing-cost-balance-fix#D001: quota provider 以 FeatureProvider 统一选择（normal=Hybrid，free=Free）
**日期**: 2026-02-01
**状态**: ✅采纳
**背景**: 当前固定使用 free provider 导致按量计费与余额扣减完全失效，需要在不破坏 self_mode/禁用计费语义下恢复正常计费路径。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: FeatureProvider(normal=Hybrid, free=Free) | 语义集中、与现有 feature_disable_* 逻辑一致、易测 | 需要在 server 组装层注入 cfg/self_mode |
| B: 在 handler 层自行判断并分支扣费 | 改动局部 | 容易分叉口径，难维护，重复判断 self_mode/feature flags |
**决策**: 选择方案 A
**理由**: 计费模式选择属于“策略层”，应集中在 quota provider 内；server 只负责组装与注入配置。
**影响**: `internal/server/app.go`、`internal/quota/*`（行为验证）与相关测试

### billing-cost-balance-fix#D002: PayG 结算支持“余额充足时补扣差额，不足则扣到 0”
**日期**: 2026-02-01
**状态**: ✅采纳
**背景**: 现有实现将 `actual_cost > reserved_usd` 直接截断为 `reserved_usd`，导致即使余额充足也无法扣到真实成本，产生“用量有成本但余额不变/少变”的错觉与财务偏差。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 结算时追加扣减差额（不允许负数） | 尽可能贴近真实成本，避免系统性少扣 | 余额不足时仍可能出现“未全额扣费”（但至少扣到 0 并阻断后续） |
| B: 仍然截断为 reserved | 实现简单、绝不负数 | 系统性少扣，无法满足按量计费预期 |
| C: 结算不足则标记异常/拒绝并回滚请求 | 财务严格 | 请求已完成无法回滚；引入复杂补偿与用户体验问题 |
**决策**: 选择方案 A
**理由**: 在不引入复杂 tokenization 与补偿的前提下，最大化扣费准确性，且保持“不负数”的业务约束。
**影响**: `internal/store/usage_balance.go` 与相关单测
