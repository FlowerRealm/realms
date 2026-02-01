-- 0049_user_tokens_plain.sql: user_tokens 增加 token_plain（用于 Web 控制台查看/复制 token）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已加上但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'user_tokens'
    AND column_name = 'token_plain'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `user_tokens` ADD COLUMN `token_plain` VARCHAR(255) NULL AFTER `token_hash`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

