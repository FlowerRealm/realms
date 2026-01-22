-- schema_sqlite.sql: Realms SQLite 一次性初始化 schema（默认单机部署）。
-- 说明：
-- - 仅用于 SQLite（db.driver=sqlite）
-- - 追求“可运行的最终形态”，不复刻历史 MySQL 迁移过程

CREATE TABLE IF NOT EXISTS `users` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `email` TEXT NOT NULL,
  `username` TEXT NOT NULL,
  `password_hash` BLOB NOT NULL,
  `role` TEXT NOT NULL DEFAULT 'user',
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_users_email` ON `users` (`email`);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_users_username` ON `users` (`username`);

CREATE TABLE IF NOT EXISTS `user_groups` (
  `user_id` INTEGER NOT NULL,
  `group_name` TEXT NOT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`user_id`, `group_name`)
);
CREATE INDEX IF NOT EXISTS `idx_user_groups_group_name` ON `user_groups` (`group_name`);
CREATE INDEX IF NOT EXISTS `idx_user_groups_user_id` ON `user_groups` (`user_id`);

CREATE TABLE IF NOT EXISTS `user_tokens` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `name` TEXT NULL,
  `token_hash` BLOB NOT NULL,
  `token_hint` TEXT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `revoked_at` DATETIME NULL,
  `last_used_at` DATETIME NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_user_tokens_hash` ON `user_tokens` (`token_hash`);
CREATE INDEX IF NOT EXISTS `idx_user_tokens_user_id` ON `user_tokens` (`user_id`);

CREATE TABLE IF NOT EXISTS `user_sessions` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `session_hash` BLOB NOT NULL,
  `csrf_token` TEXT NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL,
  `last_seen_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_user_sessions_hash` ON `user_sessions` (`session_hash`);
CREATE INDEX IF NOT EXISTS `idx_user_sessions_user_id` ON `user_sessions` (`user_id`);

CREATE TABLE IF NOT EXISTS `email_verifications` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NULL,
  `email` TEXT NOT NULL,
  `code_hash` BLOB NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `verified_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_email_verifications_user_id` ON `email_verifications` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_email_verifications_email` ON `email_verifications` (`email`);

CREATE TABLE IF NOT EXISTS `upstream_channels` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `type` TEXT NOT NULL,
  `name` TEXT NOT NULL,
  `groups` TEXT NOT NULL DEFAULT 'default',
  `status` INTEGER NOT NULL DEFAULT 1,
  `priority` INTEGER NOT NULL DEFAULT 0,
  `promotion` INTEGER NOT NULL DEFAULT 0,
  `limit_sessions` INTEGER NULL,
  `limit_rpm` INTEGER NULL,
  `limit_tpm` INTEGER NULL,
  `last_test_at` DATETIME NULL,
  `last_test_latency_ms` INTEGER NOT NULL DEFAULT 0,
  `last_test_ok` INTEGER NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS `upstream_endpoints` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `channel_id` INTEGER NOT NULL,
  `base_url` TEXT NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `priority` INTEGER NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_upstream_endpoints_channel_id` ON `upstream_endpoints` (`channel_id`);

CREATE TABLE IF NOT EXISTS `openai_compatible_credentials` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `endpoint_id` INTEGER NOT NULL,
  `name` TEXT NULL,
  `api_key_enc` BLOB NOT NULL,
  `api_key_hint` TEXT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `limit_sessions` INTEGER NULL,
  `limit_rpm` INTEGER NULL,
  `limit_tpm` INTEGER NULL,
  `last_used_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_openai_credentials_endpoint_id` ON `openai_compatible_credentials` (`endpoint_id`);

CREATE TABLE IF NOT EXISTS `codex_oauth_accounts` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `endpoint_id` INTEGER NOT NULL,
  `account_id` TEXT NOT NULL,
  `email` TEXT NULL,
  `access_token_enc` BLOB NOT NULL,
  `refresh_token_enc` BLOB NOT NULL,
  `id_token_enc` BLOB NULL,
  `expires_at` DATETIME NULL,
  `last_refresh_at` DATETIME NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `limit_sessions` INTEGER NULL,
  `limit_rpm` INTEGER NULL,
  `limit_tpm` INTEGER NULL,
  `cooldown_until` DATETIME NULL,
  `last_used_at` DATETIME NULL,

  `balance_total_granted_usd` DECIMAL(20,6) NULL,
  `balance_total_used_usd` DECIMAL(20,6) NULL,
  `balance_total_available_usd` DECIMAL(20,6) NULL,
  `balance_updated_at` DATETIME NULL,
  `balance_error` TEXT NULL,

  `quota_credits_has_credits` INTEGER NULL,
  `quota_credits_unlimited` INTEGER NULL,
  `quota_credits_balance` TEXT NULL,
  `quota_primary_used_percent` INTEGER NULL,
  `quota_primary_reset_at` DATETIME NULL,
  `quota_secondary_used_percent` INTEGER NULL,
  `quota_secondary_reset_at` DATETIME NULL,
  `quota_updated_at` DATETIME NULL,
  `quota_error` TEXT NULL,

  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_codex_accounts_endpoint_id` ON `codex_oauth_accounts` (`endpoint_id`);

