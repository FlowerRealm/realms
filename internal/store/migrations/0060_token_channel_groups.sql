-- 0060_token_channel_groups.sql: 将 token_groups 更名为 token_channel_groups（更明确语义：Token 绑定的是“渠道组”）。
--
-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“表已创建但 schema_migrations 未记录”的状态。

CREATE TABLE IF NOT EXISTS token_channel_groups (
  token_id BIGINT NOT NULL,
  channel_group_name VARCHAR(64) NOT NULL,
  priority INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (token_id, channel_group_name),
  KEY idx_token_channel_groups_token_id (token_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- migrate existing data
INSERT IGNORE INTO token_channel_groups(token_id, channel_group_name, priority, created_at, updated_at)
SELECT token_id, group_name, priority, created_at, updated_at
FROM token_groups;

DROP TABLE IF EXISTS token_groups;
