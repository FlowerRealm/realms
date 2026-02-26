// Package store 实现内置 SQL 迁移，保证单体部署时可自举初始化。
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type MigrationOptions struct {
	LockName    string
	LockTimeout time.Duration
}

const (
	defaultMigrationLockName    = "realms.schema_migrations"
	defaultMigrationLockTimeout = 30 * time.Second
)

type rowScanner interface {
	Scan(dest ...any) error
}

type mysqlLockConn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
}

type migrationConn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type sqlConnWrapper struct {
	*sql.Conn
}

func (c sqlConnWrapper) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.Conn.ExecContext(ctx, query, args...)
}

func (c sqlConnWrapper) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return c.Conn.QueryRowContext(ctx, query, args...)
}

func (c sqlConnWrapper) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return c.Conn.BeginTx(ctx, opts)
}

func ApplyMigrations(ctx context.Context, db *sql.DB, opt MigrationOptions) error {
	if db == nil {
		return errors.New("db 为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	lockName := strings.TrimSpace(opt.LockName)
	if lockName == "" {
		lockName = defaultMigrationLockName
	}
	lockTimeout := opt.LockTimeout
	if lockTimeout == 0 {
		lockTimeout = defaultMigrationLockTimeout
	}
	if lockTimeout < 0 {
		return fmt.Errorf("migration lock timeout 不合法: %s", lockTimeout.String())
	}

	start := time.Now()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("获取迁移连接失败: %w", err)
	}
	defer conn.Close()

	c := sqlConnWrapper{Conn: conn}

	release, err := acquireMySQLMigrationLock(ctx, c, lockName, lockTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(ctx); err != nil {
			slog.Warn("释放迁移锁失败", "lock", lockName, "err", err)
		}
	}()

	slog.Info("开始执行数据库迁移", "lock", lockName)

	if _, err := c.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(255) PRIMARY KEY,
  applied_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`); err != nil {
		return fmt.Errorf("创建 schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("读取 migrations 目录: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	appliedCount := 0
	for _, file := range files {
		applied, err := isMigrationApplied(ctx, c, file)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		b, err := migrationsFS.ReadFile("migrations/" + file)
		if err != nil {
			return fmt.Errorf("读取迁移 %s: %w", file, err)
		}
		slog.Info("应用迁移", "version", file)
		if err := applyMigration(ctx, c, file, string(b)); err != nil {
			return err
		}
		appliedCount++
	}

	elapsed := time.Since(start)
	if appliedCount == 0 {
		slog.Info("数据库迁移已是最新", "elapsed", elapsed.String())
	} else {
		slog.Info("数据库迁移完成", "applied", appliedCount, "elapsed", elapsed.String())
	}
	return nil
}

func isMigrationApplied(ctx context.Context, conn migrationConn, version string) (bool, error) {
	var v string
	err := conn.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version=?`, version).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("查询迁移状态: %w", err)
	}
	return true, nil
}

func applyMigration(ctx context.Context, conn migrationConn, version, sqlText string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始迁移事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := splitSQLStatements(sqlText)
	for i, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("执行迁移 %s (stmt %d/%d): %w", version, i+1, len(stmts), err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, CURRENT_TIMESTAMP)`, version); err != nil {
		return fmt.Errorf("记录迁移 %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移 %s: %w", version, err)
	}
	return nil
}

func acquireMySQLMigrationLock(ctx context.Context, conn mysqlLockConn, name string, timeout time.Duration) (func(context.Context) error, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("迁移锁名为空")
	}
	timeoutSeconds := int(timeout.Seconds())
	if timeout > 0 && timeoutSeconds == 0 {
		timeoutSeconds = 1
	}

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, name, timeoutSeconds).Scan(&got); err != nil {
		return nil, fmt.Errorf("获取迁移锁失败: %w", err)
	}
	if !got.Valid {
		return nil, errors.New("获取迁移锁失败: GET_LOCK 返回 NULL")
	}
	switch got.Int64 {
	case 1:
		return func(ctx context.Context) error {
			var released sql.NullInt64
			if err := conn.QueryRowContext(ctx, `SELECT RELEASE_LOCK(?)`, name).Scan(&released); err != nil {
				return fmt.Errorf("释放迁移锁失败: %w", err)
			}
			if !released.Valid {
				return errors.New("释放迁移锁失败: RELEASE_LOCK 返回 NULL")
			}
			if released.Int64 != 1 {
				return fmt.Errorf("释放迁移锁失败: RELEASE_LOCK=%d", released.Int64)
			}
			return nil
		}, nil
	case 0:
		return nil, fmt.Errorf("等待迁移锁超时: %s（timeout=%s）", name, timeout.String())
	default:
		return nil, fmt.Errorf("获取迁移锁失败: GET_LOCK=%d", got.Int64)
	}
}

func splitSQLStatements(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		stmt := strings.TrimSpace(p)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}
