-- 0012_usage_events_subscription_id.sql: usage_events 增加 subscription_id（用量按订阅归属；扣费按最早到期优先）。

ALTER TABLE `usage_events`
  ADD COLUMN `subscription_id` BIGINT NULL AFTER `user_id`,
  ADD KEY `idx_usage_events_user_subscription_state_time` (`user_id`, `subscription_id`, `state`, `time`);

-- 旧数据回填：将历史用量事件归属到“事件发生时刻”处于生效态（start_at<=time<end_at）的、最早到期的订阅。
UPDATE `usage_events` ue
SET ue.subscription_id = (
  SELECT us.id
  FROM user_subscriptions us
  WHERE us.user_id=ue.user_id
    AND us.status=1
    AND us.start_at <= ue.time
    AND us.end_at > ue.time
  ORDER BY us.end_at ASC, us.id ASC
  LIMIT 1
)
WHERE ue.subscription_id IS NULL;
