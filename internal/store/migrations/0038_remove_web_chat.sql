-- 0038_remove_web_chat.sql: 清理已移除的 Web 对话相关配置与数据。

DELETE FROM user_tokens WHERE name='chat';

DELETE FROM app_settings WHERE `key` IN (
  'chat_group_name',
  'feature_disable_web_chat',
  'search_searxng_enable',
  'search_searxng_base_url',
  'search_searxng_timeout',
  'search_searxng_max_results',
  'search_searxng_user_agent'
);

