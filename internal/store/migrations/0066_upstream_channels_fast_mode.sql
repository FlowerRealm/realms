-- 0066_upstream_channels_fast_mode.sql: upstream_channels 增加 fast_mode 能力位。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'fast_mode'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `fast_mode` TINYINT NOT NULL DEFAULT 1 AFTER `allow_service_tier`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
