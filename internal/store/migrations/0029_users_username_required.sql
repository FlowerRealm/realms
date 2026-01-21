-- 0029_users_username_required.sql: username 改为必填（回填存量 NULL/空串，并设置 NOT NULL）。

-- 回填缺失账号名：使用 u{id}_{md5} 形式（满足校验规则，且尽量避免与存量冲突）。
UPDATE users
SET username = CONCAT('u', id, '_', SUBSTRING(MD5(email), 1, 6))
WHERE username IS NULL OR username = '';

-- 将 username 设为 NOT NULL（已是 NOT NULL 时跳过）。
SELECT IF(
  EXISTS(
    SELECT 1
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'users'
      AND COLUMN_NAME = 'username'
      AND IS_NULLABLE = 'YES'
  ),
  'ALTER TABLE `users` MODIFY COLUMN `username` VARCHAR(64) NOT NULL',
  'SELECT 1'
) INTO @realms_stmt_users_username_not_null;

PREPARE realms_stmt_users_username_not_null FROM @realms_stmt_users_username_not_null;
EXECUTE realms_stmt_users_username_not_null;
DEALLOCATE PREPARE realms_stmt_users_username_not_null;

-- 兜底：确保唯一索引存在（历史环境可能缺失）。
SELECT IF(
  EXISTS(
    SELECT 1
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'users'
      AND INDEX_NAME = 'uk_users_username'
  ),
  'SELECT 1',
  'ALTER TABLE `users` ADD UNIQUE KEY `uk_users_username` (`username`)'
) INTO @realms_stmt_add_users_username_uk;

PREPARE realms_stmt_add_users_username_uk FROM @realms_stmt_add_users_username_uk;
EXECUTE realms_stmt_add_users_username_uk;
DEALLOCATE PREPARE realms_stmt_add_users_username_uk;

