package config_test

import (
	"testing"

	"realms/internal/config"
)

func TestLoad_DefaultsToSQLite(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_DB_DSN", "")
	t.Setenv("REALMS_SQLITE_PATH", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("expected db.driver=sqlite, got %q", cfg.DB.Driver)
	}
	if cfg.DB.SQLitePath == "" {
		t.Fatalf("expected sqlite_path to be set")
	}
}

func TestLoad_InferMySQLFromDSN(t *testing.T) {
	t.Setenv("REALMS_DB_DRIVER", "")
	t.Setenv("REALMS_SQLITE_PATH", "")
	t.Setenv("REALMS_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.Driver != "mysql" {
		t.Fatalf("expected db.driver=mysql, got %q", cfg.DB.Driver)
	}
	if cfg.DB.DSN == "" {
		t.Fatalf("expected dsn to be set")
	}
}

func TestLoad_EnvOverridesDBDriver(t *testing.T) {
	t.Setenv("REALMS_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true")
	t.Setenv("REALMS_DB_DRIVER", "sqlite")
	t.Setenv("REALMS_SQLITE_PATH", "./data/test.db?_busy_timeout=30000")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("expected db.driver=sqlite, got %q", cfg.DB.Driver)
	}
	if cfg.DB.SQLitePath == "" {
		t.Fatalf("expected sqlite_path to be set")
	}
}
