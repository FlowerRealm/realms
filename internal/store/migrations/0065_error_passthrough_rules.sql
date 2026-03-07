CREATE TABLE IF NOT EXISTS error_passthrough_rules (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  priority INT NOT NULL DEFAULT 100,
  error_codes_json JSON NOT NULL,
  keywords_json JSON NOT NULL,
  match_mode VARCHAR(16) NOT NULL DEFAULT 'any',
  platforms_json JSON NOT NULL,
  passthrough_code TINYINT(1) NOT NULL DEFAULT 0,
  response_code INT NULL,
  passthrough_body TINYINT(1) NOT NULL DEFAULT 0,
  custom_message TEXT NULL,
  skip_monitoring TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_error_passthrough_rules_enabled ON error_passthrough_rules(enabled);
CREATE INDEX idx_error_passthrough_rules_priority ON error_passthrough_rules(priority);
