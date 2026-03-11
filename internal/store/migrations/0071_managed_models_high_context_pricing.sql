-- 0071_managed_models_high_context_pricing.sql: managed_models 增加高上下文分段定价 JSON 字段。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND column_name = 'high_context_pricing_json'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `managed_models` ADD COLUMN `high_context_pricing_json` JSON NULL AFTER `priority_cache_input_usd_per_1m`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
