-- 0061_drop_channel_groups_max_attempts.sql: 移除 channel_groups.max_attempts（历史组内 failover 上限）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已删除但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'channel_groups'
    AND column_name = 'max_attempts'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `channel_groups` DROP COLUMN `max_attempts`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

