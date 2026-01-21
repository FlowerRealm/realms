-- 0024_subscription_orders.sql: 订阅订单（购买先创建订单，支付/批准后生效）。

CREATE TABLE IF NOT EXISTS `subscription_orders` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `plan_id` BIGINT NOT NULL,
  `amount_cny` DECIMAL(20,2) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 0,
  `paid_at` DATETIME NULL,
  `paid_method` VARCHAR(32) NULL,
  `paid_ref` VARCHAR(128) NULL,
  `approved_at` DATETIME NULL,
  `approved_by` BIGINT NULL,
  `subscription_id` BIGINT NULL,
  `note` VARCHAR(255) NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_subscription_orders_user_id_id` (`user_id`, `id`),
  KEY `idx_subscription_orders_status_id` (`status`, `id`),
  KEY `idx_subscription_orders_subscription_id` (`subscription_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
