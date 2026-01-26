-- 0046_usage_event_details_downstream_request_body.sql: usage_event_details 增加下游原始请求体字段（用于排障对比）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_event_details'
    AND column_name = 'downstream_request_body'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_event_details` ADD COLUMN `downstream_request_body` MEDIUMTEXT NULL AFTER `usage_event_id`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
