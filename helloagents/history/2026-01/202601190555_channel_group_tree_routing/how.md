# 技术设计: 渠道组树形路由（Channel Group Routing）

## 技术方案

### 核心技术
- Go `net/http`
- MySQL（内置迁移 `internal/store/migrations/*.sql`）

### 实现要点
1. **成员关系 SSOT**：新增 `channel_group_members` 表表达“组→子组/渠道”关系与排序；并将其作为路由编排 SSOT。
2. **组内递归调度**：数据面统一从 `default` 根组开始，按成员顺序与 `max_attempts` 递归尝试，直到选出可用叶子渠道或耗尽。
3. **复用现有三层切换**：叶子渠道选中后，仍复用现有 `scheduler` 的 Endpoint/Credential 选择与 failover 记录。
4. **渠道级自动 ban**：在 `scheduler.State` 中新增渠道冷却（ban）与失败次数计数；对“可重试失败”叠加 ban 时长，并在调度中直接跳过。
5. **兼容缓存**：保留 `upstream_channels.groups` 字段作为兼容缓存（CSV），由成员关系变更时回填更新，降低改造冲击面。

## 架构设计

```mermaid
flowchart TD
    Req[数据面请求 /v1/*] --> Handler[openai handler]
    Handler --> GroupRouter[Group Router\n(default 根组递归)]
    GroupRouter --> Scheduler[Scheduler\nEndpoint→Credential]
    Scheduler --> Exec[Upstream Executor]
    Exec --> Upstream[上游]
    GroupRouter --> Store[(MySQL\nchannel_groups + channel_group_members)]
    Scheduler --> Store
```

## 架构决策 ADR

### ADR-006: 引入 `channel_group_members` 作为路由编排 SSOT
**上下文:** 现有 `upstream_channels.groups` 为 CSV 标签，不支持表达层级/排序/递归调度；管理后台也无法在组内继续组织内容。  
**决策:** 新增 `channel_group_members`（组→成员）关系表，支持成员类型（子组/渠道）与排序字段（priority/promotion）。  
**理由:** 关系模型可表达树形结构与排序，并为 UI/调度提供明确的 SSOT。  
**替代方案:** 继续使用 CSV + 约定（拒绝原因：无法表达层级/排序/递归，且 SQL/校验复杂）。  
**影响:** 增加迁移与数据回填；需要新增管理面写入口与一致性维护。

### ADR-007: 统一从 `default` 根组递归调度
**上下文:** 需求要求默认存在一个最外层“大组”，并从该组递归进入；所有数据面请求使用该路由树。  
**决策:** 数据面入口固定从 `channel_groups.name='default'` 对应的根组开始递归调度。  
**理由:** 简化“入口选择”与 UI 心智；方便将 routing 配置集中在一个树结构下。  
**替代方案:** 每次按用户组集合选择入口组（拒绝原因：与需求不符，且入口选择会引入更多歧义）。  
**影响:** 需要确保 `default` 始终存在且不可禁用/删除；并提供迁移时的初始树结构。

## 数据模型

### 1) 新增表: `channel_group_members`

设计目标：
- 表达 parent group 的成员列表（成员可以是“子组”或“叶子渠道”）
- 支持成员排序（priority/promotion）
- 强制“子组只能有一个父组”（仅叶子渠道可被多个组引用）

```sql
-- 仅示意（最终以 migrations 为准）
CREATE TABLE IF NOT EXISTS `channel_group_members` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `parent_group_id` BIGINT NOT NULL,
  `member_group_id` BIGINT NULL,
  `member_channel_id` BIGINT NULL,
  `priority` INT NOT NULL DEFAULT 0,
  `promotion` TINYINT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_parent_member_group` (`parent_group_id`, `member_group_id`),
  UNIQUE KEY `uk_parent_member_channel` (`parent_group_id`, `member_channel_id`),
  UNIQUE KEY `uk_group_single_parent` (`member_group_id`),
  KEY `idx_parent_order` (`parent_group_id`, `promotion`, `priority`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

> 说明：`uk_group_single_parent(member_group_id)` 通过 MySQL 对 NULL 的处理实现“子组单父”；渠道复用通过 `member_channel_id` 不设全局唯一实现。

### 2) 扩展表: `channel_groups`
- 增加路由字段：`max_attempts`（组内尝试次数，默认 5，可在 UI 配置）
- 其余字段（`price_multiplier/status/description`）保持原语义，用于计费倍率、对话分组字典等

### 3) 兼容缓存: `upstream_channels.groups`
- 保留 CSV 字段作为兼容缓存
- 由 `channel_group_members`（渠道所属的所有组）回填更新

## 调度与算法

### 1) 入口与约束
- 入口固定为 `default` 根组
- 仍复用现有约束来源：
  - `cons.AllowChannelIDs`：模型绑定白名单（必选）
  - `cons.RequireChannelType`：chat 请求强制 `openai_compatible`
  - `cons.AllowGroups`：用户组集合/对话分组集合（作为附加护栏；核心过滤发生在组树 traversal 上）

### 2) 递归调度（Group Router）
- 每个组维护 `max_attempts`，在组内循环尝试成员，直到成功或耗尽
- 成员选择顺序：`promotion` 优先，其次 `priority`，再按失败评分/ID 做稳定排序
- 成员类型：
  - **子组**：递归进入子组执行同样逻辑；子组耗尽视为失败，父组继续同级下一个成员
  - **渠道**：直接锁定该 `channel_id`，交由现有 `scheduler` 选择 endpoint/credential
- 去重与安全：
  - 同一次请求中维护 `visitedGroups` 与 `triedChannels`，避免误配置导致死循环/重复尝试

### 3) 渠道 ban（自动避让）
触发条件：仅对 **可重试失败**（Retriable=true）生效。
- 状态：`channel_fail_streak[channel_id]` 与 `channel_ban_until[channel_id]`
- 策略：失败次数递增时，ban 时长递增（建议指数退避并做溢出保护）；成功清零 streak 并解除 ban
- 调度：当 `now < ban_until` 时，该渠道直接跳过

## 管理后台（UI/交互）

### 新增/调整页面能力
- 在 `/admin/channels`（或新增 group 详情页）支持：
  - 进入某个渠道组（默认进入 `default`）
  - 在组内新建子组/新建渠道/引用已有渠道
  - 组内成员拖拽排序（更新 `channel_group_members.priority`）
  - 配置组内 `max_attempts`

### 校验规则
- 子组只允许单父：写入时校验（DB `uk_group_single_parent` + 应用层提示）
- 防环：保存 parent→child group 时做环检测（避免 A→B→A）
- 组/渠道 status=0 的成员在路由中视为不可用

## 安全与性能
- **安全:**
  - 管理后台仅 root 可写；所有写操作进行参数校验（ID 存在性、组名合法性、环检测）
  - 保持 SSE “写回后禁止 failover”的边界不变
- **性能:**
  - `channel_group_members` 增加 parent+order 索引以支持快速列出成员
  - group tree 读取可做短 TTL 缓存（后续优化）；首版允许按请求读取（规模通常有限）

## 测试与部署
- **测试:**
  - 单测覆盖：组内递归选择、子组耗尽回退、叶子复用去重、ban 递增与跳过
  - 回归覆盖：原有 handler failover 与 SSE 边界、模型绑定白名单逻辑不回退
  - 执行：`go test ./...`
- **部署:**
  - 先部署代码，再由应用启动自动执行迁移
  - 首次启动后自动迁移并回填成员关系；管理后台可见树形结构并可继续调整
