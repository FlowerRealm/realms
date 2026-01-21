-- 0030_usage_events_request_details.sql: usage_events 增加“请求级别明细”字段，支持按每次请求展示用量、状态与错误信息。

ALTER TABLE `usage_events`
  ADD COLUMN `endpoint` VARCHAR(128) NULL AFTER `request_id`,
  ADD COLUMN `method` VARCHAR(16) NULL AFTER `endpoint`,
  ADD COLUMN `status_code` INT NOT NULL DEFAULT 0 AFTER `method`,
  ADD COLUMN `latency_ms` INT NOT NULL DEFAULT 0 AFTER `status_code`,
  ADD COLUMN `error_class` VARCHAR(64) NULL AFTER `latency_ms`,
  ADD COLUMN `error_message` VARCHAR(255) NULL AFTER `error_class`,
  ADD COLUMN `upstream_endpoint_id` BIGINT NULL AFTER `upstream_channel_id`,
  ADD COLUMN `upstream_credential_id` BIGINT NULL AFTER `upstream_endpoint_id`,
  ADD COLUMN `is_stream` TINYINT NOT NULL DEFAULT 0 AFTER `reserve_expires_at`,
  ADD COLUMN `request_bytes` BIGINT NOT NULL DEFAULT 0 AFTER `is_stream`,
  ADD COLUMN `response_bytes` BIGINT NOT NULL DEFAULT 0 AFTER `request_bytes`,
  ADD KEY `idx_usage_events_user_id_id` (`user_id`, `id`),
  ADD KEY `idx_usage_events_time_id` (`time`, `id`);

