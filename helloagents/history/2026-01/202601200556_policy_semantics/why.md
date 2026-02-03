# 变更提案: 语义化功能禁用（Policy Semantics）

## 需求背景

当前系统存在两类“禁用”能力：

1. `self_mode.enable=true`：偏“自用档位”，会禁用计费/支付/工单相关路由，并把数据面配额策略放宽（不再要求订阅/余额）。
2. `app_settings.feature_disable_*`：偏“UI/路由级禁用”，用于隐藏入口并对部分路由返回 404。

问题在于：`feature_disable_*` 目前主要是“页面/路由裁剪”，并不表达“业务语义”。例如用户希望：

- “禁用支付/订阅/充值”不仅是隐藏页面，而是 **数据面无限用量（不校验订阅/余额）**；
- “关闭模型”不仅是隐藏模型管理入口，而是 **模型穿透（不要求模型白名单/绑定）**。

为避免把 UI 禁用与数据面行为强耦合（误操作风险高），需要引入一组 **明确的策略开关（policy_*）**，专门控制数据面的业务语义；同时保持 `feature_disable_*` 只负责“入口/路由 404”。

## 变更内容

1. 新增 `app_settings.policy_*`：用于表达“数据面语义”的策略开关（不与 UI 禁用复用同一键）。
2. 保持 `feature_disable_*`：继续只负责“隐藏入口 + 路由 404”，并修正“配置文件默认值（app_settings_defaults）无法真正 gate 路由”的一致性问题。
3. 实现至少两条核心语义：
   - `policy_free_mode=true`：数据面进入“无限用量”模式（不校验订阅/余额；仍记录 `usage_events` 便于排障/统计）。
   - `policy_model_passthrough=true`：数据面进入“模型穿透”模式（不要求模型已启用/有绑定；请求 `model` 直接透传到上游）。

> 扩展点：同一模式可按需追加更多语义化策略（如 `policy_chat_passthrough` 等），但本提案优先把“计费”和“模型”两条主线做对。

## 影响范围

- **模块:**
  - `internal/store`（新增 policy keys、读取策略状态、迁移兼容）
  - `internal/quota`（支持 free mode；避免 unknown model 导致计费估算失败）
  - `internal/api/openai`（支持模型穿透与更宽松的调度约束）
  - `internal/middleware`（FeatureGate 一致性：支持配置默认值/自用模式的 effective gate）
  - `internal/admin`（可选：在系统设置页暴露 policy_* 开关）
- **文件:** 见 task.md 任务清单
- **API:**
  - 数据面：`POST /v1/responses`、`POST /v1/chat/completions`（行为在策略开启时变化）
  - 管理面：`GET/POST /admin/settings`（新增策略开关配置项时）
- **数据:**
  - `app_settings`：新增 `policy_*` keys
  - `usage_events`：为区分“按量计费预留（会扣余额）”与“free mode 预留（不扣余额）”，可能需要新增字段（见 how.md）

## 核心场景

### 需求: 计费域语义化禁用（无限用量）
**模块:** quota / openai handler

#### 场景: 开启 free mode 后数据面不再要求订阅/余额
条件：
- 管理后台设置 `app_settings.policy_free_mode=true`（或配置默认值开启）
- 用户无订阅、余额为 0

预期：
- `/v1/responses`、`/v1/chat/completions` 不再返回 `429 订阅未激活/额度不足` 或 `402 余额不足`
- 仍记录 `usage_events`（用于排障与用量统计）

#### 场景: 仅禁用 Billing UI/路由，不改变数据面计费语义
条件：
- 设置 `feature_disable_billing=true`
- 未开启 `policy_free_mode`

预期：
- `/subscription`、`/topup`、`/pay/*` 等返回 404（与现有一致）
- 数据面仍按原策略执行（订阅优先 + 余额兜底 / 或按现有配置）

### 需求: 模型域语义化禁用（模型穿透）
**模块:** openai handler / scheduler

#### 场景: 开启模型穿透后不再要求模型白名单/绑定
条件：
- 设置 `app_settings.policy_model_passthrough=true`
- 请求携带任意 `model`（例如 `gpt-unknown-foo`）

预期：
- 不再要求 `managed_models` 中存在该 model，也不要求 `channel_models` 绑定
- `model` 字段直接透传到上游（不做 alias rewrite）
- 调度仍受用户分组/渠道可用性约束（不绕过安全护栏）

### 需求: FeatureGate 一致性
**模块:** middleware / store

#### 场景: 配置文件默认禁用也能真正 gate 路由
条件：
- `app_settings_defaults.feature_disable_web_chat=true`
- 数据库中未写入 `app_settings.feature_disable_web_chat`

预期：
- 相关路由仍返回 404（避免“UI 显示禁用但路由可访问”的不一致）

## 风险评估

- **风险:** free mode 与按量计费（余额扣费）的状态机混淆，可能导致余额被错误返还/扣减。
  - **缓解:** 在 `usage_events` 中显式区分“是否扣过余额的预留”，并据此做 refund/expire 逻辑（见 how.md 数据模型）。
- **风险:** 模型穿透会放宽输入约束，可能导致上游返回更多 4xx/5xx。
  - **缓解:** 仍保留路由/调度护栏（用户分组、渠道健康、SSE 写回后禁止 failover），并在审计/用量明细中记录错误分类以便排障。

