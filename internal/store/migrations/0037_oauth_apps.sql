-- 0037_oauth_apps.sql: OAuth Apps（授权码换取 Realms API Token）。

CREATE TABLE IF NOT EXISTS `oauth_apps` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `client_id` VARCHAR(128) NOT NULL,
  `name` VARCHAR(255) NOT NULL,
  `client_secret_hash` VARBINARY(255) NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_oauth_apps_client_id` (`client_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `oauth_app_redirect_uris` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `app_id` BIGINT NOT NULL,
  `redirect_uri` VARCHAR(2048) NOT NULL,
  `redirect_uri_hash` VARBINARY(32) NOT NULL,
  `created_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_oauth_app_redirect_uris_app_id_hash` (`app_id`, `redirect_uri_hash`),
  KEY `idx_oauth_app_redirect_uris_app_id` (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `oauth_user_grants` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `app_id` BIGINT NOT NULL,
  `scope` VARCHAR(2048) NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_oauth_user_grants_user_app` (`user_id`, `app_id`),
  KEY `idx_oauth_user_grants_user_id` (`user_id`),
  KEY `idx_oauth_user_grants_app_id` (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `oauth_auth_codes` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `code_hash` VARBINARY(32) NOT NULL,
  `app_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `redirect_uri` VARCHAR(2048) NOT NULL,
  `scope` VARCHAR(2048) NOT NULL DEFAULT '',
  `code_challenge` VARCHAR(255) NULL,
  `code_challenge_method` VARCHAR(32) NULL,
  `expires_at` DATETIME NOT NULL,
  `consumed_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_oauth_auth_codes_hash` (`code_hash`),
  KEY `idx_oauth_auth_codes_app_id` (`app_id`),
  KEY `idx_oauth_auth_codes_user_id` (`user_id`),
  KEY `idx_oauth_auth_codes_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `oauth_app_tokens` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `app_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `token_id` BIGINT NOT NULL,
  `scope` VARCHAR(2048) NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_oauth_app_tokens_token_id` (`token_id`),
  KEY `idx_oauth_app_tokens_user_id` (`user_id`),
  KEY `idx_oauth_app_tokens_app_id` (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
