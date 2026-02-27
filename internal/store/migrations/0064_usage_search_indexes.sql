-- 0064_usage_search_indexes.sql: 为 usage 查询过滤补齐索引（user_tokens.name / usage_events token_id, upstream_channel_id）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列/索引已创建但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对索引是否存在做条件判断。

SET @idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE table_schema = DATABASE()
    AND table_name = 'user_tokens'
    AND index_name = 'idx_user_tokens_user_id_name'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_user_tokens_user_id_name` ON `user_tokens` (`user_id`, `name`)',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND index_name = 'idx_usage_events_token_time_id'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_usage_events_token_time_id` ON `usage_events` (`token_id`, `time`, `id`)',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND index_name = 'idx_usage_events_upstream_channel_time_id'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_usage_events_upstream_channel_time_id` ON `usage_events` (`upstream_channel_id`, `time`, `id`)',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

