// Package store 负责数据库连接与迁移，避免业务层直接处理 schema 细节。
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

func OpenDB(env string, driver string, mysqlDSN string, sqlitePath string) (*sql.DB, Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite":
		db, err := OpenSQLite(sqlitePath)
		if err != nil {
			return nil, "", err
		}
		return db, DialectSQLite, nil
	case "mysql":
		db, err := OpenMySQL(env, mysqlDSN)
		if err != nil {
			return nil, "", err
		}
		return db, DialectMySQL, nil
	default:
		return nil, "", fmt.Errorf("不支持的 db.driver：%s", driver)
	}
}

func OpenMySQL(env string, dsn string) (*sql.DB, error) {
	db, err := openMySQL(dsn)
	if err != nil {
		return nil, err
	}

	if env == "dev" {
		if err := pingMySQLInDev(db, dsn); err != nil {
			_ = db.Close()
			return nil, err
		}
		return db, nil
	}

	if err := pingMySQLOnce(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func OpenSQLite(path string) (*sql.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite_path 不能为空")
	}

	// 允许通过 query 参数传递 driver 选项（例如 ?_busy_timeout=30000），这里需要先确保文件目录存在。
	filePath := path
	if i := strings.IndexByte(filePath, '?'); i >= 0 {
		filePath = filePath[:i]
	}
	if filePath != "" && filePath != ":memory:" && !strings.HasPrefix(filePath, "file::memory:") {
		dir := filepath.Dir(filePath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建 sqlite 数据目录失败: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql.Open(sqlite): %w", err)
	}
	// SQLite 多连接写入容易触发锁竞争；单机默认收敛为单连接更稳。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db.Ping(sqlite): %w", err)
	}

	// WAL 模式是数据库级别持久设置，执行一次即可对后续连接生效。
	_, _ = db.Exec(`PRAGMA journal_mode=WAL`)
	return db, nil
}

func openMySQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	return db, nil
}

func pingMySQLOnce(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("db.Ping: %w", err)
	}
	return nil
}

func pingMySQLInDev(db *sql.DB, dsn string) error {
	const (
		maxWait    = 30 * time.Second
		maxBackoff = 2 * time.Second
	)

	deadline := time.Now().Add(maxWait)
	backoff := 200 * time.Millisecond
	waitLogged := false
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err

		// 数据库不存在：开发环境自动创建一次后继续尝试。
		if isUnknownDatabaseError(err) {
			if err2 := createDatabaseIfMissing(dsn); err2 != nil {
				return errors.Join(fmt.Errorf("db.Ping: %w", err), err2)
			}
			slog.Info("检测到 MySQL 数据库不存在，已自动创建并重试连接")
			continue
		}

		// 明确的配置错误：别浪费时间重试。
		if isAccessDeniedError(err) {
			return fmt.Errorf("db.Ping: %w", err)
		}

		// 其他连接类错误：MySQL 容器常见启动竞态，等待就绪。
		if !waitLogged {
			slog.Info("等待 MySQL 就绪（dev）", "timeout", maxWait.String())
			waitLogged = true
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	if lastErr == nil {
		lastErr = driver.ErrBadConn
	}
	return fmt.Errorf("db.Ping: %w", lastErr)
}

func isUnknownDatabaseError(err error) bool {
	var myErr *mysql.MySQLError
	if !errors.As(err, &myErr) {
		return false
	}
	return myErr.Number == 1049
}

func isAccessDeniedError(err error) bool {
	var myErr *mysql.MySQLError
	if !errors.As(err, &myErr) {
		return false
	}
	// 1045: ER_ACCESS_DENIED_ERROR
	// 1044: ER_DBACCESS_DENIED_ERROR
	return myErr.Number == 1045 || myErr.Number == 1044
}

func createDatabaseIfMissing(dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("mysql.ParseDSN: %w", err)
	}
	if cfg.DBName == "" {
		return errors.New("dsn 未包含数据库名")
	}

	adminCfg := *cfg
	adminCfg.DBName = ""

	adminDB, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("sql.Open(admin): %w", err)
	}
	defer adminDB.Close()

	charset := cfg.Params["charset"]
	if !isSafeMySQLWord(charset) {
		charset = ""
	}
	collation := cfg.Params["collation"]
	if !isSafeMySQLWord(collation) {
		collation = ""
	}

	escapedDBName := strings.ReplaceAll(cfg.DBName, "`", "``")
	stmt := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", escapedDBName)
	if charset != "" {
		stmt += " DEFAULT CHARACTER SET " + charset
	}
	if collation != "" {
		stmt += " DEFAULT COLLATE " + collation
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := adminDB.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	return nil
}

func isSafeMySQLWord(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '_' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		return false
	}
	return true
}
