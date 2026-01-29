package proxylog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Enable bool
	Dir    string
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
	b, err := json.Marshal(entry)
	if err != nil {
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
}

func (w *Writer) buildFilename(entry Entry) string {
	ts := entry.Time.UTC().Format("20060102_150405")
	req := sanitizeFilenameComponent(entry.RequestID, 64)
	if req == "" {
		req = randHex(8)
	}
	return fmt.Sprintf("%s_%s.json", ts, req)
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
