CREATE TABLE IF NOT EXISTS openai_object_refs (
  object_type VARCHAR(32) NOT NULL,
  object_id VARCHAR(128) NOT NULL,
  user_id BIGINT NOT NULL,
  token_id BIGINT NOT NULL DEFAULT 0,
  selection_json MEDIUMTEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (object_type, object_id),
  KEY idx_openai_object_refs_user_type_created_at (user_id, object_type, created_at),
  KEY idx_openai_object_refs_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
