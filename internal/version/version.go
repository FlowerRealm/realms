// Package version 提供构建信息，便于 healthz 与日志输出版本指纹。
package version

import (
	_ "embed"
	"strings"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

//go:embed version.txt
var embeddedVersion string

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func Info() BuildInfo {
	v := strings.TrimSpace(Version)
	if v == "" || v == "dev" {
		if ev := strings.TrimSpace(embeddedVersion); ev != "" {
			v = ev
		}
	}
	return BuildInfo{
		Version: v,
		Commit:  strings.TrimSpace(Commit),
		Date:    strings.TrimSpace(Date),
	}
}
