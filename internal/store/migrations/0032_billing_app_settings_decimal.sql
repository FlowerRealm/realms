-- 0032_billing_app_settings_decimal.sql: app_settings 计费配置改用小数键名与小数值。

INSERT INTO app_settings(`key`, value, created_at, updated_at)
SELECT
  'billing_min_topup_cny',
  CAST(value AS DECIMAL(20,2)) / 100,
  created_at,
  updated_at
FROM app_settings
WHERE `key`='billing_min_topup_cny_fen'
ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=NOW();

INSERT INTO app_settings(`key`, value, created_at, updated_at)
SELECT
  'billing_credit_usd_per_cny',
  CAST(value AS DECIMAL(20,6)) / 10000,
  created_at,
  updated_at
FROM app_settings
WHERE `key`='billing_credit_usd_micros_per_cny_fen'
ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=NOW();

DELETE FROM app_settings WHERE `key` IN (
  'billing_min_topup_cny_fen',
  'billing_credit_usd_micros_per_cny_fen'
);

