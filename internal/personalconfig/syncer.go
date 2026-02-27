package personalconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"realms/internal/store"
)

type Syncer struct {
	enabled bool
	path    string

	db      *sql.DB
	dialect store.Dialect
	st      *store.Store

	mu             sync.Mutex
	currentSHA256  string
	lastError      string
	lastWrittenSHA string

	stopCh  chan struct{}
	watchOn bool
	lastMod time.Time
}

type Options struct {
	Enabled bool
	Path    string
	DB      *sql.DB
	Dialect store.Dialect
	Store   *store.Store
}

func New(opts Options) (*Syncer, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("personal config path is empty")
	}
	s := &Syncer{
		enabled: opts.Enabled,
		path:    opts.Path,
		db:      opts.DB,
		dialect: opts.Dialect,
		st:      opts.Store,
		stopCh:  make(chan struct{}),
	}
	return s, nil
}

func (s *Syncer) Enabled() bool {
	return s != nil && s.enabled
}

func (s *Syncer) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Syncer) CurrentSHA256() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentSHA256
}

func (s *Syncer) LastError() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastError
}

func (s *Syncer) Init(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	return s.withLock(ctx, func() error {
		return s.ensureAppliedLocked(ctx)
	})
}

// BeginMutation serializes config mutations and guarantees that file<->runtime stays consistent.
// On any handler error, call Abort(). On success, call Commit() before responding.
func (s *Syncer) BeginMutation(ctx context.Context) (*Mutation, error) {
	if !s.Enabled() {
		return &Mutation{syncer: s, finalized: true}, nil
	}
	s.mu.Lock()

	if err := s.ensureAppliedLocked(ctx); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	pre, preSHA, err := s.loadBundleLocked()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	_ = preSHA
	return &Mutation{
		syncer:   s,
		pre:      pre,
		preSHA:   preSHA,
		finalized: false,
	}, nil
}

type Mutation struct {
	syncer *Syncer
	pre    Bundle
	preSHA string

	finalized bool
}

func (m *Mutation) Abort(ctx context.Context) error {
	if m == nil || m.syncer == nil || m.finalized {
		return nil
	}
	defer func() {
		m.finalized = true
		m.syncer.mu.Unlock()
	}()
	// Restore runtime to pre-bundle (authoritative snapshot at mutation start).
	if err := RebuildRuntimeFromBundle(ctx, m.syncer.db, m.syncer.dialect, m.pre); err != nil {
		m.syncer.lastError = err.Error()
		return err
	}
	m.syncer.currentSHA256 = m.preSHA
	m.syncer.lastError = ""
	return nil
}

func (m *Mutation) Commit(ctx context.Context, includeSecrets bool) error {
	if m == nil || m.syncer == nil || m.finalized {
		return nil
	}
	defer func() {
		m.finalized = true
		m.syncer.mu.Unlock()
	}()

	b, sha, err := m.syncer.exportBundleLocked(ctx, includeSecrets)
	if err != nil {
		m.syncer.lastError = err.Error()
		_ = RebuildRuntimeFromBundle(ctx, m.syncer.db, m.syncer.dialect, m.pre)
		m.syncer.currentSHA256 = m.preSHA
		return err
	}
	data, sha2, err := encodeBundle(b)
	if err != nil {
		m.syncer.lastError = err.Error()
		_ = RebuildRuntimeFromBundle(ctx, m.syncer.db, m.syncer.dialect, m.pre)
		m.syncer.currentSHA256 = m.preSHA
		return err
	}
	if sha2 != sha {
		sha = sha2
	}

	if err := writeFileAtomic(m.syncer.path, data, 0o600); err != nil {
		m.syncer.lastError = err.Error()
		_ = RebuildRuntimeFromBundle(ctx, m.syncer.db, m.syncer.dialect, m.pre)
		m.syncer.currentSHA256 = m.preSHA
		return fmt.Errorf("write config file: %w", err)
	}

	m.syncer.lastWrittenSHA = sha
	m.syncer.currentSHA256 = sha
	m.syncer.lastError = ""
	return nil
}

