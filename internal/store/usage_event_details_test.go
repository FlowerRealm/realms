package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestSQLiteUsageEventDetails_FinalizeStoresBodies(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "alice@example.com", "alice", []byte("pw-hash"), store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tokenID, _, err := st.CreateUserToken(ctx, userID, nil, "tok_test_123")
	if err != nil {
		t.Fatalf("CreateUserToken: %v", err)
	}

	usageID, err := st.ReserveUsage(ctx, store.ReserveUsageInput{
		RequestID: "req_1",
		UserID:    userID,
		TokenID:   tokenID,
		Model: func() *string {
			s := "m1"
			return &s
		}(),
		ReservedUSD:      decimal.Zero,
		ReserveExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveUsage: %v", err)
	}

	upReq := `{"model":"m1","input":"hi","max_output_tokens":123}`
	upResp := `{"detail":"Unsupported parameter: max_tokens"}`
	if err := st.FinalizeUsageEvent(ctx, store.FinalizeUsageEventInput{
		UsageEventID:         usageID,
		Endpoint:             "/v1/responses",
		Method:               "POST",
		StatusCode:           400,
		LatencyMS:            10,
		IsStream:             true,
		RequestBytes:         123,
		ResponseBytes:        46,
		UpstreamRequestBody:  &upReq,
		UpstreamResponseBody: &upResp,
	}); err != nil {
		t.Fatalf("FinalizeUsageEvent: %v", err)
	}

	detail, err := st.GetUsageEventDetail(ctx, usageID)
	if err != nil {
		t.Fatalf("GetUsageEventDetail: %v", err)
	}
	if detail.UsageEventID != usageID {
		t.Fatalf("usage_event_id mismatch: got %d want %d", detail.UsageEventID, usageID)
	}
	if detail.UpstreamRequestBody == nil || *detail.UpstreamRequestBody != upReq {
		t.Fatalf("upstream_request_body mismatch: got=%v want=%q", detail.UpstreamRequestBody, upReq)
	}
	if detail.UpstreamResponseBody == nil || *detail.UpstreamResponseBody != upResp {
		t.Fatalf("upstream_response_body mismatch: got=%v want=%q", detail.UpstreamResponseBody, upResp)
	}
	if detail.CreatedAt.IsZero() || detail.UpdatedAt.IsZero() {
		t.Fatalf("expected created_at/updated_at to be set, got created_at=%v updated_at=%v", detail.CreatedAt, detail.UpdatedAt)
	}
}
