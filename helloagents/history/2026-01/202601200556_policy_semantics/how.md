# 技术设计: 语义化功能禁用（Policy Semantics）

## 技术方案

### 核心技术
- Go（现有代码风格保持不变）
- `app_settings`（运行期策略开关存储）
- `quota.Provider`（配额/计费策略抽象）
- `scheduler`（渠道组路由与 failover）

### 实现要点

#### 1) 引入 PolicyState（与 FeatureState 并行）

- 新增一组 `app_settings.policy_*` 键（bool 为主，必要时可扩展为枚举/字符串）：
  - `policy_free_mode`：true=无限用量（不校验订阅/余额）
  - `policy_model_passthrough`：true=模型穿透（不要求模型白名单/绑定）
  - （可选扩展）`policy_chat_passthrough`：true=忽略 `X-Realms-Chat: 1` 的对话分组约束

- Store 提供 `PolicyStateEffective(ctx, selfMode bool)`：
  - 优先读取 `app_settings`（DB）
  - DB 缺省时读取 `app_settings_defaults`（配置文件默认）
  - 再缺省则为 false（保持现有行为不变）
  - `self_mode.enable=true` 作为“硬档位”覆盖部分策略（至少覆盖 `free_mode`）

> 关键点：policy 只改变数据面语义；不会影响“入口/路由 404”（那是 feature_disable 的职责）。

#### 2) quota：支持 free mode，同时保证 pay-as-you-go 的余额状态机正确

现状问题：当前余额退款/过期逻辑以 `usage_events.subscription_id IS NULL` 作为“按量计费”的判断条件；但 free mode 的 usage_event 也会是 `subscription_id IS NULL`，会导致错误返还余额。

解决方案：在 `usage_events` 显式记录“该预留是否扣过余额”。

推荐字段：
- `balance_reserved_usd` DECIMAL(20,6) NOT NULL DEFAULT 0
  - pay-as-you-go 预留时写入与 `reserved_usd` 相同的值（表示已扣余额）
  - free mode / 订阅预留时为 0

迁移后规则：
- `expireReservedUsageRefundBalances`：仅处理 `balance_reserved_usd > 0` 的过期预留（退回余额）
- `CommitUsageAndRefundBalance` / `VoidUsageAndRefundBalance`：以 `balance_reserved_usd > 0` 判定是否需要退款（而不是 `subscription_id IS NULL`）

同时，free mode 下对“未知模型”的计费估算需要宽容：
- 当 `policy_model_passthrough=true` 或模型不存在时：
  - 预留金额使用默认值（如 0.001 USD）或 0
  - 结算金额可沿用预留金额或记为 0（建议：记为 0，并保留 token 统计）

#### 3) openai handler：模型穿透（不再依赖模型目录与绑定白名单）

当 `policy_model_passthrough=true` 时：
- 不再调用 `GetEnabledManagedModelByPublicID` 强制校验
- 不再要求 `channel_models` 绑定（即不构造 `cons.AllowChannelIDs`）
- `rewriteBody`：不再根据绑定做 `payload["model"]=upstream_model` 重写；直接保持请求 `model` 字段
- Chat Completions 仍只允许 `openai_compatible` 渠道（保持现有约束）
- Responses 允许 `openai_compatible` 与 `codex_oauth`（保持现有行为；由调度器选中后执行器自行处理上游差异）

对 `X-Realms-Chat: 1`：
- 默认仍保持严格模式：需要对话分组集合可用，否则返回 400
- 若后续引入 `policy_chat_passthrough=true`：则忽略对话分组约束（把这类请求当普通请求处理）

#### 4) FeatureGate：统一“effective gate”，保证配置默认值与 self_mode 一致生效

现状：`FeatureStateEffective` 会合并 `app_settings_defaults`，但 `middleware.FeatureGate` 只读 DB，导致“UI 显示禁用但路由可访问”。

改造方向：
- 为 FeatureGate 增加“effective 读取”能力（包含 defaults + self_mode 的硬禁用）
- 保持 `/admin/settings` 不被 gate（救援通道）

## 架构决策 ADR

### ADR-006: 分离 UI 禁用与数据面语义策略
**上下文:** 现有 `feature_disable_*` 更像“入口裁剪”，无法表达“禁用=语义切换”；直接复用会导致误操作风险（隐藏页面可能意外改变计费策略）。
**决策:** 新增 `policy_*` 作为数据面语义开关；`feature_disable_*` 继续只负责 UI/404。
**理由:** 语义更清晰、误触风险更低、可扩展到更多策略域。
**替代方案:** 复用 `feature_disable_*` 直接驱动数据面语义 → 拒绝原因: UI 操作语义不够明确，风险过高。
**影响:** 需要新增少量配置键与实现逻辑；文档需要明确“feature vs policy”的边界。

## API 设计

（可选）在 `/admin/settings` 增加一组“数据面策略”开关：
- Free mode
- Model passthrough
- Chat passthrough（若纳入本轮）

> 不影响现有 API 路径；仅改变策略开启时的数据面行为。

## 数据模型

### 1) app_settings 新增 keys

- `policy_free_mode`（bool）
- `policy_model_passthrough`（bool）
- （可选）`policy_chat_passthrough`（bool）

### 2) usage_events 新增字段（区分 payg vs free）

```sql
ALTER TABLE `usage_events`
  ADD COLUMN `balance_reserved_usd` DECIMAL(20,6) NOT NULL DEFAULT 0 AFTER `reserved_usd`,
  ADD KEY `idx_usage_events_state_reserve_expires_balance` (`state`, `reserve_expires_at`, `balance_reserved_usd`);
```

## 安全与性能

- **安全:** 保持现有调度护栏（用户分组、渠道健康、SSRF 校验、SSE 写回后禁止 failover）；policy 仅放宽“计费/模型白名单”层面的限制，不绕过上游鉴权与 base_url 安全校验。
- **性能:** policy/feature 读取需要控制 DB 查询次数；优先在 Store 中做批量读取或轻量缓存（如必要）。

## 测试与部署

- **测试:**
  - `go test ./...`
  - 覆盖场景：free mode、模型穿透、feature gate defaults、生效优先级（DB > defaults > hard override）
- **部署:**
  - 先执行 DB migration（新增列）
  - 再滚动发布服务（新版本开始写入/读取新字段与 policy keys）

