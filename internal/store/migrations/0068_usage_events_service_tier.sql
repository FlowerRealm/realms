-- 0068_usage_events_service_tier.sql: usage_events 增加实际生效的 service_tier。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'service_tier'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_events` ADD COLUMN `service_tier` VARCHAR(32) NULL AFTER `model`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
