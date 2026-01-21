-- 0027_topup_orders.sql: 充值订单（支付成功后增加 user_balances）。

CREATE TABLE IF NOT EXISTS `topup_orders` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `amount_cny` DECIMAL(20,2) NOT NULL,
  `credit_usd` DECIMAL(20,6) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 0,
  `paid_at` DATETIME NULL,
  `paid_method` VARCHAR(32) NULL,
  `paid_ref` VARCHAR(128) NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_topup_orders_user_id_id` (`user_id`, `id`),
  KEY `idx_topup_orders_status_id` (`status`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
