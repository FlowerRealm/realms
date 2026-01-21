-- 0016_managed_models_drop_updated_at.sql: managed_models 移除 updated_at 字段（仅展示用途）。

-- 兼容：部分环境可能已手动删除该列，避免迁移失败。
SELECT IF(
  EXISTS(
    SELECT 1
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'managed_models'
      AND COLUMN_NAME = 'updated_at'
  ),
  'ALTER TABLE managed_models DROP COLUMN updated_at',
  'SELECT 1'
) INTO @realms_stmt_drop_updated_at;

PREPARE realms_stmt_drop_updated_at FROM @realms_stmt_drop_updated_at;
EXECUTE realms_stmt_drop_updated_at;
DEALLOCATE PREPARE realms_stmt_drop_updated_at;
