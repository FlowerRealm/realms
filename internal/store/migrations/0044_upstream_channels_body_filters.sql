-- 0044_upstream_channels_body_filters.sql: upstream_channels 增加请求体黑白名单与模型后缀保护名单（参考 new-api）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'model_suffix_preserve'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `model_suffix_preserve` TEXT NULL AFTER `status_code_mapping`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'request_body_blacklist'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `request_body_blacklist` TEXT NULL AFTER `model_suffix_preserve`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'request_body_whitelist'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `request_body_whitelist` TEXT NULL AFTER `request_body_blacklist`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

