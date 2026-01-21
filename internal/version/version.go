// Package version 提供构建信息，便于 healthz 与日志输出版本指纹。
package version

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func Info() BuildInfo {
	return BuildInfo{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}
