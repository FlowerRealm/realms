-- 0041_upstream_channels_request_policy.sql: upstream_channels 增加请求字段过滤策略（按渠道）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'allow_service_tier'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `allow_service_tier` TINYINT NOT NULL DEFAULT 0 AFTER `promotion`',
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
    AND column_name = 'disable_store'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `disable_store` TINYINT NOT NULL DEFAULT 0 AFTER `allow_service_tier`',
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
    AND column_name = 'allow_safety_identifier'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `allow_safety_identifier` TINYINT NOT NULL DEFAULT 0 AFTER `disable_store`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
