-- 0045_usage_event_details.sql: usage_event_details 存储请求/响应明细（用于用量统计排障）。

CREATE TABLE IF NOT EXISTS `usage_event_details` (
  `usage_event_id` BIGINT PRIMARY KEY,
  `upstream_request_body` MEDIUMTEXT NULL,
  `upstream_response_body` MEDIUMTEXT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_usage_event_details_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

