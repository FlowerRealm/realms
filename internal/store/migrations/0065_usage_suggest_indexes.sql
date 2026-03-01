-- 0065_usage_suggest_indexes.sql: 为 admin usage suggest 查询补齐索引（usage_events.time -> channel/model，upstream_channels.name）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列/索引已创建但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对索引是否存在做条件判断。

SET @idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND index_name = 'idx_upstream_channels_name'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_upstream_channels_name` ON `upstream_channels` (`name`)',
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
    AND index_name = 'idx_usage_events_time_upstream_channel_id'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_usage_events_time_upstream_channel_id` ON `usage_events` (`time`, `upstream_channel_id`, `id`)',
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
    AND index_name = 'idx_usage_events_time_model_id'
);
SET @ddl := IF(
  @idx_exists = 0,
  'CREATE INDEX `idx_usage_events_time_model_id` ON `usage_events` (`time`, `model`, `id`)',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

