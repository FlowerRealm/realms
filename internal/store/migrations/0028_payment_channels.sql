-- 0028_payment_channels.sql: 支付渠道（每个渠道一份配置）+ 订单记录支付渠道。

CREATE TABLE IF NOT EXISTS `payment_channels` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `type` VARCHAR(32) NOT NULL,
  `name` VARCHAR(64) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,

  -- stripe 配置
  `stripe_currency` VARCHAR(16) NULL,
  `stripe_secret_key` VARCHAR(255) NULL,
  `stripe_webhook_secret` VARCHAR(255) NULL,

  -- epay 配置
  `epay_gateway` VARCHAR(255) NULL,
  `epay_partner_id` VARCHAR(64) NULL,
  `epay_key` VARCHAR(255) NULL,

  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE `subscription_orders`
  ADD COLUMN `paid_channel_id` BIGINT NULL AFTER `paid_ref`;

ALTER TABLE `topup_orders`
  ADD COLUMN `paid_channel_id` BIGINT NULL AFTER `paid_ref`;
