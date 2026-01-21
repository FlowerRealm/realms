-- 0022_user_groups_and_plan_group.sql:
-- 1) 引入 user_groups（用户可加入多个组；强制包含 default）
-- 2) users.channel_group -> user_groups（回填后移除旧列）
-- 3) subscription_plans.channel_group -> subscription_plans.group_name

CREATE TABLE IF NOT EXISTS `user_groups` (
  `user_id` BIGINT NOT NULL,
  `group_name` VARCHAR(64) NOT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`user_id`, `group_name`),
  KEY `idx_user_groups_group_name` (`group_name`),
  KEY `idx_user_groups_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 兜底：所有用户至少属于 default 组（幂等）。
INSERT IGNORE INTO user_groups(user_id, group_name, created_at)
SELECT id, 'default', NOW()
FROM users;

-- 若 users.channel_group 存在，则回填到 user_groups（幂等）。
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'users'
    AND column_name = 'channel_group'
);
SET @ddl := IF(
  @col_exists > 0,
  'INSERT IGNORE INTO user_groups(user_id, group_name, created_at) SELECT id, TRIM(channel_group), NOW() FROM users WHERE TRIM(channel_group) <> ''''',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 回填完成后移除 users.channel_group（幂等）。
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'users'
    AND column_name = 'channel_group'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `users` DROP COLUMN `channel_group`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans: 新增 group_name（幂等）。
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'subscription_plans'
    AND column_name = 'group_name'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `subscription_plans` ADD COLUMN `group_name` VARCHAR(64) NOT NULL DEFAULT ''default'' AFTER `name`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans: 若旧列 channel_group 存在则回填到 group_name（幂等）。
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'subscription_plans'
    AND column_name = 'channel_group'
);
SET @ddl := IF(
  @col_exists > 0,
  'UPDATE subscription_plans SET group_name = TRIM(channel_group) WHERE TRIM(channel_group) <> ''''',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans: 移除旧列 channel_group（幂等）。
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'subscription_plans'
    AND column_name = 'channel_group'
);
SET @ddl := IF(
  @col_exists > 0,
  'ALTER TABLE `subscription_plans` DROP COLUMN `channel_group`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

