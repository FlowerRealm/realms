-- 0069_upstream_channels_allow_service_tier_default.sql: 默认允许透传 service_tier，并回填与 fast_mode 冲突的历史数据。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'allow_service_tier'
);
SET @ddl := IF(
  @col_exists = 1,
  'ALTER TABLE `upstream_channels` MODIFY COLUMN `allow_service_tier` TINYINT NOT NULL DEFAULT 1',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `upstream_channels`
SET `allow_service_tier` = 1
WHERE `fast_mode` = 1 AND `allow_service_tier` = 0;
