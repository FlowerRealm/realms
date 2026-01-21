-- 0038_policy_to_feature_bans.sql: 将 legacy policy_* 与 models 开关迁移到统一的 feature_disable_*。

INSERT INTO app_settings(`key`, value, created_at, updated_at)
SELECT 'feature_disable_billing', 'true', NOW(), NOW()
WHERE EXISTS (
  SELECT 1
  FROM app_settings
  WHERE `key` = 'policy_free_mode'
    AND TRIM(LOWER(value)) IN ('1', 'true', 't', 'yes', 'y', 'on')
)
ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=NOW();

INSERT INTO app_settings(`key`, value, created_at, updated_at)
SELECT 'feature_disable_models', 'true', NOW(), NOW()
WHERE EXISTS (
  SELECT 1
  FROM app_settings
  WHERE `key` IN ('policy_model_passthrough', 'feature_disable_web_models', 'feature_disable_admin_models')
    AND TRIM(LOWER(value)) IN ('1', 'true', 't', 'yes', 'y', 'on')
)
ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=NOW();

DELETE FROM app_settings
WHERE `key` IN ('policy_free_mode', 'policy_model_passthrough', 'feature_disable_web_models', 'feature_disable_admin_models');

