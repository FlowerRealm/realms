// Package tickets 提供工单系统的附件存储与通知等基础能力。
package tickets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Storage struct {
	baseDir string
}

func NewStorage(baseDir string) *Storage {
	return &Storage{baseDir: strings.TrimSpace(baseDir)}
}

func (s *Storage) BaseDir() string {
	return s.baseDir
}

type SaveResult struct {
	RelPath   string
	SizeBytes int64
}

func (s *Storage) Save(now time.Time, src io.Reader) (SaveResult, error) {
	if strings.TrimSpace(s.baseDir) == "" {
		return SaveResult{}, errors.New("附件目录未配置")
	}

	now = now.UTC()
	subDir := now.Format("20060102")
	name, err := randomToken(16)
	if err != nil {
		return SaveResult{}, err
	}

	finalRel := filepath.ToSlash(filepath.Join(subDir, name+".bin"))
	finalPath, err := s.resolve(finalRel)
	if err != nil {
		return SaveResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return SaveResult{}, fmt.Errorf("创建附件目录失败: %w", err)
	}

	tmpPath := finalPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return SaveResult{}, fmt.Errorf("创建附件文件失败: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	n, err := io.Copy(f, src)
	if err != nil {
		_ = os.Remove(tmpPath)
		return SaveResult{}, fmt.Errorf("写入附件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return SaveResult{}, fmt.Errorf("关闭附件文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return SaveResult{}, fmt.Errorf("落盘附件失败: %w", err)
	}
	return SaveResult{RelPath: finalRel, SizeBytes: n}, nil
}

func (s *Storage) Resolve(relPath string) (string, error) {
	return s.resolve(relPath)
}

func (s *Storage) resolve(relPath string) (string, error) {
	if strings.TrimSpace(s.baseDir) == "" {
		return "", errors.New("附件目录未配置")
	}
	rel := strings.TrimSpace(relPath)
	if rel == "" {
		return "", errors.New("附件路径为空")
	}
	if strings.Contains(rel, "\x00") {
		return "", errors.New("附件路径非法")
	}
	if strings.Contains(rel, "\\") {
		return "", errors.New("附件路径非法")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || cleanRel == string(filepath.Separator) {
		return "", errors.New("附件路径非法")
	}
	if filepath.IsAbs(cleanRel) {
		return "", errors.New("附件路径非法")
	}
	if strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("附件路径非法")
	}

	base := filepath.Clean(s.baseDir)
	full := filepath.Clean(filepath.Join(base, cleanRel))
	prefix := base + string(filepath.Separator)
	if full != base && !strings.HasPrefix(full, prefix) {
		return "", errors.New("附件路径非法")
	}
	return full, nil
}

func randomToken(n int) (string, error) {
	if n < 16 {
		n = 16
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
