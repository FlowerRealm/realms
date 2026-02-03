# 变更提案: 自用模式（self_mode）+ 多上游管理硬化

## 需求背景

当前 Realms 已具备多上游（OpenAI 兼容 + Codex OAuth）接入、管理后台配置、OpenAI 风格北向 API 等能力，但“自用版”与“商业版”的边界仍需要进一步收敛与硬化：

- 自用版希望聚焦 **多服务商管理与稳定转发**，尽量减少计费/支付/工单等非必要域的暴露面与运维负担。
- 商业版需要更完整的计费与支付闭环（本提案不覆盖商业闭环细节，仅保证自用模式下相关功能彻底关闭）。

## 产品分析

### 目标用户与场景
- **用户群体:** 个人开发者 / 小团队自建中转
- **使用场景:** 统一管理多个上游 endpoint/credential（OpenAI 兼容 + Codex OAuth），为本地或小规模客户端提供稳定的 `/v1/*` 兼容接口
- **核心痛点:** 自用场景不需要支付与工单，但这些入口存在会增加风险与维护成本；同时多上游的可观测性与可维护性需要更清晰的“自用优先”默认策略

### 价值主张与成功指标
- **价值主张:** 一套代码同时支持“自用（默认）”与“商业（可选）”，自用形态最小暴露面且管理体验完整
- **成功指标:**
  - self_mode=true 时：计费/支付/工单相关入口与回调不可达（404），管理后台不展示相关菜单
  - 自用形态下：OpenAI 兼容与 Codex OAuth 上游可同时配置并稳定路由（含 failover 与粘性策略不被破坏）

### 人文关怀
自用模式默认减少对外暴露的功能入口，降低误操作与泄露风险；同时在导出/导入等管理能力中避免泄露敏感信息。

## 变更内容
1. 收敛“产品形态判定”为唯一来源：`self_mode.enable` + `feature_disable_*`（最终状态以 `FeatureStateEffective/FeatureDisabledEffective` 为准）
2. self_mode 下硬禁用计费/支付/工单域：不注册相关路由 + FeatureGateEffective 拒绝访问 + UI 隐藏入口
3. 多上游管理能力补齐自用体验：确保 OpenAI 兼容与 Codex OAuth 的管理、限流、健康与用量展示在自用形态可用且一致
4. 增加“自用优先”的导出/导入策略（仅导出非敏感配置或对敏感字段做安全处理），便于自建迁移与备份

## 影响范围
- **模块:**
  - `internal/server`（路由注册与功能隔离）
  - `internal/web`（用户控制台入口与页面）
  - `internal/admin`（管理后台入口与页面）
  - `internal/store`（feature gate 最终判定、上游配置读写与导出/导入）
  - `internal/scheduler` / `internal/upstream`（多上游选择与执行链路稳定性）
- **API:**
  - self_mode 下：`/subscription`、`/topup`、`/pay/*`、`/tickets*`、`/admin/*(billing|tickets)*`、`/api/pay/*`、`/api/webhooks/subscription-orders/*` 需不可达
  - 自用核心：`/v1/*` 与上游管理相关管理面路由保持可用
- **数据:**
  - 不引入新的计费数据模型变更（商业闭环在另一方案包中处理）
  - 可能引入“导出/导入”所需的配置序列化结构（以代码结构为准）

## 核心场景

### 需求: self_mode 强制关闭 billing/tickets
**模块:** server/web/admin/store

#### 场景: self_mode=true 时入口与回调不可达
- 访问 `/subscription`、`/topup`、`/pay/{kind}/{order_id}` 返回 404
- 访问 `/tickets*`、`/admin/orders`、`/admin/subscriptions`、`/admin/payment-channels`、`/admin/tickets*` 返回 404
- 访问 `/api/pay/stripe/webhook*`、`/api/pay/epay/notify*`、`/api/webhooks/subscription-orders/*/paid` 返回 404

### 需求: 多上游（OpenAI 兼容 + Codex OAuth）可并存可路由
**模块:** admin/scheduler/upstream/store

#### 场景: 同一用户 Token 可路由到不同上游类型
- 管理后台可分别配置 OpenAI 兼容与 Codex OAuth 的 endpoint 与凭据/账号
- 数据面请求可在不同 channel/group 下选择不同上游类型并成功转发

### 需求: 上游限流/粘性/健康具备可解释性
**模块:** scheduler/admin

#### 场景: 选择结果可解释且不破坏 SSE 约束
- 发生 failover 或被限流拒绝时，管理面可观察到原因（限流/冷却/失败分等）
- SSE 已写回后不再尝试 failover（保持现有 ADR）

## 风险评估
- **风险:** self_mode 关闭不彻底导致入口残留（支付回调/管理入口仍可访问）
  - **缓解:** 路由注册层 + FeatureGateEffective 双保险；补齐路由清单测试与人工核对表
- **风险:** 导出/导入泄露敏感信息（上游 key/token、支付密钥）
  - **缓解:** 默认不导出敏感字段；如需迁移，采用“二次填充”或加密导出文件策略（商业方案中再增强）

