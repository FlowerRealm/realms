-- 0018_users_username.sql: users 增加 username（可为空，用于登录），并建立唯一索引（允许多个 NULL）。

-- 兼容：避免重复执行导致迁移失败（部分环境可能已手动加列/索引）。
SELECT IF(
  EXISTS(
    SELECT 1
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'users'
      AND COLUMN_NAME = 'username'
  ),
  'SELECT 1',
  'ALTER TABLE `users` ADD COLUMN `username` VARCHAR(64) NULL AFTER `email`'
) INTO @realms_stmt_add_users_username;

PREPARE realms_stmt_add_users_username FROM @realms_stmt_add_users_username;
EXECUTE realms_stmt_add_users_username;
DEALLOCATE PREPARE realms_stmt_add_users_username;

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
