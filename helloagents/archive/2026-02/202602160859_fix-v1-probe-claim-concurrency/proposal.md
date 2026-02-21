# 变更提案: fix-v1-probe-claim-concurrency

## 元信息
```yaml
类型: 修复
方案类型: implementation
优先级: P0
状态: 已实现（待归档）
创建: 2026-02-16
```

---

## 1. 需求

### 背景
近期对项目做了一次安全补丁后，`/v1/*` 出现并发退化：同一时间通常只能成功 1 个请求，其余并发请求直接返回 `502 上游不可用`。

问题的关键在于：上游实际可用（“有一个窗口可以用”），但 Realms 在路由/调度阶段拿不到可用 Selection 时会统一返回 `502 上游不可用`，导致对外呈现为“上游不可用”的假象。

### 目标
- `/v1/*` 在仅有 1 个上游 channel 的情况下也能支持多个并发请求
- 避免因 `probe claim` 被占用而把并发请求直接判定为“上游不可用”
- 保留 probe 单飞的初衷：当存在其它可用 channel 时优先走其它 channel，避免探测风暴

### 约束条件
```yaml
时间约束: 尽快恢复并发能力（阻断业务使用）
性能约束: 不引入全局锁/长时间等待；尽量保持原有热路径开销
兼容性约束: 不修改北向 API 行为与返回结构（仅修复错误场景的选择逻辑）
业务约束: 允许一个上游同时承载多个连接（一个 upstream 可以多连接）
```

### 验收标准
- [ ] 当唯一可用 channel 处于 `probe_due` 且 probe claim 被其他并发占用时，`SelectWithConstraints` 仍能返回 Selection（不再直接失败）
- [ ] 当存在其它可用 channel 时，仍优先选择其它 channel（维持 probe 单飞语义）
- [ ] 当其它 channel 因无可用 credential/account 等原因不可用时，可回退到被 probe claim 占用的 channel（优先可用性）
- [ ] `go test ./...` 通过

---

## 2. 方案

### 技术方案
在 `Scheduler.SelectWithConstraints` 中，对“probe_due 但 probe claim busy”的 channel 不再直接视为不可用：

1) **第一轮**：按既有策略遍历 ordered channels。遇到 `probe_due` 且 probe claim busy 的 channel 时，先记录到 `probeClaimSkipped` 并跳过，继续尝试其它 channel。  
2) **第二轮回退**：若第一轮未选出任何 Selection，则再遍历 `probeClaimSkipped`，**不再尝试 claim**，直接尝试从这些 channel 选择 endpoint + credential。  

效果：在“只有 1 个 channel”或“其它 channel 暂时不可用”的场景下，不会因 probe claim busy 直接触发 `502 上游不可用`，并发请求可继续使用同一上游。

### 影响范围
```yaml
涉及模块:
  - scheduler: 调整 probe claim busy 的回退策略，提升并发可用性
预计变更文件: 2（scheduler + scheduler tests）
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| probe_due 期间仍可能并发流量打到同一 channel（绕过“单飞探测”） | 中 | 仅在“无其它可用 Selection”时触发回退；保留优先其它 channel 的行为 |
| 行为改变导致探测/切换节奏变化 | 低 | 增加单元测试覆盖：单 channel、双 channel + 其它不可用 的回退场景 |

---

## 3. 技术设计（可选）

> 涉及架构变更、API设计、数据模型变更时填写

### 架构设计
不涉及架构调整；仅为调度器内部选择策略的行为修复。

### API设计
无（不修改北向接口）。

### 数据模型
无（不涉及数据库/存储 schema 变更）。

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: `probe_due` 且 probe claim busy 时的并发请求
**模块**: scheduler  
**条件**:
- 仅 1 个可用 channel（或其它 channel 均不可用）
- 该 channel 处于 `probe_due`，且 probe claim 已被其他并发请求占用  
**行为**: 调度器在第一轮尝试其它 channel 无果后，第二轮回退到该 channel 并继续选择 credential/account  
**结果**: 并发请求不再直接返回 `502 上游不可用`，可继续复用同一上游并发连接

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### fix-v1-probe-claim-concurrency#D001: probe claim busy 时采用“两轮选择 + 回退”以恢复并发可用性
**日期**: 2026-02-16
**状态**: ✅采纳
**背景**: 现状会在 probe claim 占用时把 channel 直接跳过；当仅有 1 个可用 channel 时会导致并发请求无 Selection → 统一返回 `502 上游不可用`，与“上游实际可用且可多连接”的业务预期冲突。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 移除 probe claim（完全不单飞） | 并发不受限，实现简单 | 可能产生探测风暴，增加不健康上游的请求压力 |
| B: probe claim busy 时等待/阻塞直到 probe 结束 | 维持严格单飞语义 | SSE/长连接会导致长时间阻塞，本质仍可能“只能同时 1 个” |
| C: 第一轮跳过、无可用则回退（不 claim） | 保留优先其它 channel + 避免探测风暴；同时在无替代时保障可用性与并发 | probe_due 期间在“无替代”情况下仍会并发打到同一 channel |
**决策**: 选择方案 C
**理由**: 目标是恢复并发能力与可用性，同时尽量保留 probe 单飞的保护价值；两轮策略在不增加全局等待的前提下解决“并发直接 502”的核心问题。
**影响**: `internal/scheduler/scheduler.go` 的 channel 选择逻辑；新增/调整单元测试；同步 scheduler 模块文档与 CHANGELOG
