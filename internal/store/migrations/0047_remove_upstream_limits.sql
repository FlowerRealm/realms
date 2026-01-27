-- 0047_remove_upstream_limits.sql: 移除上游 RPM/TPM/会话限额相关字段（渠道/账号/密钥）。

-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“列已删除但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里对列是否存在做条件判断。

-- 1) upstream_channels: drop legacy limit fields
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_sessions'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `upstream_channels` DROP COLUMN `limit_sessions`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_rpm'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `upstream_channels` DROP COLUMN `limit_rpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_tpm'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `upstream_channels` DROP COLUMN `limit_tpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_cc'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `upstream_channels` DROP COLUMN `limit_cc`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'upstream_channels'
    AND column_name = 'limit_rpd'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `upstream_channels` DROP COLUMN `limit_rpd`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2) openai_compatible_credentials: drop limit fields
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'openai_compatible_credentials'
    AND column_name = 'limit_sessions'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `openai_compatible_credentials` DROP COLUMN `limit_sessions`',
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
  @col_exists > 0,
  'ALTER TABLE `openai_compatible_credentials` DROP COLUMN `limit_rpm`',
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
  @col_exists > 0,
  'ALTER TABLE `openai_compatible_credentials` DROP COLUMN `limit_tpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 3) codex_oauth_accounts: drop limit fields
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'codex_oauth_accounts'
    AND column_name = 'limit_sessions'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `codex_oauth_accounts` DROP COLUMN `limit_sessions`',
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
  @col_exists > 0,
  'ALTER TABLE `codex_oauth_accounts` DROP COLUMN `limit_rpm`',
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
  @col_exists > 0,
  'ALTER TABLE `codex_oauth_accounts` DROP COLUMN `limit_tpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 4) anthropic_credentials: drop limit fields
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'anthropic_credentials'
    AND column_name = 'limit_sessions'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `anthropic_credentials` DROP COLUMN `limit_sessions`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'anthropic_credentials'
    AND column_name = 'limit_rpm'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `anthropic_credentials` DROP COLUMN `limit_rpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'anthropic_credentials'
    AND column_name = 'limit_tpm'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `anthropic_credentials` DROP COLUMN `limit_tpm`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

