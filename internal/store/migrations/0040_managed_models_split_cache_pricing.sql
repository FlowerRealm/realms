-- 0040_managed_models_split_cache_pricing.sql: managed_models 拆分缓存定价（输入/输出）。

ALTER TABLE managed_models
  ADD COLUMN `cache_input_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000 AFTER `output_usd_per_1m`,
  ADD COLUMN `cache_output_usd_per_1m` DECIMAL(20,6) NOT NULL DEFAULT 5.000000 AFTER `cache_input_usd_per_1m`;

UPDATE managed_models
SET
  cache_input_usd_per_1m = cache_usd_per_1m,
  cache_output_usd_per_1m = cache_usd_per_1m;

ALTER TABLE managed_models
  DROP COLUMN `cache_usd_per_1m`;

