CREATE TABLE IF NOT EXISTS session_bindings (
  user_id BIGINT NOT NULL,
  route_key_hash VARCHAR(128) NOT NULL,
  payload_json MEDIUMTEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, route_key_hash),
  KEY idx_session_bindings_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
