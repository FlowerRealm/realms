-- 0048_upstream_channels_newapi_settings.sql: upstream_channels 增加 new-api 对齐字段（openai_organization/test_model/tag/remark/weight/auto_ban/setting）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'openai_organization'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `openai_organization` VARCHAR(128) NULL AFTER `allow_safety_identifier`',
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
    AND column_name = 'test_model'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `test_model` VARCHAR(128) NULL AFTER `openai_organization`',
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
    AND column_name = 'tag'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `tag` VARCHAR(64) NULL AFTER `test_model`',
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
    AND column_name = 'remark'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `remark` VARCHAR(255) NULL AFTER `tag`',
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
    AND column_name = 'weight'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `weight` INT NOT NULL DEFAULT 0 AFTER `remark`',
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
    AND column_name = 'auto_ban'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `auto_ban` TINYINT NOT NULL DEFAULT 1 AFTER `weight`',
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
    AND column_name = 'setting'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `setting` TEXT NULL AFTER `auto_ban`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

