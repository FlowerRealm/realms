package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"realms/internal/store"
)

func TestUpdateMainGroupWithRename_CascadesReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

	db, err := store.OpenSQLite(path)
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

	const subgroup = "pw-subgroup"
	if _, err := st.CreateChannelGroup(ctx, subgroup, nil, 1, store.DefaultGroupPriceMultiplier, 5); err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}

	const oldName = "pw-old"
	if err := st.CreateMainGroup(ctx, oldName, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup: %v", err)
	}
	if err := st.ReplaceMainGroupSubgroups(ctx, oldName, []string{subgroup}); err != nil {
		t.Fatalf("ReplaceMainGroupSubgroups: %v", err)
	}

	userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserMainGroup(ctx, userID, oldName); err != nil {
		t.Fatalf("SetUserMainGroup: %v", err)
	}

	newName := "pw-new"
	gotName, err := st.UpdateMainGroupWithRename(ctx, oldName, &newName, nil, 1)
	if err != nil {
		t.Fatalf("UpdateMainGroupWithRename: %v", err)
	}
	if gotName != newName {
		t.Fatalf("rename mismatch: got %q want %q", gotName, newName)
	}

	u, err := st.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if u.MainGroup != newName {
		t.Fatalf("user main_group not updated: got %q want %q", u.MainGroup, newName)
	}

	sgNew, err := st.ListMainGroupSubgroups(ctx, newName)
	if err != nil {
		t.Fatalf("ListMainGroupSubgroups(new): %v", err)
	}
	if len(sgNew) != 1 || sgNew[0].Subgroup != subgroup {
		t.Fatalf("subgroups not updated: got %+v", sgNew)
	}
	sgOld, err := st.ListMainGroupSubgroups(ctx, oldName)
	if err != nil {
		t.Fatalf("ListMainGroupSubgroups(old): %v", err)
	}
	if len(sgOld) != 0 {
		t.Fatalf("expected old subgroups to be empty, got %+v", sgOld)
	}

	if _, err := st.GetMainGroupByName(ctx, oldName); err == nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected old main_group to be deleted, got err=%v", err)
	}
	if _, err := st.GetMainGroupByName(ctx, newName); err != nil {
		t.Fatalf("GetMainGroupByName(new): %v", err)
	}
}

func TestUpdateMainGroupWithRename_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

	db, err := store.OpenSQLite(path)
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

	const a = "pw-a"
	const b = "pw-b"
	if err := st.CreateMainGroup(ctx, a, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup(a): %v", err)
	}
	if err := st.CreateMainGroup(ctx, b, nil, 1); err != nil {
		t.Fatalf("CreateMainGroup(b): %v", err)
	}

	got, err := st.UpdateMainGroupWithRename(ctx, a, strPtr(b), nil, 1)
	if err == nil {
		t.Fatalf("expected error, got name=%q", got)
	}
	if !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func strPtr(v string) *string { return &v }
