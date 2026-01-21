-- 0007_managed_models.sql: 模型目录（白名单/别名映射/上游绑定）。

CREATE TABLE IF NOT EXISTS `managed_models` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `public_id` VARCHAR(128) NOT NULL,
  `upstream_model` VARCHAR(128) NOT NULL,
  `owned_by` VARCHAR(64) NULL,
  `description` TEXT NULL,
  `upstream_type` VARCHAR(32) NOT NULL,
  `upstream_channel_id` BIGINT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_managed_models_public_id` (`public_id`),
  KEY `idx_managed_models_status` (`status`),
  KEY `idx_managed_models_upstream_type` (`upstream_type`),
  KEY `idx_managed_models_channel_id` (`upstream_channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

