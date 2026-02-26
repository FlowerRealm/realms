-- 0062_self_mode_key_hash_to_personal_mode_key_hash.sql: 迁移 personal 模式鉴权 Key（legacy: self_mode_key_hash）。
--
-- 目标：
-- - 从 app_settings['self_mode_key_hash'] 复制到 app_settings['personal_mode_key_hash']
-- - 仅在 personal_mode_key_hash 不存在时写入（无损复制）
-- - 空值不迁移（避免写入空值导致 bootstrap 被 InsertAppSettingIfAbsent 误判为“已设置”）

INSERT IGNORE INTO app_settings(`key`, value, created_at, updated_at)
SELECT 'personal_mode_key_hash', value, created_at, updated_at
FROM app_settings
WHERE `key` = 'self_mode_key_hash'
  AND TRIM(value) <> '';

