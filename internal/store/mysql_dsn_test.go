package store

import (
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestNormalizeMySQLDSN_ForcesUTCAndTimeZone(t *testing.T) {
	t.Parallel()

	got, err := normalizeMySQLDSN("user:pass@tcp(127.0.0.1:3306)/realms?charset=utf8mb4")
	if err != nil {
		t.Fatalf("normalizeMySQLDSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(got)
	if err != nil {
		t.Fatalf("mysql.ParseDSN(normalized): %v", err)
	}
	if !cfg.ParseTime {
		t.Fatalf("ParseTime = false, want true")
	}
	if cfg.Loc != time.UTC {
		loc := "<nil>"
		if cfg.Loc != nil {
			loc = cfg.Loc.String()
		}
		t.Fatalf("Loc = %s, want UTC", loc)
	}
	if cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("time_zone = %q, want %q", cfg.Params["time_zone"], "'+00:00'")
	}
}
