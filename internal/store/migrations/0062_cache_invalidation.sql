-- 0062_cache_invalidation.sql: 多实例缓存一致性（DB version polling）
--
-- 说明：
-- - cache_invalidation 记录关键缓存的 version；管理面写操作 bump version；
--   数据面实例定期轮询 version 变化并主动失效本地缓存。

CREATE TABLE IF NOT EXISTS cache_invalidation (
  cache_key VARCHAR(64) PRIMARY KEY,
  version BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

