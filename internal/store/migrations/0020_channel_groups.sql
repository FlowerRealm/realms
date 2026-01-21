-- 0020_channel_groups.sql: 新增渠道分组表，用于下拉选择与统一管理。

CREATE TABLE IF NOT EXISTS `channel_groups` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `name` VARCHAR(64) NOT NULL,
  `description` VARCHAR(255) NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_channel_groups_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO channel_groups(name, description, status, created_at, updated_at)
SELECT 'default', '默认分组', 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM channel_groups WHERE name='default' LIMIT 1);

