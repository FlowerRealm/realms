package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func loadDotEnv() error {
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}

	for _, p := range paths {
		loaded, err := loadDotEnvFile(p)
		if err != nil {
			return fmt.Errorf("加载 .env 失败（%s）: %w", p, err)
		}
		if loaded {
			break
		}
	}
	return nil
}

func loadDotEnvFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64<<10), 1<<20)

	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimSpace(strings.TrimRight(s.Text(), "\r"))
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if strings.HasPrefix(raw, "export ") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "export "))
		}
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return true, fmt.Errorf("第 %d 行缺少 '='", lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return true, fmt.Errorf("第 %d 行 key 为空", lineNo)
		}
		if !isDotEnvKey(key) {
			return true, fmt.Errorf("第 %d 行 key 不合法: %q", lineNo, key)
		}

		value = strings.TrimSpace(value)
		unquoted, err := unquoteDotEnvValue(value)
		if err != nil {
			return true, fmt.Errorf("第 %d 行 value 不合法（%s）: %w", lineNo, key, err)
		}
		if err := os.Setenv(key, unquoted); err != nil {
			return true, fmt.Errorf("设置环境变量失败（%s）: %w", key, err)
		}
	}
	if err := s.Err(); err != nil {
		return true, err
	}
	return true, nil
}

func isDotEnvKey(key string) bool {
	for i := range key {
		c := key[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func unquoteDotEnvValue(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if len(v) < 2 {
		return v, nil
	}
	if v[0] == '"' && v[len(v)-1] == '"' {
		out, err := strconv.Unquote(v)
		if err != nil {
			return "", err
		}
		return out, nil
	}
	if v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1], nil
	}
	return v, nil
}
