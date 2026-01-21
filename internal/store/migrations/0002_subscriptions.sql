-- 0002_subscriptions.sql: 订阅套餐与用户订阅（按 USD 小数金额计费/限额）。

CREATE TABLE IF NOT EXISTS `subscription_plans` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `code` VARCHAR(64) NOT NULL,
  `name` VARCHAR(128) NOT NULL,
  `price_cny` DECIMAL(20,2) NOT NULL,
  `limit_5h_usd` DECIMAL(20,6) NOT NULL,
  `limit_7d_usd` DECIMAL(20,6) NOT NULL,
  `limit_30d_usd` DECIMAL(20,6) NOT NULL,
  `duration_days` INT NOT NULL DEFAULT 30,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_subscription_plans_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `user_subscriptions` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `plan_id` BIGINT NOT NULL,
  `start_at` DATETIME NOT NULL,
  `end_at` DATETIME NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_user_subscriptions_user` (`user_id`),
  KEY `idx_user_subscriptions_end_at` (`end_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 默认套餐：¥12/月；5h=$6，7d=$20，30d=$80。
INSERT INTO subscription_plans(
  code, name, price_cny,
  limit_5h_usd, limit_7d_usd, limit_30d_usd,
  duration_days, status, created_at, updated_at
) VALUES(
  'basic_12', '基础订阅', 12.00,
  6.000000, 20.000000, 80.000000,
  30, 1, NOW(), NOW()
) AS new
ON DUPLICATE KEY UPDATE
  name=new.name,
  price_cny=new.price_cny,
  limit_5h_usd=new.limit_5h_usd,
  limit_7d_usd=new.limit_7d_usd,
  limit_30d_usd=new.limit_30d_usd,
  duration_days=new.duration_days,
  status=new.status,
  updated_at=NOW();
