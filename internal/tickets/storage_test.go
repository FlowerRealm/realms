package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorageResolve(t *testing.T) {
	base := t.TempDir()
	s := NewStorage(base)

	okCases := []string{
		"20260101/abc.bin",
		"a/../b", // clean 后落在 base 内即可
	}
	for _, rel := range okCases {
		got, err := s.Resolve(rel)
		if err != nil {
			t.Fatalf("expected resolve ok for %q, got err=%v", rel, err)
		}
		if got == "" {
			t.Fatalf("expected non-empty path for %q", rel)
		}
		got = filepath.Clean(got)
		baseClean := filepath.Clean(base) + string(os.PathSeparator)
		if !strings.HasPrefix(got, baseClean) {
			t.Fatalf("expected %q within base %q", got, base)
		}
	}

	badCases := []string{
		"",
		"..",
		"../x",
		"../../etc/passwd",
		"/etc/passwd",
		"\\etc\\passwd",
		"\x00",
	}
	for _, rel := range badCases {
		if _, err := s.Resolve(rel); err == nil {
			t.Fatalf("expected resolve error for %q", rel)
		}
	}
}

func TestStorageSaveRespectsLimit(t *testing.T) {
	base := t.TempDir()
	s := NewStorage(base)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	res, err := s.Save(now, strings.NewReader("abc"), 3)
	if err != nil {
		t.Fatalf("expected save ok, got err=%v", err)
	}
	if res.SizeBytes != 3 {
		t.Fatalf("unexpected size: %d", res.SizeBytes)
	}
	if !strings.HasPrefix(res.RelPath, "20260101/") {
		t.Fatalf("unexpected rel path: %q", res.RelPath)
	}
	full, err := s.Resolve(res.RelPath)
	if err != nil {
		t.Fatalf("resolve saved path: %v", err)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(b) != "abc" {
		t.Fatalf("unexpected file content: %q", string(b))
	}

	_, err = s.Save(now, strings.NewReader("abcd"), 3)
	if err == nil {
		t.Fatalf("expected oversize error")
	}
	entries, err := os.ReadDir(filepath.Join(base, "20260101"))
	if err == nil && len(entries) != 1 {
		// 第一次 Save 已经创建了 1 个文件；第二次不能留下额外残留。
		t.Fatalf("unexpected dir entries after oversize save: %d", len(entries))
	}
}
