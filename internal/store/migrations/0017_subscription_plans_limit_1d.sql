-- 0017_subscription_plans_limit_1d.sql: subscription_plans 增加 1d 窗口限额（USD 小数）。

ALTER TABLE `subscription_plans`
  ADD COLUMN `limit_1d_usd` DECIMAL(20,6) NOT NULL DEFAULT 0 AFTER `limit_5h_usd`;
