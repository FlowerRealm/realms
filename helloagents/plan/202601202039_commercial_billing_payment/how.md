# 技术设计: 商业版计费与支付闭环（Stripe / Epay）

## 技术方案

### 核心技术
- MySQL 事务（订单状态与入账必须在同一事务内完成）
- Webhook 安全（Stripe 官方签名验证；Epay 使用签名/密钥校验）
- 幂等语义（事件幂等表 + 订单幂等字段）

### 实现要点
- **幂等优先:** 任何“入账/开通订阅”都必须先判断是否已处理（基于外部事件 ID/支付 ref/订单状态）。
- **状态机明确:** 订单状态转换单向、可恢复，禁止隐式跨状态跳转。
- **余额流水化:** `user_balances` 只作为缓存/快照；新增 `balance_ledger`（或等价）记录每笔变动，保证可追溯与可对账。
- **回调入口鉴权:** Stripe 使用 webhook secret；Epay 使用渠道密钥校验；同时支持按 payment_channel_id 读取独立配置。

## 架构决策 ADR

### ADR-007: 以流水总账作为余额 SSOT
**上下文:** 仅靠 `user_balances.usd` 直接加减难以对账与追溯，且在回调异常/补偿场景容易出现口径不一致。  
**决策:** 新增余额流水表作为 SSOT，所有余额变动写流水，`user_balances` 作为聚合快照（可重建）。  
**理由:** 财务正确性可验证；便于审计、对账、回滚与补偿。  
**替代方案:** 继续只更新 `user_balances` → 拒绝原因: 无法解释与审计，长期运营必出事故。  
**影响:** 需要补齐迁移、回填与管理导出接口；测试成本增加但必要。

## API 设计（关键路径）

### 支付回调（现有路径增强）
- **[POST]** `/api/pay/stripe/webhook` / `/api/pay/stripe/webhook/{payment_channel_id}`
  - 处理流程：验签 → 提取 event_id → 幂等检查 → 关联订单 → 入账/开通订阅（事务）→ 记录事件处理结果
- **[GET]** `/api/pay/epay/notify` / `/api/pay/epay/notify/{payment_channel_id}`
  - 处理流程：校验签名 → 提取 pay_ref → 幂等检查 → 关联订单 → 入账/开通订阅（事务）

## 数据模型（计划）

```sql
-- 余额流水（示意，最终以实现为准）
-- balance_ledger(id, user_id, kind, ref_type, ref_id, delta_usd, created_at, unique(ref_type, ref_id))
-- webhook_events(id, provider, event_id, processed_at, unique(provider, event_id))
```

## 安全与性能
- **安全:**
  - webhook 必须验签；拒绝未配置 secret 的回调
  - 幂等表避免重复入账；关键字段加唯一约束
  - 支付渠道密钥与上游 token 建议加密存储（与自用方案的“导出脱敏”协同）
- **性能:**
  - 幂等表按 event_id/pay_ref 建索引；流水表按 user_id+time 建索引，支持导出与查询

## 测试与部署
- **测试:**
  - 回调重放/乱序/重复用例（Stripe event 重放、Epay notify 重复）
  - 订单取消后回调、订单已入账再回调等边界用例
  - 流水对账：`sum(delta)=user_balances`（允许异步聚合时做最终一致校验）
- **部署:**
  - Stripe/Epay secrets 必须通过管理后台或配置写入，且上线前验证 webhook secret 配置正确

