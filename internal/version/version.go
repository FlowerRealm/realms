// Package version 提供构建信息，便于 healthz 与日志输出版本指纹。
package version

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

type BuildInfo struct {
	Version string
	Date    string
}

var (
	Version = "dev"
	Date    = "unknown"
)

func Info() BuildInfo {
	bi, _ := debug.ReadBuildInfo()

	v := strings.TrimSpace(Version)
	if v == "" || v == "dev" {
		if bi != nil {
			mainVersion := strings.TrimSpace(bi.Main.Version)
			if mainVersion != "" && mainVersion != "(devel)" {
				v = mainVersion
			}
		}
		if v == "" && !isTestBinary() {
			if tag := gitNearestTag(); tag != "" {
				v = tag
			}
		}
		if v == "" {
			v = "dev"
		}
	}

	d := strings.TrimSpace(Date)
	if d == "" || d == "unknown" {
		if bi != nil {
			for _, s := range bi.Settings {
				if s.Key == "vcs.time" {
					if t := strings.TrimSpace(s.Value); t != "" {
						d = t
						break
					}
				}
			}
		}
	}
	if d == "" {
		d = "unknown"
	}
	return BuildInfo{
		Version: v,
		Date:    d,
	}
}

func isTestBinary() bool {
	return flag.Lookup("test.v") != nil
}

func gitNearestTag() string {
	gitDir := findGitRoot()
	if gitDir == "" {
		return ""
	}
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--abbrev=0")
	cmd.Dir = gitDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	tag := strings.TrimSpace(string(out))
	if tag == "" {
		return ""
	}
	return tag
}

func findGitRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := filepath.Clean(wd)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
