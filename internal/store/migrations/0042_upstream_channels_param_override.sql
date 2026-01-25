-- 0042_upstream_channels_param_override.sql: upstream_channels 增加 param_override（按渠道参数改写规则）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'param_override'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `upstream_channels` ADD COLUMN `param_override` TEXT NULL AFTER `allow_safety_identifier`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

