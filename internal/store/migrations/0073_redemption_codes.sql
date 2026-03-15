CREATE TABLE IF NOT EXISTS `redemption_codes` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `batch_name` VARCHAR(128) NOT NULL,
  `code` VARCHAR(128) NOT NULL,
  `distribution_mode` VARCHAR(16) NOT NULL,
  `reward_type` VARCHAR(16) NOT NULL,
  `subscription_plan_id` BIGINT NULL,
  `balance_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `max_redemptions` INT NOT NULL DEFAULT 1,
  `redeemed_count` INT NOT NULL DEFAULT 0,
  `expires_at` DATETIME NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_by` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_redemption_codes_code` (`code`),
  KEY `idx_redemption_codes_batch_name` (`batch_name`),
  KEY `idx_redemption_codes_distribution_mode` (`distribution_mode`),
  KEY `idx_redemption_codes_reward_type` (`reward_type`),
  KEY `idx_redemption_codes_status` (`status`),
  KEY `idx_redemption_codes_subscription_plan_id` (`subscription_plan_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `redemption_code_redemptions` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `code_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `reward_type` VARCHAR(16) NOT NULL,
  `balance_usd` DECIMAL(20,6) NOT NULL DEFAULT 0,
  `subscription_id` BIGINT NULL,
  `subscription_activation_mode` VARCHAR(16) NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_redemption_code_redemptions_code_user` (`code_id`, `user_id`),
  KEY `idx_redemption_code_redemptions_user_id` (`user_id`),
  KEY `idx_redemption_code_redemptions_subscription_id` (`subscription_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
