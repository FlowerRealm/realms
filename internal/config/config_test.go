package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"realms/internal/config"
)

func TestLoad_DefaultsToSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  addr: \":8080\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("expected db.driver=sqlite, got %q", cfg.DB.Driver)
	}
	if cfg.DB.SQLitePath == "" {
		t.Fatalf("expected sqlite_path to be set")
	}
}

func TestLoad_InferMySQLFromDSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := "" +
		"server:\n" +
		"  addr: \":8080\"\n" +
		"db:\n" +
		"  dsn: \"user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true\"\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.DB.Driver != "mysql" {
		t.Fatalf("expected db.driver=mysql, got %q", cfg.DB.Driver)
	}
	if cfg.DB.DSN == "" {
		t.Fatalf("expected dsn to be set")
	}
}

func TestLoad_EnvOverridesDBDriver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := "" +
		"server:\n" +
		"  addr: \":8080\"\n" +
		"db:\n" +
		"  driver: mysql\n" +
		"  dsn: \"user:pass@tcp(127.0.0.1:3306)/realms?parseTime=true\"\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("REALMS_DB_DRIVER", "sqlite")
	t.Setenv("REALMS_SQLITE_PATH", filepath.Join(dir, "realms.db"))

	cfg, err := config.Load(path)
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
