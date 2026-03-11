-- 0071_usage_events_model_check.sql: usage_events 增加转发模型与上游回包模型字段，用于请求记录模型一致性检查。

ALTER TABLE `usage_events`
  ADD COLUMN `forwarded_model` VARCHAR(128) NULL AFTER `model`,
  ADD COLUMN `upstream_response_model` VARCHAR(128) NULL AFTER `forwarded_model`;
