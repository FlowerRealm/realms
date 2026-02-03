# 变更提案: channel-pointer-tree

## 元信息
```yaml
类型: 行为变更/重构
方案类型: implementation
优先级: P0
状态: 设计中
创建: 2026-01-28
```

---

## 1. 需求

### 1.1 背景
当前的“渠道指针/手动指定渠道”能力，仍更像“单点置顶覆盖”：只保证某个渠道在排序里优先，但**不是整棵渠道树上“应该用哪个渠道”的唯一标定**，并且缺少“ban 后自动轮转 + 环形回绕”的确定性。

### 1.2 目标（必须满足）
1. **渠道指针 = 唯一标定（SSOT）**：当指针开启时，数据面的渠道选择以该指针为唯一入口，不再受会话粘性/亲和/其它排序策略影响。
2. 指针作用域覆盖 **整棵渠道树**（以 `channel_groups.name='default'` 为根，叶子为 channel）。
3. 指针指向的渠道被调度器 **ban** 后，指针应自动移动到**下一个叶子渠道**。
4. 指针移动到末尾后，应从头开始（**环形回绕**）。

### 1.3 非目标（本方案不强行做）
- 不强制将指针持久化到数据库（进程重启后保持）。如需要可作为增强项追加。
- 不改变“credential/account 选择”的内部策略（仍按同渠道内最小 RPM/冷却等挑选）。

---

## 2. 方案

### 2.1 核心概念定义

#### 2.1.1 Channel Ring（渠道环）
- 定义：把 **default 根组**的“可路由叶子渠道”按照一个**确定的、稳定的规则**展开为线性序列 `ring=[ch1,ch2,...,chn]`，并视为一个环（`next(i)=(i+1)%n`）。
- 约束：`ring` 仅包含“叶子 channel”，不包含 group 节点。

#### 2.1.2 Channel Pointer（渠道指针）
- 定义：全局唯一的 `pointer_channel_id`，始终指向 `ring` 中的一个 channel（当 `ring` 变化导致指针不在 `ring` 中时，会触发“修正”）。
- 语义：当指针开启时，**当前请求的首选渠道**必须从 `pointer_channel_id` 开始（必要时再按 failover 规则向后轮转）。

#### 2.1.3 轮转规则
- **触发点**：当 `pointer_channel_id` 对应渠道进入 ban（即 `ban_until > now` 生效）时，指针必须自动更新为 `ring` 的下一个。
- **回绕**：到达 `ring` 末尾后，回到 `ring[0]`。
- **连续跳过**：若下一个渠道也处于 ban，则继续向后跳（最多走一整圈；一圈都不可用则返回“上游不可用”）。

---

### 2.2 Ring 的生成（如何把树展开为环）

> 关键点：指针在“整棵渠道树”上工作，必须先定义“树 → 线性顺序”的 SSOT。

#### 2.2.1 展开数据来源
- 使用现有 **Channel Group Routing** SSOT：`channel_group_members`（父组 → 子组/渠道）。
- 根组：`channel_groups.name='default'`。

#### 2.2.2 展开顺序（建议采用：稳定 DFS + 组内稳定排序）
1. 对每个 group 的成员，按下述稳定规则排序（不引入运行态动态因子）：
   - `promotion DESC`
   - `priority DESC`
   - `member_id ASC`（或 `created_at ASC`，取决于数据结构里哪个是最稳定且可用的）
2. 按排序后的顺序遍历成员：
   - 成员为 channel：加入 `ring`
   - 成员为 group：递归展开该子组，把子组展开序列直接追加到 `ring`
3. **去重规则**：同一 channel 在树中出现多次时，保留第一次出现（保证 `ring` 里每个 channel 唯一）。
4. 空树：`ring=[]` 视为不可路由。

> 说明：现有实现 `collectCandidates + sortCandidates` 会把整棵树的叶子候选压平成 map 再排序，容易丢失“树结构顺序”。本方案明确把“树的顺序”定义为 DFS 结果，保证指针“在树上移动”更符合直觉。

---

### 2.3 指针状态与写入点

