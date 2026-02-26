package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakeNullIntRow struct {
	v   sql.NullInt64
	err error
}

func (r fakeNullIntRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return fmt.Errorf("unexpected dest len: %d", len(dest))
	}
	p, ok := dest[0].(*sql.NullInt64)
	if !ok {
		return fmt.Errorf("unexpected dest type: %T", dest[0])
	}
	*p = r.v
	return nil
}

type fakeLockConn struct {
	queries []string
	args    [][]any
	rows    []rowScanner
}

func (c *fakeLockConn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, errors.New("unexpected ExecContext call")
}

func (c *fakeLockConn) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	c.queries = append(c.queries, query)
	c.args = append(c.args, args)
	if len(c.rows) == 0 {
		return fakeNullIntRow{err: errors.New("no rows queued")}
	}
	r := c.rows[0]
	c.rows = c.rows[1:]
	return r
}

func TestAcquireMySQLMigrationLock_SuccessAndRelease(t *testing.T) {
	conn := &fakeLockConn{
		rows: []rowScanner{
			fakeNullIntRow{v: sql.NullInt64{Int64: 1, Valid: true}}, // GET_LOCK
			fakeNullIntRow{v: sql.NullInt64{Int64: 1, Valid: true}}, // RELEASE_LOCK
		},
	}

	release, err := acquireMySQLMigrationLock(context.Background(), conn, "realms.schema_migrations", 30*time.Second)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if err := release(context.Background()); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	if len(conn.queries) != 2 {
		t.Fatalf("unexpected query count: %d", len(conn.queries))
	}
}

func TestAcquireMySQLMigrationLock_Timeout(t *testing.T) {
	conn := &fakeLockConn{
		rows: []rowScanner{
			fakeNullIntRow{v: sql.NullInt64{Int64: 0, Valid: true}}, // GET_LOCK timeout
		},
	}

	_, err := acquireMySQLMigrationLock(context.Background(), conn, "realms.schema_migrations", 30*time.Second)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAcquireMySQLMigrationLock_SubSecondTimeoutRoundsUp(t *testing.T) {
	conn := &fakeLockConn{
		rows: []rowScanner{
			fakeNullIntRow{v: sql.NullInt64{Int64: 1, Valid: true}}, // GET_LOCK
			fakeNullIntRow{v: sql.NullInt64{Int64: 1, Valid: true}}, // RELEASE_LOCK
		},
	}

	release, err := acquireMySQLMigrationLock(context.Background(), conn, "realms.schema_migrations", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	_ = release(context.Background())

	if len(conn.args) < 1 || len(conn.args[0]) != 2 {
		t.Fatalf("unexpected GET_LOCK args: %#v", conn.args)
	}
	gotSeconds, ok := conn.args[0][1].(int)
	if !ok {
		t.Fatalf("unexpected arg type: %T", conn.args[0][1])
	}
	if gotSeconds != 1 {
		t.Fatalf("expected timeout seconds=1, got %d", gotSeconds)
	}
}
