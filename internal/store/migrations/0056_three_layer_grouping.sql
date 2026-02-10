-- 0056_three_layer_grouping.sql: 三层分组系统（主组/Token 分组/倍率拆分）。

-- 注意：MySQL 的 DDL 会隐式提交事务；为让迁移可重入，这里对列是否存在做条件判断。

-- users.main_group
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'users'
    AND column_name = 'main_group'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `users` ADD COLUMN `main_group` VARCHAR(64) NOT NULL DEFAULT ''default'' AFTER `role`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- subscription_plans.price_multiplier
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'subscription_plans'
    AND column_name = 'price_multiplier'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `subscription_plans` ADD COLUMN `price_multiplier` DECIMAL(20,6) NOT NULL DEFAULT 1.000000 AFTER `group_name`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
UPDATE subscription_plans SET price_multiplier=1.000000 WHERE price_multiplier IS NULL OR price_multiplier<=0;

-- usage_events multiplier columns
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'price_multiplier'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_events` ADD COLUMN `price_multiplier` DECIMAL(20,6) NOT NULL DEFAULT 1.000000 AFTER `committed_usd`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'price_multiplier_group'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_events` ADD COLUMN `price_multiplier_group` DECIMAL(20,6) NOT NULL DEFAULT 1.000000 AFTER `price_multiplier`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'price_multiplier_payment'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_events` ADD COLUMN `price_multiplier_payment` DECIMAL(20,6) NOT NULL DEFAULT 1.000000 AFTER `price_multiplier_group`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE table_schema = DATABASE()
    AND table_name = 'usage_events'
    AND column_name = 'price_multiplier_group_name'
);
SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `usage_events` ADD COLUMN `price_multiplier_group_name` VARCHAR(64) NULL AFTER `price_multiplier_payment`',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE usage_events SET price_multiplier=1.000000 WHERE price_multiplier IS NULL OR price_multiplier<=0;
UPDATE usage_events SET price_multiplier_group=1.000000 WHERE price_multiplier_group IS NULL OR price_multiplier_group<=0;
UPDATE usage_events SET price_multiplier_payment=1.000000 WHERE price_multiplier_payment IS NULL OR price_multiplier_payment<=0;

-- main_groups + main_group_subgroups
CREATE TABLE IF NOT EXISTS main_groups (
  name VARCHAR(64) NOT NULL,
  description VARCHAR(255) NULL,
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (name),
  KEY idx_main_groups_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS main_group_subgroups (
  main_group VARCHAR(64) NOT NULL,
  subgroup VARCHAR(64) NOT NULL,
  priority INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (main_group, subgroup),
  KEY idx_main_group_subgroups_main_group (main_group),
  KEY idx_main_group_subgroups_subgroup (subgroup)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- token_groups
CREATE TABLE IF NOT EXISTS token_groups (
  token_id BIGINT NOT NULL,
  group_name VARCHAR(64) NOT NULL,
  priority INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (token_id, group_name),
  KEY idx_token_groups_token_id (token_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- seed default main group + default mapping
INSERT INTO main_groups(name, description, status, created_at, updated_at)
SELECT 'default', '默认主组', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM main_groups WHERE name='default' LIMIT 1);

INSERT IGNORE INTO main_group_subgroups(main_group, subgroup, priority, created_at, updated_at)
VALUES('default', 'default', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- backfill users.main_group
UPDATE users SET main_group='default' WHERE TRIM(IFNULL(main_group, ''))='';

-- backfill token_groups from legacy user_groups (active tokens only)
INSERT IGNORE INTO token_groups(token_id, group_name, priority, created_at, updated_at)
SELECT t.id, ug.group_name, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM user_tokens t
JOIN user_groups ug ON ug.user_id=t.user_id
WHERE t.status=1;

-- ensure each active token has at least one group
INSERT IGNORE INTO token_groups(token_id, group_name, priority, created_at, updated_at)
SELECT t.id, 'default', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM user_tokens t
WHERE t.status=1
  AND NOT EXISTS (SELECT 1 FROM token_groups tg WHERE tg.token_id=t.id);

