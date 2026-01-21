-- 0010_usage_events_cached_tokens.sql: usage_events 增加缓存 token 统计字段（输入/输出）。

ALTER TABLE `usage_events`
  ADD COLUMN `cached_input_tokens` BIGINT NULL AFTER `input_tokens`,
  ADD COLUMN `cached_output_tokens` BIGINT NULL AFTER `output_tokens`;

