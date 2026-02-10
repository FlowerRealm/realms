-- 0051_managed_models_group_name.sql: managed_models 增加 group_name，用于按用户组控制模型可见性。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列/索引已创建但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列与索引是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND column_name = 'group_name'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `managed_models` ADD COLUMN `group_name` VARCHAR(64) NOT NULL DEFAULT '''' AFTER `public_id`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE managed_models
SET group_name = ''
WHERE group_name IS NULL OR TRIM(group_name) = '';

SET @idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE table_schema = DATABASE()
    AND table_name = 'managed_models'
    AND index_name = 'idx_managed_models_status_group'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_managed_models_status_group` ON `managed_models` (`status`, `group_name`)',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
