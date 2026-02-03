# 变更提案: 缓存粘性路由最小增强

## 需求背景

Realms 当前的“缓存相关优化”主要集中在两类：

1. **请求体缓存（BodyCache）**：让 handler 可重复读取 body，用于“校验 → 转发 → 重试”。
2. **路由粘性（RouteKey 绑定）**：基于请求中的某个稳定 key 做选择结果的短期绑定，以提升上游 prompt cache 命中率并减少抖动。

对照 `rc-balance` 的实现与目标，Realms 在“粘性绑定 + failover”上已具备主体能力（routeKey 提取、绑定命中、审计事件等），但仍有几个真实短板需要补齐：

- **429 冷却窗口偏短**：当前可重试失败的冷却窗口一致，遇到上游 `429` 时容易在短时间内重复撞限流，导致抖动与尾延迟升高。
- **关键路径缺少单元测试**：routeKey（body 优先、header 兜底）与审计事件“不记录上游错误 body/请求体”的约束缺少测试覆盖，容易回归。
- **缺少明确的行为口径文档**：`wiki/modules/realms.md` 未沉淀 routeKey 提取顺序、冷却口径与缓存 token 统计口径，定位问题成本高。

本变更以“最小变更增强”为原则：不引入新依赖、不新增数据库表，优先补齐稳定性与可验证性（测试/文档）以及 `429` 冷却细化。

## 变更内容

1. **细化 429 冷却**
   - 对上游返回 `429` 的凭证设置更长的短期冷却窗口（仍为内存态），减少反复撞限流
2. **补齐单元测试（防回归）**
   - routeKey：验证 `prompt_cache_key`（body）优先级与 header 兜底候选
   - 审计事件：验证 failover/上游错误会落库且不记录上游错误 body/请求体
3. **补齐模块文档口径**
   - 在 `wiki/modules/realms.md` 增加 routeKey 提取顺序、粘性绑定与 429 冷却口径、缓存 token 统计口径

## 影响范围

- **模块:**
  - `internal/scheduler`：冷却策略（429 特例）
  - `internal/api/openai`：补齐 routeKey/审计相关测试
  - `helloagents/wiki`：补齐行为口径文档
- **API:**
  - 不新增/不破坏对外 API；仅增强路由行为（向后兼容）
- **数据:**
  - 不新增表/不做迁移；复用 `audit_events`（已有）

## 核心场景

### 需求: RouteKey 提取（body 优先）
**模块:** `internal/api/openai`

#### 场景: body 含 prompt_cache_key，无 header
- 预期结果: 以 `prompt_cache_key` 作为 routeKey 参与粘性绑定；不要求客户端额外设置 header。

#### 场景: header 兼容（更广候选）
- 预期结果: 当 body 未提供时，仍可从 `x-prompt-cache-key`/`x-rc-route-key`/`conversation_id`/`session_id` 等头部稳定提取 routeKey（不透传内部头）。

### 需求: 绑定命中与冷却联动
**模块:** `internal/api/openai` / `internal/scheduler`

#### 场景: 命中绑定但凭证已冷却
- 预期结果: 跳过已冷却 selection，快速选择其他可用 selection；避免固定重试造成的额外失败延迟。

### 需求: 429 冷却更长
**模块:** `internal/scheduler`

#### 场景: 上游 429 后短期内不再选择同一凭证
- 预期结果: 在冷却窗口内同一凭证不会被再次选中，从而减少“重复撞限流→失败→重试”的抖动。

### 需求: 基础审计事件（路由关键事件）
**模块:** `internal/api/openai` / `internal/store`

#### 场景: failover/上游错误可追溯
- 预期结果: 当发生 failover 或最终返回上游错误时，审计表可查询到事件（不含敏感信息），便于复盘与定位。

## 风险评估

- **风险:** 路由策略更敏感可能导致“切换更快”，在少数短暂抖动场景下可能减少“坚持同一凭证”的重试机会  
  **缓解:** 仅避免无意义重复（例如已冷却/429）；保留有限次数的整体尝试上限。
- **风险:** 审计事件写入带来额外 DB I/O  
  **缓解:** 仅记录关键事件（failover/上游错误），不做“每请求全量落库”。