// ReplaceFileAndApply replaces the file content and rebuilds runtime from it.
func (s *Syncer) ReplaceFileAndApply(ctx context.Context, bundleJSON []byte) (string, error) {
	if !s.Enabled() {
		return "", errors.New("personal config not enabled")
	}
	var sha string
	err := s.withLock(ctx, func() error {
		b, parsedSHA, err := decodeBundle(bundleJSON)
		if err != nil {
			s.lastError = err.Error()
			return err
		}
		sha = parsedSHA
		// Write file first (authoritative), then apply to runtime.
		if err := writeFileAtomic(s.path, bundleJSON, 0o600); err != nil {
			s.lastError = err.Error()
			return err
		}
		if err := RebuildRuntimeFromBundle(ctx, s.db, s.dialect, b); err != nil {
			s.lastError = err.Error()
			return err
		}
		s.lastWrittenSHA = sha
		s.currentSHA256 = sha
		s.lastError = ""
		return nil
	})
	return sha, err
}

func (s *Syncer) ReadRaw(ctx context.Context) ([]byte, string, error) {
	if !s.Enabled() {
		return nil, "", errors.New("personal config not enabled")
	}
	var out []byte
	var sha string
	err := s.withLock(ctx, func() error {
		if err := s.ensureAppliedLocked(ctx); err != nil {
			return err
		}
		data, ok, err := readFileIfExists(s.path)
		if err != nil {
			s.lastError = err.Error()
			return err
		}
		if !ok {
			return fmt.Errorf("config file not found: %s", s.path)
		}
		_, sha, err = decodeBundle(data)
		if err != nil {
			s.lastError = err.Error()
			return err
		}
		out = data
		return nil
	})
	return out, sha, err
}

func (s *Syncer) StartWatcher() error {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchOn {
		return nil
	}
	s.watchOn = true
	go s.pollLoop()
	return nil
}

func (s *Syncer) StopWatcher() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

func (s *Syncer) pollLoop() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			// Cheap change detector via mtime; sha check happens in ApplyExternalChange.
			info, err := os.Stat(s.path)
			if err != nil {
				continue
			}
			mod := info.ModTime()
			s.mu.Lock()
			changed := s.lastMod.IsZero() || mod.After(s.lastMod)
			if changed {
				s.lastMod = mod
			}
			s.mu.Unlock()
			if !changed {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = s.ApplyExternalChange(ctx)
			cancel()
		}
	}
}

func (s *Syncer) ApplyExternalChange(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	return s.withLock(ctx, func() error {
		b, sha, err := s.loadBundleLocked()
		if err != nil {
			s.lastError = err.Error()
			return err
		}
		if sha == "" {
			return nil
		}
		// Ignore self-write events.
		if sha == s.lastWrittenSHA {
			s.currentSHA256 = sha
			s.lastError = ""
			return nil
		}
		if sha == s.currentSHA256 {
			return nil
		}
		if err := RebuildRuntimeFromBundle(ctx, s.db, s.dialect, b); err != nil {
			s.lastError = err.Error()
			return err
		}
		s.currentSHA256 = sha
		s.lastError = ""
		return nil
	})
}

func (s *Syncer) withLock(ctx context.Context, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = ctx
	return fn()
}

func (s *Syncer) ensureAppliedLocked(ctx context.Context) error {
	data, ok, err := readFileIfExists(s.path)
	if err != nil {
		s.lastError = err.Error()
		return err
	}
	if !ok {
		// No file: export current runtime and create baseline.
		b, sha, err := s.exportBundleLocked(ctx, true)
		if err != nil {
			s.lastError = err.Error()
			return err
		}
		out, sha2, err := encodeBundle(b)
		if err != nil {
			s.lastError = err.Error()
			return err
		}
		if sha2 != sha {
			sha = sha2
		}
		if err := writeFileAtomic(s.path, out, 0o600); err != nil {
			s.lastError = err.Error()
			return err
		}
		s.lastWrittenSHA = sha
		s.currentSHA256 = sha
		s.lastError = ""
		return nil
	}

	b, sha, err := decodeBundle(data)
	if err != nil {
		s.lastError = err.Error()
		return err
	}
	if sha == s.currentSHA256 {
		return nil
	}
	if err := RebuildRuntimeFromBundle(ctx, s.db, s.dialect, b); err != nil {
		s.lastError = err.Error()
		return err
	}
	s.currentSHA256 = sha
	s.lastError = ""
	return nil
}

