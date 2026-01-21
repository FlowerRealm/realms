package proxylog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Enable   bool
	Dir      string
	MaxBytes int64
	MaxFiles int
}

type Entry struct {
	Time      time.Time `json:"time"`
	RequestID string    `json:"request_id"`

	Path   string `json:"path"`
	Method string `json:"method"`

	UserID  int64 `json:"user_id"`
	TokenID int64 `json:"token_id"`

	Model  *string `json:"model,omitempty"`
	Stream bool    `json:"stream"`

	ChannelID       int64  `json:"channel_id"`
	ChannelType     string `json:"channel_type"`
	CredentialType  string `json:"credential_type"`
	CredentialID    int64  `json:"credential_id"`
	UpstreamBaseURL string `json:"upstream_base_url"`

	StatusCode int    `json:"status_code"`
	ErrorClass string `json:"error_class"`
	ErrorMsg   string `json:"error_message,omitempty"`

	LatencyMS int `json:"latency_ms"`
}

type Writer struct {
	cfg Config
	mu  sync.Mutex
}

func New(cfg Config) *Writer {
	cfg.Dir = strings.TrimSpace(cfg.Dir)
	if cfg.Dir == "" {
		cfg.Dir = "./out/proxy"
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 128 << 10
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = 200
	}
	return &Writer{cfg: cfg}
}

func (w *Writer) Enabled() bool {
	return w != nil && w.cfg.Enable
}

func (w *Writer) WriteFailure(ctx context.Context, entry Entry) {
	if w == nil || !w.cfg.Enable {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	b, ok := w.marshalWithinLimit(entry)
	if !ok {
		return
	}

	dir := w.cfg.Dir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	name := w.buildFilename(entry)
	tmp := filepath.Join(dir, name+".tmp")
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, dst)
	w.cleanupOldFiles(dir)
}

func (w *Writer) buildFilename(entry Entry) string {
	ts := entry.Time.UTC().Format("20060102_150405")
	req := sanitizeFilenameComponent(entry.RequestID, 64)
	if req == "" {
		req = randHex(8)
	}
	return fmt.Sprintf("%s_%s.json", ts, req)
}

func (w *Writer) marshalWithinLimit(entry Entry) ([]byte, bool) {
	max := w.cfg.MaxBytes
	if max <= 0 {
		max = 128 << 10
	}
	trim := func(s string, n int) string {
		s = strings.TrimSpace(s)
		if n <= 0 || s == "" {
			return s
		}
		if len(s) <= n {
			return s
		}
		return s[:n] + "…"
	}

	b, err := json.Marshal(entry)
	if err == nil && int64(len(b)) <= max {
		return b, true
	}

	// 降级：逐步截断长字段，优先保留结构字段。
	entry.ErrorMsg = trim(entry.ErrorMsg, 400)
	entry.UpstreamBaseURL = trim(entry.UpstreamBaseURL, 200)
	entry.Path = trim(entry.Path, 120)
	entry.Method = trim(entry.Method, 16)
	entry.ChannelType = trim(entry.ChannelType, 32)
	entry.CredentialType = trim(entry.CredentialType, 32)
	entry.ErrorClass = trim(entry.ErrorClass, 64)
	if entry.Model != nil {
		m := trim(*entry.Model, 128)
		entry.Model = &m
	}
	b, err = json.Marshal(entry)
	if err == nil && int64(len(b)) <= max {
		return b, true
	}

	// 最后降级：去掉错误信息/URL，确保写盘不爆。
	entry.ErrorMsg = ""
	entry.UpstreamBaseURL = ""
	b, err = json.Marshal(entry)
	if err == nil && int64(len(b)) <= max {
		return b, true
	}
	return nil, false
}

func sanitizeFilenameComponent(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '_'
		}
	}, s)
	s = strings.Trim(s, "._-")
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

func randHex(n int) string {
	if n <= 0 {
		n = 8
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "rand"
	}
	return hex.EncodeToString(b)
}

func (w *Writer) cleanupOldFiles(dir string) {
	max := w.cfg.MaxFiles
	if max <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		name    string
		modTime time.Time
	}
	var files []f
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, f{name: name, modTime: info.ModTime()})
	}
	if len(files) <= max {
		return
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, ff := range files[:len(files)-max] {
		_ = os.Remove(filepath.Join(dir, ff.name))
	}
}

