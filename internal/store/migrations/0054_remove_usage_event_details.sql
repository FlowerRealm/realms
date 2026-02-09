-- 0054_remove_usage_event_details.sql: 移除 usage_event_details（请求/响应 body 明细）表。
--
-- 注意：MySQL 的 DDL 语句会隐式提交事务；一旦迁移中途失败，可能出现“表已删除但 schema_migrations 未记录”的状态。
-- 为了让迁移可重入，这里使用 IF EXISTS 避免重复执行报错。

DROP TABLE IF EXISTS `usage_event_details`;

