-- 0023_usage_events_upstream_channel_id.sql: usage_events 增加 upstream_channel_id，用于按渠道统计用量。

ALTER TABLE `usage_events`
  ADD COLUMN `upstream_channel_id` BIGINT NULL AFTER `token_id`,
  ADD KEY `idx_usage_events_state_time_upstream_channel` (`state`, `time`, `upstream_channel_id`);

