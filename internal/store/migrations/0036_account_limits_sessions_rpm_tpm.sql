-- 0036_account_limits_sessions_rpm_tpm.sql: 账号维度限额（sessions/rpm/tpm）+ 渠道 limit_cc 改名为 limit_sessions。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已变更但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

-- 1) upstream_channels: limit_cc -> limit_sessions（语义迁移）
SET @has_limit_sessions := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_sessions'
);
SET @has_limit_cc := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_cc'
);
SET @ddl := IF(
  @has_limit_sessions = 0 AND @has_limit_cc > 0,
  'ALTER TABLE `upstream_channels` CHANGE COLUMN `limit_cc` `limit_sessions` INT NULL AFTER `promotion`',
  IF(
    @has_limit_sessions = 0 AND @has_limit_cc = 0,
    'ALTER TABLE `upstream_channels` ADD COLUMN `limit_sessions` INT NULL AFTER `promotion`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2) openai_compatible_credentials: add limit_sessions/limit_rpm/limit_tpm
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'openai_compatible_credentials'
    AND column_name = 'limit_sessions'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `openai_compatible_credentials` ADD COLUMN `limit_sessions` INT NULL AFTER `status`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'openai_compatible_credentials'
    AND column_name = 'limit_rpm'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `openai_compatible_credentials` ADD COLUMN `limit_rpm` INT NULL AFTER `limit_sessions`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'openai_compatible_credentials'
    AND column_name = 'limit_tpm'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `openai_compatible_credentials` ADD COLUMN `limit_tpm` INT NULL AFTER `limit_rpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 3) codex_oauth_accounts: add limit_sessions/limit_rpm/limit_tpm
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'codex_oauth_accounts'
    AND column_name = 'limit_sessions'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `codex_oauth_accounts` ADD COLUMN `limit_sessions` INT NULL AFTER `status`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'codex_oauth_accounts'
    AND column_name = 'limit_rpm'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `codex_oauth_accounts` ADD COLUMN `limit_rpm` INT NULL AFTER `limit_sessions`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'codex_oauth_accounts'
    AND column_name = 'limit_tpm'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `codex_oauth_accounts` ADD COLUMN `limit_tpm` INT NULL AFTER `limit_rpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 4) Backfill：将 channel 级 limits 作为默认值回填到账号（仅当账号未显式设置时）
UPDATE openai_compatible_credentials c
JOIN upstream_endpoints e ON e.id = c.endpoint_id
JOIN upstream_channels ch ON ch.id = e.channel_id
SET c.limit_sessions = ch.limit_sessions
WHERE c.limit_sessions IS NULL AND ch.limit_sessions IS NOT NULL;

UPDATE openai_compatible_credentials c
JOIN upstream_endpoints e ON e.id = c.endpoint_id
JOIN upstream_channels ch ON ch.id = e.channel_id
SET c.limit_rpm = ch.limit_rpm
WHERE c.limit_rpm IS NULL AND ch.limit_rpm IS NOT NULL;

UPDATE openai_compatible_credentials c
JOIN upstream_endpoints e ON e.id = c.endpoint_id
JOIN upstream_channels ch ON ch.id = e.channel_id
SET c.limit_tpm = ch.limit_tpm
WHERE c.limit_tpm IS NULL AND ch.limit_tpm IS NOT NULL;

UPDATE codex_oauth_accounts a
JOIN upstream_endpoints e ON e.id = a.endpoint_id
JOIN upstream_channels ch ON ch.id = e.channel_id
SET a.limit_sessions = ch.limit_sessions
WHERE a.limit_sessions IS NULL AND ch.limit_sessions IS NOT NULL;

UPDATE codex_oauth_accounts a
JOIN upstream_endpoints e ON e.id = a.endpoint_id
JOIN upstream_channels ch ON ch.id = e.channel_id
SET a.limit_rpm = ch.limit_rpm
WHERE a.limit_rpm IS NULL AND ch.limit_rpm IS NOT NULL;

UPDATE codex_oauth_accounts a
JOIN upstream_endpoints e ON e.id = a.endpoint_id
JOIN upstream_channels ch ON ch.id = e.channel_id
SET a.limit_tpm = ch.limit_tpm
WHERE a.limit_tpm IS NULL AND ch.limit_tpm IS NOT NULL;

