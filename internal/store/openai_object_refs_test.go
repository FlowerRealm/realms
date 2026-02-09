package store_test

import (
	"context"
	"testing"

	"realms/internal/store"
)

func TestOpenAIObjectRefs_SQLiteRoundTrip(t *testing.T) {
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
	ref := store.OpenAIObjectRef{
		ObjectType:    "chat_completion",
		ObjectID:      "chatcmpl-test",
		UserID:        9,
		TokenID:       123,
		SelectionJSON: `{"channel_id":1,"credential_type":"openai_compatible","credential_id":7,"base_url":"https://a.example"}`,
	}
	if err := st.UpsertOpenAIObjectRef(ctx, ref); err != nil {
		t.Fatalf("UpsertOpenAIObjectRef: %v", err)
	}

	got, ok, err := st.GetOpenAIObjectRef(ctx, ref.ObjectType, ref.ObjectID)
	if err != nil {
		t.Fatalf("GetOpenAIObjectRef: %v", err)
	}
	if !ok {
		t.Fatalf("expected object ref to exist")
	}
	if got.UserID != ref.UserID || got.TokenID != ref.TokenID || got.SelectionJSON != ref.SelectionJSON {
		t.Fatalf("unexpected object ref: %+v", got)
	}

	if _, ok, err := st.GetOpenAIObjectRefForUser(ctx, 10, ref.ObjectType, ref.ObjectID); err != nil {
		t.Fatalf("GetOpenAIObjectRefForUser mismatch: %v", err)
	} else if ok {
		t.Fatalf("expected mismatch user to be treated as missing")
	}
	if got2, ok, err := st.GetOpenAIObjectRefForUser(ctx, 9, ref.ObjectType, ref.ObjectID); err != nil {
		t.Fatalf("GetOpenAIObjectRefForUser: %v", err)
	} else if !ok {
		t.Fatalf("expected object ref to exist for owner")
	} else if got2.ObjectID != ref.ObjectID {
		t.Fatalf("unexpected object id: %s", got2.ObjectID)
	}

	ls, err := st.ListOpenAIObjectRefsByUser(ctx, 9, ref.ObjectType, 10)
	if err != nil {
		t.Fatalf("ListOpenAIObjectRefsByUser: %v", err)
	}
	if len(ls) != 1 || ls[0].ObjectID != ref.ObjectID {
		t.Fatalf("unexpected list: %+v", ls)
	}

	if err := st.DeleteOpenAIObjectRef(ctx, ref.ObjectType, ref.ObjectID); err != nil {
		t.Fatalf("DeleteOpenAIObjectRef: %v", err)
	}
	if _, ok, err := st.GetOpenAIObjectRef(ctx, ref.ObjectType, ref.ObjectID); err != nil {
		t.Fatalf("GetOpenAIObjectRef after delete: %v", err)
	} else if ok {
		t.Fatalf("expected object ref to be deleted")
	}
}