func (s *Syncer) loadBundleLocked() (Bundle, string, error) {
	data, ok, err := readFileIfExists(s.path)
	if err != nil {
		return Bundle{}, "", err
	}
	if !ok {
		return Bundle{}, "", fmt.Errorf("config file not found: %s", s.path)
	}
	return decodeBundle(data)
}

func (s *Syncer) exportBundleLocked(ctx context.Context, includeSecrets bool) (Bundle, string, error) {
	if s.st == nil {
		return Bundle{}, "", errors.New("store is nil")
	}
	admin, err := s.st.ExportAdminConfig(ctx)
	if err != nil {
		return Bundle{}, "", err
	}
	b := Bundle{
		Version:    BundleVersion,
		ExportedAt: time.Now(),
		Admin:      admin,
	}
	if includeSecrets {
		sec, err := exportSecrets(ctx, s.st)
		if err != nil {
			return Bundle{}, "", err
		}
		b.Secrets = &sec
	}
	_, sha, err := encodeBundle(b)
	if err != nil {
		return Bundle{}, "", err
	}
	return b, sha, nil
}

func exportSecrets(ctx context.Context, st *store.Store) (Secrets, error) {
	channels, err := st.ListUpstreamChannels(ctx)
	if err != nil {
		return Secrets{}, err
	}
	openaiOut := make([]EndpointSecrets, 0)
	anthropicOut := make([]EndpointSecrets, 0)

	for _, ch := range channels {
		ep, err := st.GetUpstreamEndpointByChannelID(ctx, ch.ID)
		if err != nil || ep.ID <= 0 {
			continue
		}
		switch ch.Type {
		case store.UpstreamTypeOpenAICompatible:
			cs, err := st.ListOpenAICompatibleCredentialsByEndpoint(ctx, ep.ID)
			if err != nil {
				return Secrets{}, err
			}
			var creds []CredentialSecret
			for _, c := range cs {
				if c.Status != 1 {
					continue
				}
				sec, err := st.GetOpenAICompatibleCredentialSecret(ctx, c.ID)
				if err != nil {
					return Secrets{}, err
				}
				name := sec.Name
				key := strings.TrimSpace(sec.APIKey)
				if key == "" {
					continue
				}
				creds = append(creds, CredentialSecret{Name: name, APIKey: key})
			}
			openaiOut = append(openaiOut, EndpointSecrets{
				ChannelType: ch.Type,
				ChannelName: ch.Name,
				BaseURL:     ep.BaseURL,
				Credentials: creds,
			})
		case store.UpstreamTypeAnthropic:
			cs, err := st.ListAnthropicCredentialsByEndpoint(ctx, ep.ID)
			if err != nil {
				return Secrets{}, err
			}
			var creds []CredentialSecret
			for _, c := range cs {
				if c.Status != 1 {
					continue
				}
				sec, err := st.GetAnthropicCredentialSecret(ctx, c.ID)
				if err != nil {
					return Secrets{}, err
				}
				name := sec.Name
				key := strings.TrimSpace(sec.APIKey)
				if key == "" {
					continue
				}
				creds = append(creds, CredentialSecret{Name: name, APIKey: key})
			}
			anthropicOut = append(anthropicOut, EndpointSecrets{
				ChannelType: ch.Type,
				ChannelName: ch.Name,
				BaseURL:     ep.BaseURL,
				Credentials: creds,
			})
		}
	}
	return Secrets{OpenAICompatible: openaiOut, Anthropic: anthropicOut}, nil
}
