package store

func forUpdateClause(d Dialect) string {
	if d == DialectMySQL {
		return " FOR UPDATE"
	}
	return ""
}

func insertIgnoreVerb(d Dialect) string {
	if d == DialectSQLite {
		return "INSERT OR IGNORE"
	}
	return "INSERT IGNORE"
}
