-- 0012_drop_pricing_models.sql: 移除 legacy pricing_models（定价与模型绑定到 managed_models）。

DROP TABLE IF EXISTS pricing_models;
