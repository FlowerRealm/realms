-- 0006_single_endpoint_per_channel.sql: 每个 channel 仅保留一个 endpoint，并补齐缺失的默认 endpoint。

-- 1) 对于已有多个 endpoint 的 channel：
--    选出主 endpoint（priority DESC, id DESC），把其余 endpoint 的 credential/account 迁移过去，然后删除其余 endpoint。
CREATE TABLE IF NOT EXISTS tmp_endpoint_keep (
  channel_id BIGINT PRIMARY KEY,
  endpoint_id BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DELETE FROM tmp_endpoint_keep;

INSERT INTO tmp_endpoint_keep(channel_id, endpoint_id)
SELECT channel_id,
       CAST(SUBSTRING_INDEX(GROUP_CONCAT(id ORDER BY priority DESC, id DESC), ',', 1) AS UNSIGNED) AS endpoint_id
FROM upstream_endpoints
GROUP BY channel_id;

UPDATE openai_compatible_credentials c
JOIN upstream_endpoints e ON e.id=c.endpoint_id
JOIN tmp_endpoint_keep k ON k.channel_id=e.channel_id
SET c.endpoint_id=k.endpoint_id, c.updated_at=NOW()
WHERE c.endpoint_id<>k.endpoint_id;

UPDATE codex_oauth_accounts a
JOIN upstream_endpoints e ON e.id=a.endpoint_id
JOIN tmp_endpoint_keep k ON k.channel_id=e.channel_id
SET a.endpoint_id=k.endpoint_id, a.updated_at=NOW()
WHERE a.endpoint_id<>k.endpoint_id;

DELETE e FROM upstream_endpoints e
JOIN tmp_endpoint_keep k ON k.channel_id=e.channel_id
WHERE e.id<>k.endpoint_id;

DROP TABLE tmp_endpoint_keep;

-- 2) 补齐缺失 endpoint：为内置 codex_oauth 和 openai_compatible 创建默认 base_url。
INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
SELECT c.id, 'https://chatgpt.com/backend-api/codex', 1, 0, NOW(), NOW()
FROM upstream_channels c
WHERE c.type='codex_oauth'
  AND NOT EXISTS (SELECT 1 FROM upstream_endpoints e WHERE e.channel_id=c.id);

INSERT INTO upstream_endpoints(channel_id, base_url, status, priority, created_at, updated_at)
SELECT c.id, 'https://api.openai.com', 1, 0, NOW(), NOW()
FROM upstream_channels c
WHERE c.type='openai_compatible'
  AND NOT EXISTS (SELECT 1 FROM upstream_endpoints e WHERE e.channel_id=c.id);

-- 3) 加唯一约束，禁止一个 channel 多 endpoint。
ALTER TABLE upstream_endpoints
  DROP INDEX idx_upstream_endpoints_channel_id;

ALTER TABLE upstream_endpoints
  ADD UNIQUE KEY uk_upstream_endpoints_channel_id (channel_id);

