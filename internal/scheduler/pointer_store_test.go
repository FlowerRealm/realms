package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"realms/internal/store"
)

type fakePointerStore struct {
	mu    sync.Mutex
	value string
	calls int
}

func (f *fakePointerStore) GetAppSetting(_ context.Context, key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key != store.SettingSchedulerChannelPointer {
		return "", false, nil
	}
	if f.value == "" {
		return "", false, nil
	}
	return f.value, true, nil
}

func (f *fakePointerStore) UpsertAppSetting(_ context.Context, key string, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key != store.SettingSchedulerChannelPointer {
		return nil
	}
	f.value = value
	f.calls++
	return nil
}

func (f *fakePointerStore) snapshot() (calls int, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.value
}

func TestPointerStore_PersistOnlyOnPointerChange(t *testing.T) {
	s := New(&fakeStore{})
	ps := &fakePointerStore{}
	s.SetPointerStore(ps)

	s.TouchChannelPointer(1, "route")
	calls, raw := ps.snapshot()
	if calls != 1 {
		t.Fatalf("expected calls=1, got %d", calls)
	}
	st, ok, err := store.ParseSchedulerChannelPointerState(raw)
	if err != nil || !ok {
		t.Fatalf("ParseSchedulerChannelPointerState ok=%v err=%v raw=%q", ok, err, raw)
	}
	if st.ChannelID != 1 {
		t.Fatalf("expected channel_id=1, got %d", st.ChannelID)
	}
	if st.Pinned {
		t.Fatalf("expected pinned=false on touch")
	}

	// Same channel: no persist.
	s.TouchChannelPointer(1, "route")
	calls, _ = ps.snapshot()
	if calls != 1 {
		t.Fatalf("expected calls to stay 1, got %d", calls)
	}

	// Change channel: persist.
	s.TouchChannelPointer(2, "route")
	calls, raw = ps.snapshot()
	if calls != 2 {
		t.Fatalf("expected calls=2, got %d", calls)
	}
	st, ok, err = store.ParseSchedulerChannelPointerState(raw)
	if err != nil || !ok {
		t.Fatalf("ParseSchedulerChannelPointerState ok=%v err=%v raw=%q", ok, err, raw)
	}
	if st.ChannelID != 2 {
		t.Fatalf("expected channel_id=2, got %d", st.ChannelID)
	}
}

func TestPointerStore_PinPersistsPinnedFlagEvenIfIDSame(t *testing.T) {
	s := New(&fakeStore{})
	ps := &fakePointerStore{}
	s.SetPointerStore(ps)

	s.TouchChannelPointer(3, "route")
	calls, _ := ps.snapshot()
	if calls != 1 {
		t.Fatalf("expected calls=1, got %d", calls)
	}

	// Pin same ID should persist pinned=true (id unchanged but pinned flag changes).
	s.PinChannel(3)
	calls, raw := ps.snapshot()
	if calls != 2 {
		t.Fatalf("expected calls=2, got %d", calls)
	}
	st, ok, err := store.ParseSchedulerChannelPointerState(raw)
	if err != nil || !ok {
		t.Fatalf("ParseSchedulerChannelPointerState ok=%v err=%v raw=%q", ok, err, raw)
	}
	if st.ChannelID != 3 || !st.Pinned {
		t.Fatalf("expected channel_id=3 pinned=true, got channel_id=%d pinned=%v", st.ChannelID, st.Pinned)
	}
}

func TestPointerStore_BanRotationPersistsNextPointer(t *testing.T) {
	s := New(&fakeStore{})
	ps := &fakePointerStore{}
	s.SetPointerStore(ps)

	s.state.SetChannelPointerRing([]int64{1, 2})
	s.PinChannel(1)

	now := time.Now()
	s.state.BanChannelImmediate(1, now, 10*time.Second)

	id, _, reason, ok := s.PinnedChannelInfo()
	if !ok {
		t.Fatalf("expected pointer to be active")
	}
	if id != 2 {
		t.Fatalf("expected pointer to rotate to 2, got %d", id)
	}
	if reason != "ban" {
		t.Fatalf("expected reason=ban, got %q", reason)
	}

	_, raw := ps.snapshot()
	st, ok2, err := store.ParseSchedulerChannelPointerState(raw)
	if err != nil || !ok2 {
		t.Fatalf("ParseSchedulerChannelPointerState ok=%v err=%v raw=%q", ok2, err, raw)
	}
	if st.ChannelID != 2 {
		t.Fatalf("expected persisted channel_id=2, got %d", st.ChannelID)
	}
}
