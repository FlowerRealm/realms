package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := ensureDir(path); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmp != nil {
			_ = tmp.Close()
		}
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if perm != 0 {
		_ = tmp.Chmod(perm) // best-effort (esp. on Windows)
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	tmp = nil

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	tmpName = ""

	if runtime.GOOS != "windows" {
		df, err := os.Open(dir)
		if err == nil {
			_ = df.Sync()
			_ = df.Close()
		}
	}
	return nil
}
