-- 0059_channel_group_pointers.sql: 按分组持久化调度器“渠道指针”运行态（每组独立）。

CREATE TABLE IF NOT EXISTS channel_group_pointers (
  group_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL DEFAULT 0,
  pinned TINYINT NOT NULL DEFAULT 0,
  moved_at_unix_ms BIGINT NOT NULL DEFAULT 0,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (group_id),
  KEY idx_channel_group_pointers_channel_id (channel_id),
  KEY idx_channel_group_pointers_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

