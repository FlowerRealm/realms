-- 0031_billing_decimal.sql: 计费金额字段改用小数（可重复执行）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“部分列已改但 schema_migrations 未记录”的状态。
-- 本迁移使用 information_schema 做存在性判断，允许重复执行以完成剩余步骤。

-- user_balances.usd_micros -> user_balances.usd
SET @has_user_balances_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='user_balances' AND COLUMN_NAME='usd_micros'
);
SET @has_user_balances_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='user_balances' AND COLUMN_NAME='usd'
);
SET @is_user_balances_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='user_balances' AND COLUMN_NAME='usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_user_balances_usd_micros>0 AND @has_user_balances_usd=0,
  'ALTER TABLE `user_balances` CHANGE COLUMN `usd_micros` `usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
  IF(@has_user_balances_usd>0,
    'ALTER TABLE `user_balances` MODIFY COLUMN `usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_user_balances_usd_micros>0 AND @has_user_balances_usd=0) OR @is_user_balances_usd_int>0,
  'UPDATE `user_balances` SET `usd` = `usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- usage_events.reserved_usd_micros -> usage_events.reserved_usd
SET @has_usage_events_reserved_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='usage_events' AND COLUMN_NAME='reserved_usd_micros'
);
SET @has_usage_events_reserved_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='usage_events' AND COLUMN_NAME='reserved_usd'
);
SET @is_usage_events_reserved_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='usage_events' AND COLUMN_NAME='reserved_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_usage_events_reserved_usd_micros>0 AND @has_usage_events_reserved_usd=0,
  'ALTER TABLE `usage_events` CHANGE COLUMN `reserved_usd_micros` `reserved_usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
  IF(@has_usage_events_reserved_usd>0,
    'ALTER TABLE `usage_events` MODIFY COLUMN `reserved_usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_usage_events_reserved_usd_micros>0 AND @has_usage_events_reserved_usd=0) OR @is_usage_events_reserved_usd_int>0,
  'UPDATE `usage_events` SET `reserved_usd` = `reserved_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- usage_events.committed_usd_micros -> usage_events.committed_usd
SET @has_usage_events_committed_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='usage_events' AND COLUMN_NAME='committed_usd_micros'
);
SET @has_usage_events_committed_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='usage_events' AND COLUMN_NAME='committed_usd'
);
SET @is_usage_events_committed_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='usage_events' AND COLUMN_NAME='committed_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_usage_events_committed_usd_micros>0 AND @has_usage_events_committed_usd=0,
  'ALTER TABLE `usage_events` CHANGE COLUMN `committed_usd_micros` `committed_usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
  IF(@has_usage_events_committed_usd>0,
    'ALTER TABLE `usage_events` MODIFY COLUMN `committed_usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_usage_events_committed_usd_micros>0 AND @has_usage_events_committed_usd=0) OR @is_usage_events_committed_usd_int>0,
  'UPDATE `usage_events` SET `committed_usd` = `committed_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans.price_cny_fen -> subscription_plans.price_cny
SET @has_subscription_plans_price_cny_fen := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='price_cny_fen'
);
SET @has_subscription_plans_price_cny := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='price_cny'
);
SET @is_subscription_plans_price_cny_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='price_cny'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_subscription_plans_price_cny_fen>0 AND @has_subscription_plans_price_cny=0,
  'ALTER TABLE `subscription_plans` CHANGE COLUMN `price_cny_fen` `price_cny` DECIMAL(20,2) NOT NULL',
  IF(@has_subscription_plans_price_cny>0,
    'ALTER TABLE `subscription_plans` MODIFY COLUMN `price_cny` DECIMAL(20,2) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_subscription_plans_price_cny_fen>0 AND @has_subscription_plans_price_cny=0) OR @is_subscription_plans_price_cny_int>0,
  'UPDATE `subscription_plans` SET `price_cny` = `price_cny` / 100',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans.limit_5h_usd_micros -> subscription_plans.limit_5h_usd
SET @has_subscription_plans_limit_5h_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_5h_usd_micros'
);
SET @has_subscription_plans_limit_5h_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_5h_usd'
);
SET @is_subscription_plans_limit_5h_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_5h_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_subscription_plans_limit_5h_usd_micros>0 AND @has_subscription_plans_limit_5h_usd=0,
  'ALTER TABLE `subscription_plans` CHANGE COLUMN `limit_5h_usd_micros` `limit_5h_usd` DECIMAL(20,6) NOT NULL',
  IF(@has_subscription_plans_limit_5h_usd>0,
    'ALTER TABLE `subscription_plans` MODIFY COLUMN `limit_5h_usd` DECIMAL(20,6) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_subscription_plans_limit_5h_usd_micros>0 AND @has_subscription_plans_limit_5h_usd=0) OR @is_subscription_plans_limit_5h_usd_int>0,
  'UPDATE `subscription_plans` SET `limit_5h_usd` = `limit_5h_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans.limit_1d_usd_micros -> subscription_plans.limit_1d_usd
SET @has_subscription_plans_limit_1d_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_1d_usd_micros'
);
SET @has_subscription_plans_limit_1d_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_1d_usd'
);
SET @is_subscription_plans_limit_1d_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_1d_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_subscription_plans_limit_1d_usd_micros>0 AND @has_subscription_plans_limit_1d_usd=0,
  'ALTER TABLE `subscription_plans` CHANGE COLUMN `limit_1d_usd_micros` `limit_1d_usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
  IF(@has_subscription_plans_limit_1d_usd>0,
    'ALTER TABLE `subscription_plans` MODIFY COLUMN `limit_1d_usd` DECIMAL(20,6) NOT NULL DEFAULT 0',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_subscription_plans_limit_1d_usd_micros>0 AND @has_subscription_plans_limit_1d_usd=0) OR @is_subscription_plans_limit_1d_usd_int>0,
  'UPDATE `subscription_plans` SET `limit_1d_usd` = `limit_1d_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans.limit_7d_usd_micros -> subscription_plans.limit_7d_usd
