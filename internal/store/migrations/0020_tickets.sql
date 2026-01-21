-- 0020_tickets.sql: 工单系统（工单 + 消息线程 + 附件）。

CREATE TABLE IF NOT EXISTS `tickets` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `subject` VARCHAR(200) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `last_message_at` DATETIME NOT NULL,
  `closed_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_tickets_user_id_id` (`user_id`, `id`),
  KEY `idx_tickets_status_last_message_at` (`status`, `last_message_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `ticket_messages` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `ticket_id` BIGINT NOT NULL,
  `actor_type` VARCHAR(16) NOT NULL,
  `actor_user_id` BIGINT NULL,
  `body` TEXT NOT NULL,
  `created_at` DATETIME NOT NULL,
  KEY `idx_ticket_messages_ticket_id_id` (`ticket_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `ticket_attachments` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `ticket_id` BIGINT NOT NULL,
  `message_id` BIGINT NOT NULL,
  `uploader_user_id` BIGINT NULL,
  `original_name` VARCHAR(255) NOT NULL,
  `content_type` VARCHAR(255) NULL,
  `size_bytes` BIGINT NOT NULL,
  `sha256` VARBINARY(32) NULL,
  `storage_rel_path` VARCHAR(512) NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL,
  KEY `idx_ticket_attachments_ticket_id_id` (`ticket_id`, `id`),
  KEY `idx_ticket_attachments_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

