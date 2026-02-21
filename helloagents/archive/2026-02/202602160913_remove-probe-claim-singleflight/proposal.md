# 变更提案: remove-probe-claim-singleflight

## 元信息
```yaml
类型: 重构
方案类型: implementation
优先级: P0
状态: ✅完成
创建: 2026-02-16
```

---

## 1. 需求

### 背景
当前 `scheduler` 存在 “probe claim 单飞（single-flight）” 机制：当 channel 处于 `probe_due` 时会尝试抢占 probe claim；抢占失败会跳过该 channel。该机制在特定场景下会让并发请求被错误地判定为“上游不可用”（例如：Token 实际只允许 1 个可用 channel、或其它 channel 暂不可用时）。

用户期望：API 端口允许多个并发连接，且同一个上游（单 channel）可以承载多个连接；不希望任何形式的“单连接限制”在调度层生效。

### 目标
- 完全移除 `probe claim` 单飞限制：并发请求不因 probe claim 而被拒绝/失败
- 在仅有 1 个可用 channel 的情况下，`/v1/*` 支持多个并发请求持续可用
- 保持接口行为不变（不修改北向 API / 返回结构），仅改变内部调度行为

### 约束条件
```yaml
时间约束: P0（需要尽快恢复并发可用性）
性能约束: 不引入全局锁/等待；保持热路径简单
兼容性约束: 不修改北向 API 行为与返回结构（仅内部调度策略变更）
业务约束: 一个 upstream/channel 允许多个并发连接
```

### 验收标准
- [√] 删除 `TryClaimChannelProbe` / `ReleaseChannelProbeClaim` / `channelProbeClaimUntil` 等 probe claim 相关实现与引用
- [√] `SelectWithConstraints` 不再因 probe claim busy 跳过 channel 或返回无 Selection
- [√] 调度相关单元测试更新并覆盖“probe_due 下多次选择不再单飞跳过”的行为
- [√] `go test ./...` 通过

---

## 2. 方案

### 技术方案
移除 probe claim 单飞机制（删干净）：

1) `State` 移除 `channelProbeClaimUntil` 运行态字段及相关方法（`TryClaimChannelProbe`、`ReleaseChannelProbeClaim`），并简化 `IsChannelProbePending` 的实现（仅由 `channelProbeDueAt` 判断）。  
2) `Scheduler.SelectWithConstraints` 删除 probe claim 相关分支：
   - 绑定命中路径不再抢占 probe claim（不再因为 claim busy 清理 binding）
   - channel 选择路径不再抢占/释放 probe claim，也不再跳过 probe_due channel  
3) 清理关联的运行态清扫逻辑与测试用例，确保行为与代码一致。  

备注：`probe_due`（`channelProbeDueAt`）仍保留，用于表达“封禁过期后需要恢复探测”的语义；只是移除了“并发单飞”的限制。

### 影响范围
```yaml
涉及模块:
  - scheduler: 移除 probe claim 单飞机制与相关状态/逻辑
预计变更文件: 3-5（scheduler/state/runtime_stats/tests + KB 文档）
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| probe_due 场景可能触发更多并发请求打到同一 channel（潜在探测风暴） | 中 | 业务明确要求“一个 upstream 可多连接”；保留 `Report` 及时清理 probe_due；必要时后续引入非阻断的节流（不在本次范围） |
| 行为变更影响故障恢复节奏（探测优先级/切换） | 低 | 更新单元测试覆盖关键行为；发布后观察 proxylog/审计（如启用） |

---

## 3. 技术设计（可选）

> 涉及架构变更、API设计、数据模型变更时填写

### 架构设计
不涉及架构调整；仅删除调度器内部 probe claim 单飞机制。

### API设计
无（不修改北向接口）。

### 数据模型
无（不涉及数据库 schema 变更）。

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: probe_due 下的并发请求
**模块**: scheduler  
**条件**:
- channel 处于 `probe_due`
- 存在并发请求（包括 SSE/长连接）  
**行为**: 调度器不再做 probe claim 单飞限制，允许并发请求继续选择并使用同一 channel  
**结果**: 并发请求不因 “probe claim busy / 单飞机制” 触发“上游不可用”

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### remove-probe-claim-singleflight#D001: 完全移除 probe claim 单飞机制以满足“单上游多连接”
**日期**: 2026-02-16
**状态**: ✅采纳
**背景**: probe claim 单飞会在 `probe_due` 场景下把并发请求收敛为“最多 1 个可用”；在仅 1 个可用 channel 时会导致其它并发请求拿不到 Selection，最终对外呈现为 `502 上游不可用`。该行为与“一个 upstream 可以多连接”的业务预期冲突。
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: 保留 probe claim，但在 claim busy 时回退（不再直接失败） | 可用性提升，仍保留一定单飞语义 | 仍存在“单飞机制”，不满足“删干净”诉求；逻辑更复杂 |
| B: 完全移除 probe claim（本方案） | 满足“单上游多连接、无单飞限制”；逻辑最简单 | probe_due 场景可能产生更多并发探测请求 |
**决策**: 选择方案 B
**理由**: 用户明确要求撤掉单连接限制功能；并发可用性优先于单飞探测保护。
**影响**: `internal/scheduler/*`（state/选择逻辑/运行态统计）及相关测试与模块文档
