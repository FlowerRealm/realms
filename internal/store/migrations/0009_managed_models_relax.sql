-- 0009_managed_models_relax.sql: managed_models 上游字段不再作为必填（为“渠道绑定模型”模式做兼容）。

ALTER TABLE managed_models
  MODIFY `upstream_model` VARCHAR(128) NULL;

ALTER TABLE managed_models
  MODIFY `upstream_type` VARCHAR(32) NULL;

