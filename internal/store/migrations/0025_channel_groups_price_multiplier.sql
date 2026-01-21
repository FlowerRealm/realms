-- 0025_channel_groups_price_multiplier.sql: channel_groups 增加价格倍率（默认 1.0）。

ALTER TABLE `channel_groups`
  ADD COLUMN `price_multiplier` DECIMAL(25,6) NOT NULL DEFAULT 1.000000 AFTER `description`;
