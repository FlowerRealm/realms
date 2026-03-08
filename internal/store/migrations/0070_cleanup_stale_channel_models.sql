-- 0070_cleanup_stale_channel_models.sql: 删除不再对应 managed_models 的脏 channel_models 绑定。

DELETE cm
FROM channel_models cm
LEFT JOIN managed_models m ON m.public_id = cm.public_id
WHERE m.public_id IS NULL;
