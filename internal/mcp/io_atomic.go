package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func isIgnorableDirSyncErr(err error) bool {
	// Some platforms/filesystems don't support fsync on directories (e.g. macOS often returns EINVAL).
	// Treat these as best-effort so we don't break normal writes.
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EPERM)
}

func syncDirBestEffort(dir string) error {
	df, err := os.Open(dir)
	if err != nil {
		return nil
	}
	syncErr := df.Sync()
	closeErr := df.Close()
	if syncErr != nil && !isIgnorableDirSyncErr(syncErr) {
		return fmt.Errorf("fsync dir: %w", syncErr)
	}
	if closeErr != nil && !isIgnorableDirSyncErr(closeErr) {
		return fmt.Errorf("close dir: %w", closeErr)
	}
	return nil
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
		if err := syncDirBestEffort(dir); err != nil {
			return err
		}
	}
	return nil
}
