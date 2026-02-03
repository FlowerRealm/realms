# 渠道封禁到期自动探测（半开）+ 封禁时长上限

## 背景

当前上游调度在发生可重试失败时，会对 credential 做冷却（cooldown），并对 channel 做封禁（ban）。封禁到期后 channel 会重新进入候选集，但在“渠道组路由”的排序中，**失败分值（failScore）会持续累加且不会自然回落**，导致某些 channel 即使已恢复，也可能长期处于候选队列后部，难以“回归使用”。

同时，现有封禁时长会随连续失败线性累加，并可能在并发失败场景下叠加延长，**在极端情况下会超过 10 分钟**，不符合“封禁时间不超过 10 分钟”的期望。

## 现状梳理（代码定位）

- 渠道封禁与到期判断：
  - `internal/scheduler/state.go`：`BanChannel`、`IsChannelBanned`、`ClearChannelBan`
- 渠道失败分值用于排序（不回落）：
  - `internal/scheduler/state.go`：`RecordChannelResult`、`ChannelFailScore`
- 渠道选择（非组路由路径）：
  - `internal/scheduler/scheduler.go`：`SelectWithConstraints`、`orderChannels`
- 渠道组路由排序（生产主路径）：
  - `internal/scheduler/group_router.go`：`sortCandidates`

## 目标

1. **封禁时长上限**：任一 channel 的封禁截止时间 `ban_until` 与“当前时间”的差值不超过 **10 分钟**。
2. **封禁到期自动探测（无流量也测）**：当 channel 封禁到期后，自动进入“半开（probe）”状态，并在**无用户流量**时由后台触发一次测试：
   - 探测成功 → 视为恢复，回归正常调度；
   - 探测失败（可重试失败）→ 重新进入封禁。
3. **回归使用**：探测成功或正常成功后，失败分值不应永久压制该 channel，需支持恢复排序权重。

## 非目标

- 不引入跨实例共享的封禁状态（当前 `scheduler.State` 为单实例内存态）；如需多实例一致性，另开需求。
- 不引入常驻高频健康检查：后台仅在 **ban 到期且 probe due** 时触发测试，并限制单次 tick 的探测数量，避免探活流量膨胀。

## 方案设计（推荐：半开探测）

### 1) 为 channel 增加“到期探测”状态（State 内存态）

在 `scheduler.State` 增加两张表：

- `channelProbeDueAt[channelID] = time`：封禁到期后标记“可探测”。
- `channelProbeClaimedUntil[channelID] = time`：探测已被占用（防止并发流量同时把探测 channel 顶到第一位）。

触发与清理规则：

- 当 `IsChannelBanned` 发现封禁到期时：
  - 删除 `channelBanUntil[channelID]`；
  - 写入 `channelProbeDueAt[channelID]=now`（若不存在）。
- 当一次请求实际对该 channel 产生结果回报（`Scheduler.Report` 被调用）时：
  - 清理 `channelProbeDueAt/channelProbeClaimedUntil`（表示“已完成探测”）。

### 2) 调度排序：probe 优先于 promotion/priority

在两处排序逻辑加入 probe 优先级（仅对“未被 claimed 的 probe”生效）：

- `internal/scheduler/group_router.go`：`sortCandidates` 比较器中，forced 之后插入 probe。
- `internal/scheduler/scheduler.go`：`orderChannels` 中加入 probe 分类（兼容非组路由路径）。

### 3) “回归使用”：成功后重置 channel 失败分值

在 `Scheduler.Report(success=true)` 分支中，除现有 `ClearChannelBan` 外，额外将 `channelFails[channelID]` 重置为 0（或显著衰减）。这样 channel 一旦恢复成功即可回归正常排序，避免永久压制。

### 4) 封禁时长上限（10 分钟）

在 `State.BanChannel` 计算 `newUntil` 后，做 clamp：

`newUntil = min(newUntil, now.Add(10*time.Minute))`

并确保当已有 `ban_until` 超出上限时也会被收敛到上限。

### 5) 无流量自动探测（后台 loop）

为满足“无流量也测”的诉求，在服务启动时新增后台 loop：

- `internal/server/app.go`：`channelAutoProbeLoop`
  - 周期性调用 `Scheduler.SweepExpiredChannelBans(now)`，把“已到期 ban”转为 probe due；
  - 通过 `Scheduler.ListProbeDueChannels(now, limit)` 取出待探测渠道（limit 默认 1，避免并发探测风暴）；
  - `TryClaimChannelProbe(channelID, now, claimTTL)` 做单飞；
  - 复用管理后台的渠道测试逻辑 `admin.RunChannelTest`（真实走 upstream executor）：
    - 成功：`ClearChannelBan` + `ResetChannelFailScore`（回归使用）；
    - 失败：清理 probe 并重新 ban（仍会被 clamp 到 ≤10 分钟）。

## 测试计划

新增/更新单元测试覆盖：

- `BanChannel` 不会将 `ban_until` 推到 10 分钟之后（包含并发/多次失败叠加场景）。
- 封禁到期后会进入 probe，并在排序中优先被选择一次（组路由路径）。
- probe 被占用（claimed）时不会被并发请求持续优先（避免探测风暴）。
- 成功后 `channelFails` 被重置，排序可恢复。
- probe due 列表可正确过滤 claim 中的 channel，并按 dueAt 排序。

## 风险与回滚

- 风险：封禁到期后的“首个请求”可能被用于探测，若 channel 仍不稳定，会产生一次 failover。
- 风险：在无流量场景下后台会产生少量探测请求（仅对 probe due channel 触发，受限于 tick/limit）。
- 缓解：probe claim（占用）+ 仅一次探测；失败仍走原有 failover。
- 回滚：该功能全部为内存态策略，可通过回退提交恢复到原行为。
