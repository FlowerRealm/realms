-- 0058_rename_default_main_group_description.sql: 将默认主组描述更新为“默认用户分组”（避免 UI 文案混淆）。

SET @tbl_exists := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE table_schema = DATABASE()
    AND table_name = 'main_groups'
);
SET @ddl := IF(
  @tbl_exists > 0,
  "UPDATE main_groups SET description='默认用户分组', updated_at=CURRENT_TIMESTAMP WHERE name='default' AND (description IS NULL OR TRIM(description)='' OR description='默认主组')",
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

