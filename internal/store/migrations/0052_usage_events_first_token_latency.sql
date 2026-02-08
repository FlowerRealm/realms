-- 0052_usage_events_first_token_latency.sql: usage_events 增加 first_token_latency_ms（首字延迟）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'first_token_latency_ms'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_events` ADD COLUMN `first_token_latency_ms` INT NOT NULL DEFAULT 0 AFTER `latency_ms`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
