-- 0070_cleanup_stale_channel_models.sql: 将已启用但不再对应 managed_models 的脏 channel_models 绑定降为禁用。

UPDATE channel_models cm
LEFT JOIN managed_models m ON m.public_id = cm.public_id
SET cm.status = 0,
    cm.updated_at = CURRENT_TIMESTAMP
WHERE cm.status = 1
  AND m.public_id IS NULL;
