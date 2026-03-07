-- 0067_managed_models_priority_pricing.sql: managed_models 增加 fast mode（priority）显式开关与价格字段。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND column_name = 'priority_pricing_enabled'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `managed_models` ADD COLUMN `priority_pricing_enabled` TINYINT NOT NULL DEFAULT 0 AFTER `cache_output_usd_per_1m`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND column_name = 'priority_input_usd_per_1m'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `managed_models` ADD COLUMN `priority_input_usd_per_1m` DECIMAL(20,6) NULL AFTER `priority_pricing_enabled`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND column_name = 'priority_output_usd_per_1m'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `managed_models` ADD COLUMN `priority_output_usd_per_1m` DECIMAL(20,6) NULL AFTER `priority_input_usd_per_1m`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND column_name = 'priority_cache_input_usd_per_1m'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `managed_models` ADD COLUMN `priority_cache_input_usd_per_1m` DECIMAL(20,6) NULL AFTER `priority_output_usd_per_1m`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `managed_models`
SET `priority_pricing_enabled` = 0
WHERE `priority_pricing_enabled` IS NULL;
