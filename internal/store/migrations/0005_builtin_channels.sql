-- 0005_builtin_channels.sql: 初始化内置 upstream_channels，避免用户手工创建。

INSERT INTO upstream_channels(type, name, status, priority, promotion, created_at, updated_at)
SELECT 'codex_oauth', 'Codex OAuth', 1, 0, 0, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM upstream_channels WHERE type='codex_oauth' LIMIT 1);
