package store

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	sqliteDriver "modernc.org/sqlite"
)

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == 1062 {
		return true
	}

	var sqliteErr *sqliteDriver.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 19, 1555, 2067:
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") || strings.Contains(msg, "unique constraint")
}
