package store_test

import (
	"context"
	"testing"
	"time"

	"realms/internal/store"
)

func TestChannelGroupPointers_SQLiteRoundTrip(t *testing.T) {
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	ctx := context.Background()
	now := time.Now()
	ms := now.UnixMilli()

	if err := st.UpsertChannelGroupPointer(ctx, store.ChannelGroupPointer{
		GroupID:       7,
		ChannelID:     11,
		Pinned:        true,
		MovedAtUnixMS: ms,
		Reason:        "manual",
	}); err != nil {
		t.Fatalf("UpsertChannelGroupPointer: %v", err)
	}

	got, ok, err := st.GetChannelGroupPointer(ctx, 7)
	if err != nil {
		t.Fatalf("GetChannelGroupPointer: %v", err)
	}
	if !ok {
		t.Fatalf("expected pointer to exist")
	}
	if got.GroupID != 7 || got.ChannelID != 11 || !got.Pinned || got.Reason != "manual" {
		t.Fatalf("unexpected pointer: %+v", got)
	}
	if got.MovedAtUnixMS != ms {
		t.Fatalf("moved_at_unix_ms mismatch: got=%d want=%d", got.MovedAtUnixMS, ms)
	}
	if got.MovedAt().IsZero() {
		t.Fatalf("expected moved_at to be parsed")
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("expected created_at/updated_at to be parsed")
	}

	// Update existing.
	if err := st.UpsertChannelGroupPointer(ctx, store.ChannelGroupPointer{
		GroupID:       7,
		ChannelID:     12,
		Pinned:        false,
		MovedAtUnixMS: ms + 1,
		Reason:        "route",
	}); err != nil {
		t.Fatalf("UpsertChannelGroupPointer (update): %v", err)
	}

	got, ok, err = st.GetChannelGroupPointer(ctx, 7)
	if err != nil {
		t.Fatalf("GetChannelGroupPointer (2): %v", err)
	}
	if !ok {
		t.Fatalf("expected pointer to exist")
	}
	if got.ChannelID != 12 || got.Pinned || got.Reason != "route" {
		t.Fatalf("unexpected pointer after update: %+v", got)
	}
}

