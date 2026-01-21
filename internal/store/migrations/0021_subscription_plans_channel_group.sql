-- 0021_subscription_plans_channel_group.sql: subscription_plans 增加渠道分组字段（用于订阅与调度分组绑定）。

ALTER TABLE `subscription_plans`
  ADD COLUMN `channel_group` VARCHAR(64) NOT NULL DEFAULT 'default' AFTER `name`;

