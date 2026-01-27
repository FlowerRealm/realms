-- 0039_anthropic_credentials.sql: 新增 anthropic_credentials，用于存储 Anthropic Messages 上游 API key。

CREATE TABLE IF NOT EXISTS `anthropic_credentials` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `endpoint_id` BIGINT NOT NULL,
  `name` VARCHAR(128) NULL,
  `api_key_enc` BLOB NOT NULL,
  `api_key_hint` VARCHAR(32) NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `last_used_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_anthropic_credentials_endpoint_id` (`endpoint_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
