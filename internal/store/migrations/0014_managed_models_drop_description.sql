-- 0014_managed_models_drop_description.sql: managed_models 移除 description 字段（仅展示用途）。

ALTER TABLE managed_models
  DROP COLUMN description;

