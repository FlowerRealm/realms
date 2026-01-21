-- 0026_user_balances.sql: 用户余额（用于按量计费）。

CREATE TABLE IF NOT EXISTS `user_balances` (
  `user_id` BIGINT PRIMARY KEY,
  `usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
