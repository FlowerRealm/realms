-- 0057_drop_user_groups.sql: 下线并删除遗留 user_groups 表（已不再作为运行时 SSOT）。
--
-- ⚠️ 注意：这是不可逆操作（DROP TABLE）。

DROP TABLE IF EXISTS user_groups;

