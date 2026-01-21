package store

// Dialect 表示数据库方言，用于处理 MySQL/SQLite 的 SQL 语法差异。
type Dialect string

const (
	DialectMySQL  Dialect = "mysql"
	DialectSQLite Dialect = "sqlite"
)
