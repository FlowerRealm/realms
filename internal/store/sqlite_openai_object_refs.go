package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func ensureSQLiteOpenAIObjectRefsTable(db *sql.DB) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS openai_object_refs (
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  user_id INTEGER NOT NULL,
  token_id INTEGER NOT NULL DEFAULT 0,
  selection_json TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (object_type, object_id)
)
`); err != nil {
		return fmt.Errorf("创建 openai_object_refs 表失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_openai_object_refs_user_type_created_at ON openai_object_refs (user_id, object_type, created_at)`); err != nil {
		return fmt.Errorf("创建 openai_object_refs user/type/created_at 索引失败: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_openai_object_refs_user_id ON openai_object_refs (user_id)`); err != nil {
		return fmt.Errorf("创建 openai_object_refs user_id 索引失败: %w", err)
	}
	return nil
}
