-- 0003_channel_test_fields.sql: upstream_channels 增加最近一次测试指标（可用性/延迟）。

ALTER TABLE `upstream_channels`
  ADD COLUMN `last_test_at` DATETIME NULL AFTER `promotion`,
  ADD COLUMN `last_test_latency_ms` INT NOT NULL DEFAULT 0 AFTER `last_test_at`,
  ADD COLUMN `last_test_ok` TINYINT NOT NULL DEFAULT 0 AFTER `last_test_latency_ms`;

