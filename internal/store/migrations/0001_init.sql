-- 0001_init.sql: realms 最小可用 schema（用户/上游/审计/用量/定价）。

CREATE TABLE IF NOT EXISTS `users` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `email` VARCHAR(255) NOT NULL,
  `password_hash` VARBINARY(255) NOT NULL,
  `role` VARCHAR(32) NOT NULL DEFAULT 'user',
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_users_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `user_tokens` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `name` VARCHAR(128) NULL,
  `token_hash` VARBINARY(32) NOT NULL,
  `token_hint` VARCHAR(32) NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `revoked_at` DATETIME NULL,
  `last_used_at` DATETIME NULL,
  UNIQUE KEY `uk_user_tokens_hash` (`token_hash`),
  KEY `idx_user_tokens_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `user_sessions` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `session_hash` VARBINARY(32) NOT NULL,
  `csrf_token` VARCHAR(64) NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL,
  `last_seen_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_user_sessions_hash` (`session_hash`),
  KEY `idx_user_sessions_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `email_verifications` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `email` VARCHAR(255) NOT NULL,
  `code_hash` VARBINARY(32) NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `verified_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  KEY `idx_email_verifications_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `upstream_channels` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `type` VARCHAR(32) NOT NULL,
  `name` VARCHAR(64) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `priority` INT NOT NULL DEFAULT 0,
  `promotion` TINYINT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `upstream_endpoints` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `channel_id` BIGINT NOT NULL,
  `base_url` VARCHAR(255) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `priority` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_upstream_endpoints_channel_id` (`channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `openai_compatible_credentials` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `endpoint_id` BIGINT NOT NULL,
  `name` VARCHAR(128) NULL,
  `api_key_enc` BLOB NOT NULL,
  `api_key_hint` VARCHAR(32) NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `last_used_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_openai_credentials_endpoint_id` (`endpoint_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `codex_oauth_accounts` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `endpoint_id` BIGINT NOT NULL,
  `account_id` VARCHAR(64) NOT NULL,
  `email` VARCHAR(255) NULL,
  `access_token_enc` BLOB NOT NULL,
  `refresh_token_enc` BLOB NOT NULL,
  `id_token_enc` BLOB NULL,
  `expires_at` DATETIME NULL,
  `last_refresh_at` DATETIME NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `cooldown_until` DATETIME NULL,
  `last_used_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_codex_accounts_endpoint_id` (`endpoint_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `audit_events` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `time` DATETIME NOT NULL,
  `request_id` VARCHAR(64) NOT NULL,
  `actor_type` VARCHAR(16) NOT NULL,
  `user_id` BIGINT NULL,
  `token_id` BIGINT NULL,
  `action` VARCHAR(64) NOT NULL,
  `endpoint` VARCHAR(128) NOT NULL,
  `model` VARCHAR(128) NULL,
  `upstream_channel_id` BIGINT NULL,
  `upstream_endpoint_id` BIGINT NULL,
  `upstream_credential_id` BIGINT NULL,
  `status_code` INT NOT NULL,
  `latency_ms` INT NOT NULL,
  `error_class` VARCHAR(64) NULL,
  `error_message` VARCHAR(255) NULL,
  KEY `idx_audit_events_time` (`time`),
  KEY `idx_audit_events_request_id` (`request_id`),
  KEY `idx_audit_events_user_id` (`user_id`),
  KEY `idx_audit_events_token_id` (`token_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `usage_events` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `time` DATETIME NOT NULL,
  `request_id` VARCHAR(64) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `token_id` BIGINT NOT NULL,
  `state` VARCHAR(16) NOT NULL,
  `model` VARCHAR(128) NULL,
  `input_tokens` BIGINT NULL,
  `output_tokens` BIGINT NULL,
  `reserved_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `committed_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `reserve_expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_usage_events_time` (`time`),
  KEY `idx_usage_events_request_id` (`request_id`),
  KEY `idx_usage_events_user_id` (`user_id`),
  KEY `idx_usage_events_token_id` (`token_id`),
  KEY `idx_usage_events_state` (`state`),
  KEY `idx_usage_events_user_state_time` (`user_id`, `state`, `time`),
  KEY `idx_usage_events_state_reserve_expires` (`state`, `reserve_expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
