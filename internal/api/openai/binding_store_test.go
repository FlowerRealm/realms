package openai

import (
	"context"
	"strconv"
	"sync"
	"time"

	"realms/internal/scheduler"
)

var _ scheduler.BindingStore = (*recordingBindingStore)(nil)

type recordingBindingStore struct {
	mu       sync.Mutex
	payloads map[string]string
}

func newRecordingBindingStore() *recordingBindingStore {
	return &recordingBindingStore{
		payloads: make(map[string]string),
	}
}

func (s *recordingBindingStore) key(userID int64, routeKeyHash string) string {
	return strconv.FormatInt(userID, 10) + ":" + routeKeyHash
}

func (s *recordingBindingStore) Has(userID int64, routeKeyHash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.payloads[s.key(userID, routeKeyHash)]
	return ok
}

func (s *recordingBindingStore) GetSessionBindingPayload(_ context.Context, userID int64, routeKeyHash string, _ time.Time) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.payloads[s.key(userID, routeKeyHash)]
	return v, ok, nil
}

func (s *recordingBindingStore) UpsertSessionBindingPayload(_ context.Context, userID int64, routeKeyHash string, payload string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads[s.key(userID, routeKeyHash)] = payload
	return nil
}

func (s *recordingBindingStore) DeleteSessionBinding(_ context.Context, userID int64, routeKeyHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.payloads, s.key(userID, routeKeyHash))
	return nil
}
