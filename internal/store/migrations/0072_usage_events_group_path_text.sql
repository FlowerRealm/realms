SET @col_type := (
  SELECT DATA_TYPE
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'price_multiplier_group_name'
  LIMIT 1
);
SET @ddl := IF(
  @col_type IS NOT NULL AND @col_type <> 'text',
  'ALTER TABLE `usage_events` MODIFY COLUMN `price_multiplier_group_name` TEXT NULL',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
