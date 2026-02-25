-- 清理残留的渠道-模型绑定（channel_models）
--
-- 场景：
-- - 你已从模型目录（managed_models）删除某个模型；
-- - 但旧渠道仍残留该 public_id 的绑定（channel_models），导致管理端渠道配置里还能看到它。
--
-- ⚠️ 风险提示：
-- - 本脚本包含 DELETE 语句，会永久删除数据；请先备份数据库。
-- - 仅清理“public_id 在 managed_models 中已不存在”的残留绑定。
--
-- 使用方式：
-- - SQLite：执行本文件内 “SQLite” 小节
-- - MySQL：建议直接使用单独脚本 `scripts/cleanup_stale_channel_models.mysql.sql`

-- =========================================================
-- SQLite
-- =========================================================

-- 1) 预览：残留数量
SELECT COUNT(*) AS stale_count
FROM channel_models cm
LEFT JOIN managed_models m
  ON m.public_id = TRIM(cm.public_id)
WHERE m.id IS NULL;

-- 2) 预览：残留明细
SELECT cm.id, cm.channel_id, cm.public_id, cm.upstream_model, cm.status, cm.created_at, cm.updated_at
FROM channel_models cm
LEFT JOIN managed_models m
  ON m.public_id = TRIM(cm.public_id)
WHERE m.id IS NULL
ORDER BY cm.channel_id, cm.public_id, cm.id;

-- 3) 执行清理：删除残留绑定（建议先确认上面的预览结果）
BEGIN;
DELETE FROM channel_models
WHERE id IN (
  SELECT cm.id
  FROM channel_models cm
  LEFT JOIN managed_models m
    ON m.public_id = TRIM(cm.public_id)
  WHERE m.id IS NULL
);
COMMIT;

-- 4) 复查
SELECT COUNT(*) AS stale_count_after
FROM channel_models cm
LEFT JOIN managed_models m
  ON m.public_id = TRIM(cm.public_id)
WHERE m.id IS NULL;

-- =========================================================
-- MySQL
-- =========================================================

-- 1) 预览：残留数量
-- SELECT COUNT(*) AS stale_count
-- FROM channel_models cm
-- LEFT JOIN managed_models m ON m.public_id = TRIM(cm.public_id)
-- WHERE m.id IS NULL;

-- 2) 预览：残留明细
-- SELECT cm.id, cm.channel_id, cm.public_id, cm.upstream_model, cm.status, cm.created_at, cm.updated_at
-- FROM channel_models cm
-- LEFT JOIN managed_models m ON m.public_id = TRIM(cm.public_id)
-- WHERE m.id IS NULL
-- ORDER BY cm.channel_id, cm.public_id, cm.id;

-- 3) 执行清理：删除残留绑定
-- START TRANSACTION;
-- DELETE cm
-- FROM channel_models cm
-- LEFT JOIN managed_models m ON m.public_id = TRIM(cm.public_id)
-- WHERE m.id IS NULL;
-- COMMIT;