SET @has_subscription_plans_limit_7d_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_7d_usd_micros'
);
SET @has_subscription_plans_limit_7d_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_7d_usd'
);
SET @is_subscription_plans_limit_7d_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_7d_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_subscription_plans_limit_7d_usd_micros>0 AND @has_subscription_plans_limit_7d_usd=0,
  'ALTER TABLE `subscription_plans` CHANGE COLUMN `limit_7d_usd_micros` `limit_7d_usd` DECIMAL(20,6) NOT NULL',
  IF(@has_subscription_plans_limit_7d_usd>0,
    'ALTER TABLE `subscription_plans` MODIFY COLUMN `limit_7d_usd` DECIMAL(20,6) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_subscription_plans_limit_7d_usd_micros>0 AND @has_subscription_plans_limit_7d_usd=0) OR @is_subscription_plans_limit_7d_usd_int>0,
  'UPDATE `subscription_plans` SET `limit_7d_usd` = `limit_7d_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans.limit_30d_usd_micros -> subscription_plans.limit_30d_usd
SET @has_subscription_plans_limit_30d_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_30d_usd_micros'
);
SET @has_subscription_plans_limit_30d_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_30d_usd'
);
SET @is_subscription_plans_limit_30d_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_plans' AND COLUMN_NAME='limit_30d_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_subscription_plans_limit_30d_usd_micros>0 AND @has_subscription_plans_limit_30d_usd=0,
  'ALTER TABLE `subscription_plans` CHANGE COLUMN `limit_30d_usd_micros` `limit_30d_usd` DECIMAL(20,6) NOT NULL',
  IF(@has_subscription_plans_limit_30d_usd>0,
    'ALTER TABLE `subscription_plans` MODIFY COLUMN `limit_30d_usd` DECIMAL(20,6) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_subscription_plans_limit_30d_usd_micros>0 AND @has_subscription_plans_limit_30d_usd=0) OR @is_subscription_plans_limit_30d_usd_int>0,
  'UPDATE `subscription_plans` SET `limit_30d_usd` = `limit_30d_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_orders.amount_cny_fen -> subscription_orders.amount_cny
SET @has_subscription_orders_amount_cny_fen := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_orders' AND COLUMN_NAME='amount_cny_fen'
);
SET @has_subscription_orders_amount_cny := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_orders' AND COLUMN_NAME='amount_cny'
);
SET @is_subscription_orders_amount_cny_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='subscription_orders' AND COLUMN_NAME='amount_cny'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_subscription_orders_amount_cny_fen>0 AND @has_subscription_orders_amount_cny=0,
  'ALTER TABLE `subscription_orders` CHANGE COLUMN `amount_cny_fen` `amount_cny` DECIMAL(20,2) NOT NULL',
  IF(@has_subscription_orders_amount_cny>0,
    'ALTER TABLE `subscription_orders` MODIFY COLUMN `amount_cny` DECIMAL(20,2) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_subscription_orders_amount_cny_fen>0 AND @has_subscription_orders_amount_cny=0) OR @is_subscription_orders_amount_cny_int>0,
  'UPDATE `subscription_orders` SET `amount_cny` = `amount_cny` / 100',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- topup_orders.amount_cny_fen -> topup_orders.amount_cny
SET @has_topup_orders_amount_cny_fen := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='topup_orders' AND COLUMN_NAME='amount_cny_fen'
);
SET @has_topup_orders_amount_cny := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='topup_orders' AND COLUMN_NAME='amount_cny'
);
SET @is_topup_orders_amount_cny_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='topup_orders' AND COLUMN_NAME='amount_cny'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_topup_orders_amount_cny_fen>0 AND @has_topup_orders_amount_cny=0,
  'ALTER TABLE `topup_orders` CHANGE COLUMN `amount_cny_fen` `amount_cny` DECIMAL(20,2) NOT NULL',
  IF(@has_topup_orders_amount_cny>0,
    'ALTER TABLE `topup_orders` MODIFY COLUMN `amount_cny` DECIMAL(20,2) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_topup_orders_amount_cny_fen>0 AND @has_topup_orders_amount_cny=0) OR @is_topup_orders_amount_cny_int>0,
  'UPDATE `topup_orders` SET `amount_cny` = `amount_cny` / 100',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- topup_orders.credit_usd_micros -> topup_orders.credit_usd
SET @has_topup_orders_credit_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='topup_orders' AND COLUMN_NAME='credit_usd_micros'
);
SET @has_topup_orders_credit_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='topup_orders' AND COLUMN_NAME='credit_usd'
);
SET @is_topup_orders_credit_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='topup_orders' AND COLUMN_NAME='credit_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_topup_orders_credit_usd_micros>0 AND @has_topup_orders_credit_usd=0,
  'ALTER TABLE `topup_orders` CHANGE COLUMN `credit_usd_micros` `credit_usd` DECIMAL(20,6) NOT NULL',
  IF(@has_topup_orders_credit_usd>0,
    'ALTER TABLE `topup_orders` MODIFY COLUMN `credit_usd` DECIMAL(20,6) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_topup_orders_credit_usd_micros>0 AND @has_topup_orders_credit_usd=0) OR @is_topup_orders_credit_usd_int>0,
  'UPDATE `topup_orders` SET `credit_usd` = `credit_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- managed_models.*_usd_micros_per_1m -> managed_models.*_usd_per_1m
SET @has_managed_models_input_usd_micros_per_1m := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='input_usd_micros_per_1m'
);
SET @has_managed_models_input_usd_per_1m := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='input_usd_per_1m'
);
SET @is_managed_models_input_usd_per_1m_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='input_usd_per_1m'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_managed_models_input_usd_micros_per_1m>0 AND @has_managed_models_input_usd_per_1m=0,
  'ALTER TABLE `managed_models` CHANGE COLUMN `input_usd_micros_per_1m` `input_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000',
  IF(@has_managed_models_input_usd_per_1m>0,
    'ALTER TABLE `managed_models` MODIFY COLUMN `input_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_managed_models_input_usd_micros_per_1m>0 AND @has_managed_models_input_usd_per_1m=0) OR @is_managed_models_input_usd_per_1m_int>0,
  'UPDATE `managed_models` SET `input_usd_per_1m` = `input_usd_per_1m` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_managed_models_output_usd_micros_per_1m := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='output_usd_micros_per_1m'
);
SET @has_managed_models_output_usd_per_1m := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='output_usd_per_1m'
);
SET @is_managed_models_output_usd_per_1m_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='output_usd_per_1m'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_managed_models_output_usd_micros_per_1m>0 AND @has_managed_models_output_usd_per_1m=0,
  'ALTER TABLE `managed_models` CHANGE COLUMN `output_usd_micros_per_1m` `output_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 15.000000',
  IF(@has_managed_models_output_usd_per_1m>0,
    'ALTER TABLE `managed_models` MODIFY COLUMN `output_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 15.000000',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_managed_models_output_usd_micros_per_1m>0 AND @has_managed_models_output_usd_per_1m=0) OR @is_managed_models_output_usd_per_1m_int>0,
  'UPDATE `managed_models` SET `output_usd_per_1m` = `output_usd_per_1m` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_managed_models_cache_usd_micros_per_1m := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='cache_usd_micros_per_1m'
);
SET @has_managed_models_cache_usd_per_1m := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='cache_usd_per_1m'
);
SET @is_managed_models_cache_usd_per_1m_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='managed_models' AND COLUMN_NAME='cache_usd_per_1m'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_managed_models_cache_usd_micros_per_1m>0 AND @has_managed_models_cache_usd_per_1m=0,
  'ALTER TABLE `managed_models` CHANGE COLUMN `cache_usd_micros_per_1m` `cache_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000',
  IF(@has_managed_models_cache_usd_per_1m>0,
    'ALTER TABLE `managed_models` MODIFY COLUMN `cache_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_managed_models_cache_usd_micros_per_1m>0 AND @has_managed_models_cache_usd_per_1m=0) OR @is_managed_models_cache_usd_per_1m_int>0,
  'UPDATE `managed_models` SET `cache_usd_per_1m` = `cache_usd_per_1m` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- channel_groups.price_multiplier_micros -> channel_groups.price_multiplier
SET @has_channel_groups_price_multiplier_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='channel_groups' AND COLUMN_NAME='price_multiplier_micros'
);
SET @has_channel_groups_price_multiplier := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='channel_groups' AND COLUMN_NAME='price_multiplier'
);
SET @is_channel_groups_price_multiplier_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='channel_groups' AND COLUMN_NAME='price_multiplier'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_channel_groups_price_multiplier_micros>0 AND @has_channel_groups_price_multiplier=0,
  'ALTER TABLE `channel_groups` CHANGE COLUMN `price_multiplier_micros` `price_multiplier` DECIMAL(25,6) NOT NULL DEFAULT 1.000000',
  IF(@has_channel_groups_price_multiplier>0,
    'ALTER TABLE `channel_groups` MODIFY COLUMN `price_multiplier` DECIMAL(25,6) NOT NULL DEFAULT 1.000000',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_channel_groups_price_multiplier_micros>0 AND @has_channel_groups_price_multiplier=0) OR @is_channel_groups_price_multiplier_int>0,
  'UPDATE `channel_groups` SET `price_multiplier` = `price_multiplier` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- codex_oauth_accounts.balance_total_granted_usd_micros -> codex_oauth_accounts.balance_total_granted_usd
