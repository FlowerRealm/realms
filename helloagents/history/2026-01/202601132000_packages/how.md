# 套餐功能（用量配额：5 小时 / 周 / 月，按月购买）- 方案设计

## 总体策略（先落地最小闭环）

这件事本质是“按用户做多时间窗配额控制”，并且计量单位是“成本”，时间窗要求“滑动窗口 + 相对时间”。落地优先级：
1. 先实现按用户的配额校验闭环（`user_id` 用占位符接入，不阻塞 user-system 其他模块）
2. 套餐按月开通，允许并行叠加，额度做汇总
3. 超额直接拒绝；通过配置开关控制启用，默认关闭以保证兼容

## 关键概念与数据模型（建议）

### 1) 套餐定义（PlanDefinition / SKU）

套餐定义来自配置（避免硬编码）：
- `sku`：例如 `BASIC` / `PRO`
- `limits`：
  - `limit_cost_5h`
  - `limit_cost_week`
  - `limit_cost_month`
- `enabled`

> 这里的三个限额是“同一套餐内的三类时间窗配额”（多桶配额），而不是三种互斥套餐。

### 2) 用户订阅（UserSubscription）

每次按月购买/开通生成一条订阅记录：
- `user_id`（占位符即可）
- `sku`
- `starts_at`
- `ends_at`（按月：相对月，按购买时间 + 1 month）
- `status`：`active` | `expired` | `revoked`
- `source`：`admin_grant` | `purchase`（初期只用 `admin_grant`）

### 3) 并行叠加规则（已确认）

同一用户的“有效额度”按**所有 active 订阅**汇总：
- `effective_limit_cost_5h = Σ(plan.limit_usd_micros_5h)`
- `effective_limit_cost_week = Σ(plan.limit_usd_micros_week)`
- `effective_limit_cost_month = Σ(plan.limit_usd_micros_month)`

## 成本计量（Cost）实现要点

### 成本计算

成本通常需要“token 使用量 + 模型定价表”才能计算，因此需要：
- 从现有用量统计中拿到 tokens（若已有 `input/output/reasoning/total`，优先复用）
- 为每个 model 提供定价配置（管理员维护；至少 input/output 两个单价；reasoning 的归属需明确）
- 统一成本单位（`usd_micros`，整数微单位，避免 float）

### 未知模型/alias 的处理

必须明确兜底策略，否则会出现“无法计费 → 无法校验”：
- 不认识的模型：默认拒绝（提示“余额不足”或“未配置定价”，实现阶段统一错误体）

## 滑动窗口（Rolling Window）实现要点

需求要求：
- `5h`：过去 5 小时内累计成本
- `week`：过去 7 天内累计成本
- `month`：过去 30 天内累计成本（相对时间）

### 朴素方案（简单但可能重）

记录每次请求的 `usage_event(user_id, ts, cost)`，校验时做：
- `sum(cost) where ts >= now - window`

优点：逻辑直观。
缺点：高 QPS 下读写压力大，索引与表膨胀会很快成为瓶颈。

### 推荐方案（分桶聚合 + 近似滑动）

用“时间分桶”来近似滑动窗口（精度由桶宽决定）：
- 5h：按 1 分钟桶或 5 分钟桶聚合
- week/month：按 1 小时桶聚合（或日桶）

校验时对最近 N 个桶求和即可。桶宽越小越接近严格 rolling，但读写成本更高。

## 校验挂载点与兼容性

### 启用开关

新增配置项：
- `packages.enabled`（默认 `false`）

当 `false`：不校验，保持现有行为。
当 `true`：对业务请求做配额校验；管理接口不校验。

### 校验流程（建议）

1. 从数据面鉴权链路提取 `user_id`（来自数据库）
2. 查询用户 active subscriptions → 汇总有效额度（并行叠加）
3. 读取对应三类窗口的已用成本（rolling 5h / 7d / 30d）
4. 若任一窗口超额 → 直接拒绝（提示“余额不足”，无需返回额外字段）
5. 请求结束后落账：把本次成本写入分桶（或事件表）

> 注意：成本往往在请求结束后才能精确拿到；要做“绝对硬限制”需要预扣/预估（复杂度更高），初期可接受最后一笔轻微穿透。

## 管理侧能力（最小闭环）

建议新增管理 API（路径与现有 management 路由保持一致）：
- 列出套餐定义（SKUs 与三类成本限额）
- 为用户开通套餐（按月）
- 查询用户订阅与用量（各窗口已用/剩余）
- 撤销订阅（可选）

## 存储建议

需要同时存两类数据：
1. 订阅（低频写）
2. 用量桶/事件（高频写，必须并发安全）

如果系统可能多实例运行，优先选择数据库（例如 Postgres/MySQL）作为底座；纯文件存储很快会遇到一致性问题。