CREATE TABLE IF NOT EXISTS `codex_oauth_pending` (
  `state` TEXT PRIMARY KEY,
  `endpoint_id` INTEGER NOT NULL,
  `actor_user_id` INTEGER NOT NULL,
  `code_verifier` TEXT NOT NULL,
  `created_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_codex_oauth_pending_created_at` ON `codex_oauth_pending` (`created_at`);
CREATE INDEX IF NOT EXISTS `idx_codex_oauth_pending_endpoint_id` ON `codex_oauth_pending` (`endpoint_id`);

CREATE TABLE IF NOT EXISTS `audit_events` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `time` DATETIME NOT NULL,
  `request_id` TEXT NOT NULL,
  `actor_type` TEXT NOT NULL,
  `user_id` INTEGER NULL,
  `token_id` INTEGER NULL,
  `action` TEXT NOT NULL,
  `endpoint` TEXT NOT NULL,
  `model` TEXT NULL,
  `upstream_channel_id` INTEGER NULL,
  `upstream_endpoint_id` INTEGER NULL,
  `upstream_credential_id` INTEGER NULL,
  `status_code` INTEGER NOT NULL,
  `latency_ms` INTEGER NOT NULL,
  `error_class` TEXT NULL,
  `error_message` TEXT NULL
);
CREATE INDEX IF NOT EXISTS `idx_audit_events_time` ON `audit_events` (`time`);
CREATE INDEX IF NOT EXISTS `idx_audit_events_request_id` ON `audit_events` (`request_id`);
CREATE INDEX IF NOT EXISTS `idx_audit_events_user_id` ON `audit_events` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_audit_events_token_id` ON `audit_events` (`token_id`);

CREATE TABLE IF NOT EXISTS `usage_events` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `time` DATETIME NOT NULL,
  `request_id` TEXT NOT NULL,
  `endpoint` TEXT NULL,
  `method` TEXT NULL,
  `user_id` INTEGER NOT NULL,
  `subscription_id` INTEGER NULL,
  `token_id` INTEGER NOT NULL,
  `upstream_channel_id` INTEGER NULL,
  `upstream_endpoint_id` INTEGER NULL,
  `upstream_credential_id` INTEGER NULL,
  `state` TEXT NOT NULL,
  `model` TEXT NULL,
  `input_tokens` INTEGER NULL,
  `cached_input_tokens` INTEGER NULL,
  `output_tokens` INTEGER NULL,
  `cached_output_tokens` INTEGER NULL,
  `reserved_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `committed_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `reserve_expires_at` DATETIME NOT NULL,
  `status_code` INTEGER NOT NULL DEFAULT 0,
  `latency_ms` INTEGER NOT NULL DEFAULT 0,
  `error_class` TEXT NULL,
  `error_message` TEXT NULL,
  `is_stream` INTEGER NOT NULL DEFAULT 0,
  `request_bytes` INTEGER NOT NULL DEFAULT 0,
  `response_bytes` INTEGER NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_usage_events_time` ON `usage_events` (`time`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_request_id` ON `usage_events` (`request_id`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_user_id` ON `usage_events` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_token_id` ON `usage_events` (`token_id`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_state` ON `usage_events` (`state`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_user_state_time` ON `usage_events` (`user_id`, `state`, `time`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_state_reserve_expires` ON `usage_events` (`state`, `reserve_expires_at`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_user_subscription_state_time` ON `usage_events` (`user_id`, `subscription_id`, `state`, `time`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_state_time_upstream_channel` ON `usage_events` (`state`, `time`, `upstream_channel_id`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_user_id_id` ON `usage_events` (`user_id`, `id`);
CREATE INDEX IF NOT EXISTS `idx_usage_events_time_id` ON `usage_events` (`time`, `id`);

CREATE TABLE IF NOT EXISTS `subscription_plans` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `code` TEXT NOT NULL,
  `name` TEXT NOT NULL,
  `group_name` TEXT NOT NULL DEFAULT 'default',
  `price_cny` DECIMAL(20,2) NOT NULL,
  `limit_5h_usd` DECIMAL(20,6) NOT NULL,
  `limit_1d_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `limit_7d_usd` DECIMAL(20,6) NOT NULL,
  `limit_30d_usd` DECIMAL(20,6) NOT NULL,
  `duration_days` INTEGER NOT NULL DEFAULT 30,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_subscription_plans_code` ON `subscription_plans` (`code`);

CREATE TABLE IF NOT EXISTS `user_subscriptions` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `plan_id` INTEGER NOT NULL,
  `start_at` DATETIME NOT NULL,
  `end_at` DATETIME NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_user_subscriptions_end_at` ON `user_subscriptions` (`end_at`);
CREATE INDEX IF NOT EXISTS `idx_user_subscriptions_user_status_end_at` ON `user_subscriptions` (`user_id`, `status`, `end_at`);

CREATE TABLE IF NOT EXISTS `user_balances` (
  `user_id` INTEGER PRIMARY KEY,
  `usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS `payment_channels` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `type` TEXT NOT NULL,
  `name` TEXT NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,

  `stripe_currency` TEXT NULL,
  `stripe_secret_key` TEXT NULL,
  `stripe_webhook_secret` TEXT NULL,

  `epay_gateway` TEXT NULL,
  `epay_partner_id` TEXT NULL,
  `epay_key` TEXT NULL,

  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS `subscription_orders` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `plan_id` INTEGER NOT NULL,
  `amount_cny` DECIMAL(20,2) NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 0,
  `paid_at` DATETIME NULL,
  `paid_method` TEXT NULL,
  `paid_ref` TEXT NULL,
  `paid_channel_id` INTEGER NULL,
  `approved_at` DATETIME NULL,
  `approved_by` INTEGER NULL,
  `subscription_id` INTEGER NULL,
  `note` TEXT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_subscription_orders_user_id_id` ON `subscription_orders` (`user_id`, `id`);
CREATE INDEX IF NOT EXISTS `idx_subscription_orders_status_id` ON `subscription_orders` (`status`, `id`);
CREATE INDEX IF NOT EXISTS `idx_subscription_orders_subscription_id` ON `subscription_orders` (`subscription_id`);

CREATE TABLE IF NOT EXISTS `topup_orders` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `amount_cny` DECIMAL(20,2) NOT NULL,
  `credit_usd` DECIMAL(20,6) NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 0,
  `paid_at` DATETIME NULL,
  `paid_method` TEXT NULL,
  `paid_ref` TEXT NULL,
  `paid_channel_id` INTEGER NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_topup_orders_user_id_id` ON `topup_orders` (`user_id`, `id`);
CREATE INDEX IF NOT EXISTS `idx_topup_orders_status_id` ON `topup_orders` (`status`, `id`);

CREATE TABLE IF NOT EXISTS `tickets` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `subject` TEXT NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `last_message_at` DATETIME NOT NULL,
  `closed_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_tickets_user_id_id` ON `tickets` (`user_id`, `id`);
CREATE INDEX IF NOT EXISTS `idx_tickets_status_last_message_at` ON `tickets` (`status`, `last_message_at`);

CREATE TABLE IF NOT EXISTS `ticket_messages` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `ticket_id` INTEGER NOT NULL,
  `actor_type` TEXT NOT NULL,
  `actor_user_id` INTEGER NULL,
  `body` TEXT NOT NULL,
  `created_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_ticket_messages_ticket_id_id` ON `ticket_messages` (`ticket_id`, `id`);

CREATE TABLE IF NOT EXISTS `ticket_attachments` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `ticket_id` INTEGER NOT NULL,
  `message_id` INTEGER NOT NULL,
  `uploader_user_id` INTEGER NULL,
  `original_name` TEXT NOT NULL,
  `content_type` TEXT NULL,
  `size_bytes` INTEGER NOT NULL,
  `sha256` BLOB NULL,
  `storage_rel_path` TEXT NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_ticket_attachments_ticket_id_id` ON `ticket_attachments` (`ticket_id`, `id`);
CREATE INDEX IF NOT EXISTS `idx_ticket_attachments_expires_at` ON `ticket_attachments` (`expires_at`);

CREATE TABLE IF NOT EXISTS `announcements` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `title` TEXT NOT NULL,
  `body` TEXT NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS `idx_announcements_status_created` ON `announcements` (`status`, `created_at`);
CREATE INDEX IF NOT EXISTS `idx_announcements_created` ON `announcements` (`created_at`);

CREATE TABLE IF NOT EXISTS `announcement_reads` (
  `user_id` INTEGER NOT NULL,
  `announcement_id` INTEGER NOT NULL,
  `read_at` DATETIME NOT NULL,
  PRIMARY KEY (`user_id`, `announcement_id`)
);
CREATE INDEX IF NOT EXISTS `idx_announcement_reads_announcement_id` ON `announcement_reads` (`announcement_id`);

CREATE TABLE IF NOT EXISTS `channel_groups` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `name` TEXT NOT NULL,
  `description` TEXT NULL,
  `price_multiplier` DECIMAL(25,6) NOT NULL DEFAULT 1.000000,
  `status` INTEGER NOT NULL DEFAULT 1,
  `max_attempts` INTEGER NOT NULL DEFAULT 5,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_channel_groups_name` ON `channel_groups` (`name`);

CREATE TABLE IF NOT EXISTS `channel_group_members` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `parent_group_id` INTEGER NOT NULL,
  `member_group_id` INTEGER NULL,
  `member_channel_id` INTEGER NULL,
  `priority` INTEGER NOT NULL DEFAULT 0,
  `promotion` INTEGER NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_parent_member_group` ON `channel_group_members` (`parent_group_id`, `member_group_id`);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_parent_member_channel` ON `channel_group_members` (`parent_group_id`, `member_channel_id`);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_group_single_parent` ON `channel_group_members` (`member_group_id`);
CREATE INDEX IF NOT EXISTS `idx_parent_order` ON `channel_group_members` (`parent_group_id`, `promotion`, `priority`, `id`);

CREATE TABLE IF NOT EXISTS `managed_models` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `public_id` TEXT NOT NULL,
  `upstream_model` TEXT NULL,
  `owned_by` TEXT NULL,
  `input_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000,
  `output_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 15.000000,
  `cache_input_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000,
  `cache_output_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_managed_models_public_id` ON `managed_models` (`public_id`);
CREATE INDEX IF NOT EXISTS `idx_managed_models_status` ON `managed_models` (`status`);

CREATE TABLE IF NOT EXISTS `channel_models` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `channel_id` INTEGER NOT NULL,
  `public_id` TEXT NOT NULL,
  `upstream_model` TEXT NOT NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_channel_models_channel_public` ON `channel_models` (`channel_id`, `public_id`);
CREATE INDEX IF NOT EXISTS `idx_channel_models_public_id` ON `channel_models` (`public_id`);
CREATE INDEX IF NOT EXISTS `idx_channel_models_status` ON `channel_models` (`status`);
CREATE INDEX IF NOT EXISTS `idx_channel_models_channel_id` ON `channel_models` (`channel_id`);

CREATE TABLE IF NOT EXISTS `app_settings` (
  `key` TEXT PRIMARY KEY,
  `value` TEXT NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS `oauth_apps` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `client_id` TEXT NOT NULL,
  `name` TEXT NOT NULL,
  `client_secret_hash` BLOB NULL,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_oauth_apps_client_id` ON `oauth_apps` (`client_id`);

CREATE TABLE IF NOT EXISTS `oauth_app_redirect_uris` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `app_id` INTEGER NOT NULL,
  `redirect_uri` TEXT NOT NULL,
  `redirect_uri_hash` BLOB NOT NULL,
  `created_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_oauth_app_redirect_uris_app_id_hash` ON `oauth_app_redirect_uris` (`app_id`, `redirect_uri_hash`);
CREATE INDEX IF NOT EXISTS `idx_oauth_app_redirect_uris_app_id` ON `oauth_app_redirect_uris` (`app_id`);

CREATE TABLE IF NOT EXISTS `oauth_user_grants` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `user_id` INTEGER NOT NULL,
  `app_id` INTEGER NOT NULL,
  `scope` TEXT NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_oauth_user_grants_user_app` ON `oauth_user_grants` (`user_id`, `app_id`);
CREATE INDEX IF NOT EXISTS `idx_oauth_user_grants_user_id` ON `oauth_user_grants` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_oauth_user_grants_app_id` ON `oauth_user_grants` (`app_id`);

CREATE TABLE IF NOT EXISTS `oauth_auth_codes` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `code_hash` BLOB NOT NULL,
  `app_id` INTEGER NOT NULL,
  `user_id` INTEGER NOT NULL,
  `redirect_uri` TEXT NOT NULL,
  `scope` TEXT NOT NULL DEFAULT '',
  `code_challenge` TEXT NULL,
  `code_challenge_method` TEXT NULL,
  `expires_at` DATETIME NOT NULL,
  `consumed_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_oauth_auth_codes_hash` ON `oauth_auth_codes` (`code_hash`);
CREATE INDEX IF NOT EXISTS `idx_oauth_auth_codes_app_id` ON `oauth_auth_codes` (`app_id`);
CREATE INDEX IF NOT EXISTS `idx_oauth_auth_codes_user_id` ON `oauth_auth_codes` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_oauth_auth_codes_expires_at` ON `oauth_auth_codes` (`expires_at`);

CREATE TABLE IF NOT EXISTS `oauth_app_tokens` (
  `id` INTEGER PRIMARY KEY AUTOINCREMENT,
  `app_id` INTEGER NOT NULL,
  `user_id` INTEGER NOT NULL,
  `token_id` INTEGER NOT NULL,
  `scope` TEXT NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS `uk_oauth_app_tokens_token_id` ON `oauth_app_tokens` (`token_id`);
CREATE INDEX IF NOT EXISTS `idx_oauth_app_tokens_user_id` ON `oauth_app_tokens` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_oauth_app_tokens_app_id` ON `oauth_app_tokens` (`app_id`);

-- Seed: channel_groups 默认分组
INSERT INTO channel_groups(name, description, price_multiplier, status, max_attempts, created_at, updated_at)
SELECT 'default', '默认分组', 1.000000, 1, 5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM channel_groups WHERE name='default' LIMIT 1);

-- Seed: 内置 Codex OAuth 渠道
INSERT INTO upstream_channels(type, name, `groups`, status, priority, promotion, limit_sessions, limit_rpm, limit_tpm, last_test_at, last_test_latency_ms, last_test_ok, created_at, updated_at)
SELECT 'codex_oauth', 'Codex OAuth', 'default', 1, 0, 0, NULL, NULL, NULL, NULL, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM upstream_channels WHERE type='codex_oauth' LIMIT 1);

-- Seed: 为内置 codex_oauth 补齐默认 endpoint（与 MySQL 迁移一致）
INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
SELECT c.id, 'https://chatgpt.com/backend-api/codex', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM upstream_channels c
WHERE c.type='codex_oauth'
  AND NOT EXISTS (SELECT 1 FROM upstream_endpoints e WHERE e.channel_id=c.id);

-- Seed: 默认订阅套餐（保持与历史迁移一致；limit_1d_usd=0 为后续字段默认）
INSERT INTO subscription_plans(
  code, name, group_name, price_cny,
  limit_5h_usd, limit_1d_usd, limit_7d_usd, limit_30d_usd,
  duration_days, status, created_at, updated_at
)
SELECT
  'basic_12', '基础订阅', 'default', 12.00,
  6.000000, 0.000000, 20.000000, 80.000000,
  30, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM subscription_plans WHERE code='basic_12' LIMIT 1);
