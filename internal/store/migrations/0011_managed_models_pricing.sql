-- 0011_managed_models_pricing.sql: managed_models 移除 legacy 上游字段，新增按模型定价（input/output/cache）。

ALTER TABLE managed_models
  DROP INDEX idx_managed_models_upstream_type,
  DROP INDEX idx_managed_models_channel_id,
  DROP COLUMN upstream_type,
  DROP COLUMN upstream_channel_id,
  ADD COLUMN `input_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000 AFTER `description`,
  ADD COLUMN `output_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 15.000000 AFTER `input_usd_per_1m`,
  ADD COLUMN `cache_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000 AFTER `output_usd_per_1m`;
