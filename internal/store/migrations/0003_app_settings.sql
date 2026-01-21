-- 0003_app_settings.sql: 应用级可变配置（UI 可配置开关）。

CREATE TABLE IF NOT EXISTS `app_settings` (
  `key` VARCHAR(64) PRIMARY KEY,
  `value` VARCHAR(255) NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

