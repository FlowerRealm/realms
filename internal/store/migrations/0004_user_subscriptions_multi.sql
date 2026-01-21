-- 0004_user_subscriptions_multi.sql: 允许同一用户并发多条订阅记录（不再用 end_at 叠加延长）。

-- 移除“一用户一条订阅”的唯一约束，改为按 user_id 维度支持多条 active 订阅。
ALTER TABLE user_subscriptions
  DROP INDEX uk_user_subscriptions_user;

-- 支持按 user_id + status + end_at 高效查询有效订阅（以及按 end_at 排序）。
ALTER TABLE user_subscriptions
  ADD KEY idx_user_subscriptions_user_status_end_at (user_id, status, end_at);

