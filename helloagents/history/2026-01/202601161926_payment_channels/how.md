# 技术设计: 支付渠道化（按渠道配置 EPay/Stripe）

## 技术方案

### 核心技术
- Go 1.22（`net/http` + Go1.22 ServeMux 路由）
- MySQL（内置迁移）
- Stripe：`stripe-go`（Checkout Session + Webhook）
- EPay：`go-epay`（网关跳转 + notify 回调验签）

### 实现要点
- **新增支付渠道表 `payment_channels`：** 以渠道为主实体，`type` 决定所需配置字段。
- **用户侧支付改为“选渠道”：** `/pay/{kind}/{order_id}` 展示可用渠道；`/pay/.../start` 通过 `payment_channel_id` 发起支付。
- **回调按渠道验签：**
  - Stripe：新增 `POST /api/pay/stripe/webhook/{payment_channel_id}`，按渠道 `webhook_secret` 验签。
  - EPay：新增 `GET /api/pay/epay/notify/{payment_channel_id}`，按渠道 `gateway/partner_id/key` 验签。
- **兼容旧行为：** 当 `payment_channels` 为空时，继续使用现有 `app_settings.payment_*` 与旧回调地址（避免破坏现有部署）。
- **可追溯：** 订单表补充 `paid_channel_id`（或等价字段），记录“实际使用的支付渠道”。

## 架构设计（支付相关）

```mermaid
flowchart TD
    User[用户] --> PayPage[支付页 /pay/...]
    PayPage --> Start[POST /pay/.../start]
    Start --> StripeCheckout[Stripe Checkout]
    Start --> EPayGateway[EPay 网关]

    StripeCheckout --> StripeHook[POST /api/pay/stripe/webhook/{channel_id}]
    EPayGateway --> EPayNotify[GET /api/pay/epay/notify/{channel_id}]

    StripeHook --> Store[(MySQL)]
    EPayNotify --> Store
```

## 架构决策 ADR

### ADR-007: 支付配置从 app_settings 升级为 payment_channels 表
**上下文:** 现有支付配置为 `app_settings` 的全局键值对，只能表达单一 Stripe/EPay 实例；无法支持同类型多份配置与排序/启停。
**决策:** 引入 `payment_channels` 表作为支付配置 SSOT（渠道级），以 `type` 约束字段，并通过管理后台维护。
**替代方案:**
- 继续使用 `app_settings`，用 `payment.<channel>.<key>` 命名空间模拟多渠道 → 拒绝原因：不可维护、难以校验/查询、缺乏实体与约束。
- 仅通过 `config.yaml` 配置渠道列表 → 拒绝原因：无法在管理后台动态维护，不符合现有“界面覆盖优先”的使用习惯。
**影响:** 需要新增迁移与管理后台页面，但模型清晰，扩展性与可运维性更好。

## API设计（SSR + Webhook）

### 用户侧（需要登录）
- `GET /pay/{kind}/{order_id}`：支付页展示可用“支付渠道”
- `POST /pay/{kind}/{order_id}/start`：发起支付（新增参数 `payment_channel_id`；保留旧 `method` 兼容）

### 管理后台（仅 root）
- `GET /admin/payment-channels`：支付渠道列表/创建入口
- `POST /admin/payment-channels`：创建渠道（name/type/priority/status）
- `POST /admin/payment-channels/{payment_channel_id}`：更新渠道配置（含敏感字段）
- `POST /admin/payment-channels/{payment_channel_id}/delete`：删除渠道（仅允许删除非内置/无引用或软删除）

### 支付回调（无需登录）
- `POST /api/pay/stripe/webhook/{payment_channel_id}`：Stripe webhook（验签 + 幂等入账/生效）
- `GET /api/pay/epay/notify/{payment_channel_id}`：EPay notify（验签 + 幂等入账/生效；按网关约定返回 `success`/`fail`）
- 保留现有：
  - `POST /api/pay/stripe/webhook`
  - `GET /api/pay/epay/notify`
  用于“未迁移/旧配置”场景

## 数据模型

```sql
-- 支付渠道（按渠道存一份配置）
CREATE TABLE IF NOT EXISTS `payment_channels` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `type` VARCHAR(32) NOT NULL,
  `name` VARCHAR(64) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `priority` INT NOT NULL DEFAULT 0,

  -- stripe 配置
  `stripe_currency` VARCHAR(16) NULL,
  `stripe_secret_key` VARCHAR(255) NULL,
  `stripe_webhook_secret` VARCHAR(255) NULL,

  -- epay 配置
  `epay_gateway` VARCHAR(255) NULL,
  `epay_partner_id` VARCHAR(64) NULL,
  `epay_key` VARCHAR(255) NULL,

  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_payment_channels_type_status_priority` (`type`, `status`, `priority`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 订单记录支付渠道（追溯用；允许为空以兼容历史）
ALTER TABLE `subscription_orders`
  ADD COLUMN `paid_channel_id` BIGINT NULL;
ALTER TABLE `topup_orders`
  ADD COLUMN `paid_channel_id` BIGINT NULL;
```

## 安全与性能
- **安全:**
  - 回调按渠道验签；回调中校验金额/币种与本地订单快照一致
  - 敏感字段（secret/key/webhook_secret）不回显、不写日志
  - 后台仅 `root` 可配置；用户发起支付相关 POST 走 CSRF
  - 订单入账/生效走事务与幂等逻辑，避免重复入账
- **性能:**
  - 支付回调频率低；渠道配置读取可按需做短 TTL 缓存（非必须）

## 测试与部署
- **测试:** `go test ./...`；新增覆盖：支付渠道选择/回调路由/幂等更新
- **部署:**
  1. 升级后自动执行迁移
  2. 旧部署可继续使用原全局配置与旧回调 URL
  3. 如启用新支付渠道：在后台创建渠道并填入配置；Stripe 需在 Stripe 后台配置 webhook URL 为 `/api/pay/stripe/webhook/{payment_channel_id}`