#### 2.3.1 运行态存储位置
继续使用调度器内存态（`internal/scheduler/state.go`）存储指针，但升级为“指针 + ring”组合态：
- `pointerChannelID int64`
- `pointerRing []int64`（最近一次从树展开得到的 ring）
- `pointerRingIndex map[int64]int`（可选：快速定位）
- `pointerAdvancedAt time.Time`（可选：便于管理后台展示/排障）
- `pointerAdvanceReason string`（可选：ban/manual/invalid）

#### 2.3.2 指针写入点（SSOT 规则）
1. **手动设置**（管理后台）：直接把 `pointerChannelID=目标 channel`，并刷新 ring（或标记 ring 需要刷新）。
2. **自动轮转**（ban 触发）：当且仅当对 `pointerChannelID` 的 ban 生效时，指针更新为下一个。
3. **自愈修正**（配置变更/渠道下线）：当发现 `pointerChannelID` 不在 ring（例如渠道被禁用/删除/移出树），自动把指针修正为 `ring[0]`（或“最近邻”，见 2.4）。

---

### 2.4 调度与 failover 行为（指针如何成为唯一标定）

#### 2.4.1 当指针开启时
1. GroupRouter 的渠道遍历必须改为“从指针开始的顺序遍历”，而不是“每次重新按 promotion/priority 选第一名”：
   - 构造 `ring`（2.2）
   - 在 `ring` 中定位 `pointerChannelID` 的 index（找不到则触发修正）
   - 遍历顺序为：`ring[index], ring[index+1], ..., ring[n-1], ring[0], ..., ring[index-1]`（最多一圈）
2. Scheduler 的会话粘性（binding）与亲和（affinity）在指针开启时必须被旁路/覆盖：
   - 指针开启 → 不允许绑定把流量“拉回旧渠道”
   - 这条规则保证“唯一标定”成立

#### 2.4.2 当指针关闭时
保持现有行为（promotion/priority/affinity/binding/探测/失败计分等照旧）。

---

### 2.5 “ban 后轮转”的实现建议（保证自动、确定、可测试）

#### 2.5.1 触发判定
在 `Scheduler.Report` 内，当 `res.Retriable=true` 且 `state.BanChannel*()` **确实把 `ban_until` 推进到未来**时，视为“ban 生效”。

#### 2.5.2 指针推进
- 若 `sel.ChannelID == pointerChannelID` 且本次 ban 生效：
  - 依据 `pointerRing` 计算下一个可用 channel，更新 `pointerChannelID`
  - 若 `pointerRing` 为空或不包含该 channel：触发 ring 重新构建（或先降级为 `pointerChannelID=0`，下一次请求再自愈）

---

### 2.6 管理后台（UI/API）

#### 2.6.1 UI 目标
- `/admin/channels` 页头展示：
  - `渠道指针：{name (#id)}` 或 `-`
  - （可选）最近一次自动轮转时间/原因
- 渠道列表行内：
  - 当前指针渠道显示“指针”标记
  - 操作按钮：“设为指针”

#### 2.6.2 API
为兼容历史入口，沿用路由：
- `POST /admin/channels/{channel_id}/promote`
但语义明确为：**设为渠道指针**（可选：重命名 handler 为 `PinChannelPointer`）

可选增强：
- `POST /admin/channels/pointer/clear`：清除指针，恢复“自动调度模式”

---

## 3. 验收标准
- [ ] 指针开启后，所有请求都从指针渠道开始选（不会被会话粘性/亲和拉走）。
- [ ] 指针渠道 ban 生效后，指针自动移动到下一个渠道；并可持续轮转与回绕。
- [ ] 指针轮转到末尾后，会从头开始（环形）。
- [ ] `/admin/channels` 能看到当前指针与行内标记；按钮可把指针指到任意渠道。
- [ ] `go test ./...` 通过，且新增/更新测试覆盖“ban 触发轮转 + 回绕”。

---

## 4. 风险与边界
- 配置变更（树结构/排序）会导致 ring 变化：需要定义“指针不在 ring 时”的自愈策略，避免 UI/调度出现悬挂指针。
- 指针为全局共享状态：高并发下推进必须保证原子性（`State.mu` 足够，但要避免长时间持锁做 IO；ring 构建应在锁外完成）。
