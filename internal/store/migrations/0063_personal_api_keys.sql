-- 0063_personal_api_keys.sql: personal 模式“数据面 API Key”表（可创建多个，用于 /v1/*）。

CREATE TABLE IF NOT EXISTS `personal_api_keys` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `name` VARCHAR(128) NULL,
  `key_hash` VARBINARY(32) NOT NULL,
  `key_hint` VARCHAR(32) NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `revoked_at` DATETIME NULL,
  `last_used_at` DATETIME NULL,
  UNIQUE KEY `uk_personal_api_keys_hash` (`key_hash`),
  KEY `idx_personal_api_keys_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

