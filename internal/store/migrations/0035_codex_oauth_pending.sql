-- 0035_codex_oauth_pending.sql: 持久化 Codex OAuth pending state（支持“粘贴回调 URL”与多实例/重启场景）。

CREATE TABLE IF NOT EXISTS `codex_oauth_pending` (
  `state` VARCHAR(128) PRIMARY KEY,
  `endpoint_id` BIGINT NOT NULL,
  `actor_user_id` BIGINT NOT NULL,
  `code_verifier` VARCHAR(255) NOT NULL,
  `created_at` DATETIME NOT NULL,
  KEY `idx_codex_oauth_pending_created_at` (`created_at`),
  KEY `idx_codex_oauth_pending_endpoint_id` (`endpoint_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