SET @has_codex_oauth_accounts_balance_total_granted_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_granted_usd_micros'
);
SET @has_codex_oauth_accounts_balance_total_granted_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_granted_usd'
);
SET @is_codex_oauth_accounts_balance_total_granted_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_granted_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_codex_oauth_accounts_balance_total_granted_usd_micros>0 AND @has_codex_oauth_accounts_balance_total_granted_usd=0,
  'ALTER TABLE `codex_oauth_accounts` CHANGE COLUMN `balance_total_granted_usd_micros` `balance_total_granted_usd` DECIMAL(20,6) NULL',
  IF(@has_codex_oauth_accounts_balance_total_granted_usd>0,
    'ALTER TABLE `codex_oauth_accounts` MODIFY COLUMN `balance_total_granted_usd` DECIMAL(20,6) NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_codex_oauth_accounts_balance_total_granted_usd_micros>0 AND @has_codex_oauth_accounts_balance_total_granted_usd=0) OR @is_codex_oauth_accounts_balance_total_granted_usd_int>0,
  'UPDATE `codex_oauth_accounts` SET `balance_total_granted_usd` = `balance_total_granted_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- codex_oauth_accounts.balance_total_used_usd_micros -> codex_oauth_accounts.balance_total_used_usd
SET @has_codex_oauth_accounts_balance_total_used_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_used_usd_micros'
);
SET @has_codex_oauth_accounts_balance_total_used_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_used_usd'
);
SET @is_codex_oauth_accounts_balance_total_used_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_used_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_codex_oauth_accounts_balance_total_used_usd_micros>0 AND @has_codex_oauth_accounts_balance_total_used_usd=0,
  'ALTER TABLE `codex_oauth_accounts` CHANGE COLUMN `balance_total_used_usd_micros` `balance_total_used_usd` DECIMAL(20,6) NULL',
  IF(@has_codex_oauth_accounts_balance_total_used_usd>0,
    'ALTER TABLE `codex_oauth_accounts` MODIFY COLUMN `balance_total_used_usd` DECIMAL(20,6) NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_codex_oauth_accounts_balance_total_used_usd_micros>0 AND @has_codex_oauth_accounts_balance_total_used_usd=0) OR @is_codex_oauth_accounts_balance_total_used_usd_int>0,
  'UPDATE `codex_oauth_accounts` SET `balance_total_used_usd` = `balance_total_used_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- codex_oauth_accounts.balance_total_available_usd_micros -> codex_oauth_accounts.balance_total_available_usd
SET @has_codex_oauth_accounts_balance_total_available_usd_micros := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_available_usd_micros'
);
SET @has_codex_oauth_accounts_balance_total_available_usd := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_available_usd'
);
SET @is_codex_oauth_accounts_balance_total_available_usd_int := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='codex_oauth_accounts' AND COLUMN_NAME='balance_total_available_usd'
    AND DATA_TYPE IN ('tinyint','smallint','mediumint','int','bigint')
);
SET @sql := IF(@has_codex_oauth_accounts_balance_total_available_usd_micros>0 AND @has_codex_oauth_accounts_balance_total_available_usd=0,
  'ALTER TABLE `codex_oauth_accounts` CHANGE COLUMN `balance_total_available_usd_micros` `balance_total_available_usd` DECIMAL(20,6) NULL',
  IF(@has_codex_oauth_accounts_balance_total_available_usd>0,
    'ALTER TABLE `codex_oauth_accounts` MODIFY COLUMN `balance_total_available_usd` DECIMAL(20,6) NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((@has_codex_oauth_accounts_balance_total_available_usd_micros>0 AND @has_codex_oauth_accounts_balance_total_available_usd=0) OR @is_codex_oauth_accounts_balance_total_available_usd_int>0,
  'UPDATE `codex_oauth_accounts` SET `balance_total_available_usd` = `balance_total_available_usd` / 1000000',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
