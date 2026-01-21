-- 0008_channel_models.sql: 渠道绑定模型（channel_models）。

CREATE TABLE IF NOT EXISTS `channel_models` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `channel_id` BIGINT NOT NULL,
  `public_id` VARCHAR(128) NOT NULL,
  `upstream_model` VARCHAR(128) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_channel_models_channel_public` (`channel_id`, `public_id`),
  KEY `idx_channel_models_public_id` (`public_id`),
  KEY `idx_channel_models_status` (`status`),
  KEY `idx_channel_models_channel_id` (`channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

