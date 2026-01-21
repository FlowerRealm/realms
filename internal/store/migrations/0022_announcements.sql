-- 0022_announcements.sql: 公告系统（announcements + per-user read marks）。

CREATE TABLE IF NOT EXISTS `announcements` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `title` VARCHAR(200) NOT NULL,
  `body` TEXT NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  KEY `idx_announcements_status_created` (`status`, `created_at`),
  KEY `idx_announcements_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `announcement_reads` (
  `user_id` BIGINT NOT NULL,
  `announcement_id` BIGINT NOT NULL,
  `read_at` DATETIME NOT NULL,
  PRIMARY KEY (`user_id`, `announcement_id`),
  KEY `idx_announcement_reads_announcement_id` (`announcement_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

