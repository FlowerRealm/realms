package scheduler

import (
	"context"
	"sync"
	"testing"

	"realms/internal/store"
)

type fakeGroupPointerStore struct {
	mu     sync.Mutex
	recs   map[int64]store.ChannelGroupPointer
	calls  int
	gets   int
	errGet error
}

func (f *fakeGroupPointerStore) GetChannelGroupPointer(_ context.Context, groupID int64) (store.ChannelGroupPointer, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.errGet != nil {
		return store.ChannelGroupPointer{}, false, f.errGet
	}
	if f.recs == nil {
		return store.ChannelGroupPointer{}, false, nil
	}
	rec, ok := f.recs[groupID]
	return rec, ok, nil
}

func (f *fakeGroupPointerStore) UpsertChannelGroupPointer(_ context.Context, in store.ChannelGroupPointer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recs == nil {
		f.recs = make(map[int64]store.ChannelGroupPointer)
	}
	f.recs[in.GroupID] = in
	f.calls++
	return nil
}

func (f *fakeGroupPointerStore) snapshot(groupID int64) (calls int, ok bool, rec store.ChannelGroupPointer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recs == nil {
		return f.calls, false, store.ChannelGroupPointer{}
	}
	rec, ok = f.recs[groupID]
	return f.calls, ok, rec
}

func TestGroupPointerStore_PersistOnlyOnPointerChange(t *testing.T) {
	s := New(&fakeStore{})
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	s.setChannelGroupPointer(1, 10, false, "route")
	calls, ok, rec := ps.snapshot(1)
	if !ok {
		t.Fatalf("expected record to be written")
	}
	if calls != 1 {
		t.Fatalf("expected calls=1, got %d", calls)
	}
	if rec.GroupID != 1 || rec.ChannelID != 10 || rec.Pinned {
		t.Fatalf("unexpected rec=%+v", rec)
	}

	// Same channel + same pinned: no persist.
	s.setChannelGroupPointer(1, 10, false, "route")
	calls, _, _ = ps.snapshot(1)
	if calls != 1 {
		t.Fatalf("expected calls to stay 1, got %d", calls)
	}

	// Change channel: persist.
	s.setChannelGroupPointer(1, 11, false, "route")
	calls, _, rec = ps.snapshot(1)
	if calls != 2 {
		t.Fatalf("expected calls=2, got %d", calls)
	}
	if rec.ChannelID != 11 {
		t.Fatalf("expected channel_id=11, got %d", rec.ChannelID)
	}
}

func TestGroupPointerStore_PersistWhenPinnedFlagChangesEvenIfIDSame(t *testing.T) {
	s := New(&fakeStore{})
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	s.setChannelGroupPointer(1, 10, false, "route")
	calls, _, rec := ps.snapshot(1)
	if calls != 1 {
		t.Fatalf("expected calls=1, got %d", calls)
	}
	if rec.Pinned {
		t.Fatalf("expected pinned=false, got true")
	}

	// Same channel, pinned changes: should persist.
	s.setChannelGroupPointer(1, 10, true, "manual")
	calls, _, rec = ps.snapshot(1)
	if calls != 2 {
		t.Fatalf("expected calls=2, got %d", calls)
	}
	if rec.ChannelID != 10 || !rec.Pinned {
		t.Fatalf("expected channel_id=10 pinned=true, got %+v", rec)
	}
}

func TestTouchChannelGroupPointer_PreservesPinned(t *testing.T) {
	s := New(&fakeStore{})
	ps := &fakeGroupPointerStore{}
	s.SetGroupPointerStore(ps)

	s.setChannelGroupPointer(1, 10, true, "manual")
	_, _, rec := ps.snapshot(1)
	if rec.ChannelID != 10 || !rec.Pinned {
		t.Fatalf("expected initial pinned record, got %+v", rec)
	}

	s.touchChannelGroupPointer(context.Background(), 1, 11, "route")
	_, _, rec = ps.snapshot(1)
	if rec.ChannelID != 11 {
		t.Fatalf("expected channel_id=11, got %d", rec.ChannelID)
	}
	if !rec.Pinned {
		t.Fatalf("expected pinned to be preserved, got false")
	}
}

