package openai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"realms/internal/auth"
	"realms/internal/middleware"
	"realms/internal/scheduler"
	"realms/internal/store"
	"realms/internal/upstream"
)

func TestResponsesCompact_Remote_ProxiesHeadersAndCommitsQuota(t *testing.T) {
	var gotAuth string
	var gotAccept string
	var gotSessionID string
	var gotOriginator string
	var gotUA string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotSessionID = r.Header.Get("session_id")
		gotOriginator = r.Header.Get("originator")
		gotUA = r.Header.Get("user-agent")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20,"input_tokens_details":{"cached_tokens":3}}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	usage := &recordingUsage{}
	sub2api := upstream.NewSub2APIClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, false, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, sub2api)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-should-not-leak")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("session_id", "s1h")
	req.Header.Set("originator", "cli")
	req.Header.Set("User-Agent", "ua-test")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	if gotAuth != "Bearer gwk_test" {
		t.Fatalf("expected gateway auth, got=%q", gotAuth)
	}
	if strings.TrimSpace(gotAccept) != "" {
		t.Fatalf("expected accept to be removed, got=%q", gotAccept)
	}
	if gotSessionID != "s1h" {
		t.Fatalf("expected session_id from header to be forwarded, got=%q", gotSessionID)
	}
	if gotOriginator != "cli" {
		t.Fatalf("expected originator forwarded, got=%q", gotOriginator)
	}
	if gotUA != "ua-test" {
		t.Fatalf("expected user-agent forwarded, got=%q", gotUA)
	}

	if len(q.reserveCalls) != 1 {
		t.Fatalf("expected reserve called once, got=%d", len(q.reserveCalls))
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected commit called once, got=%d", len(q.commitCalls))
	}
	if len(q.voidCalls) != 0 {
		t.Fatalf("expected void not called, got=%d", len(q.voidCalls))
	}
	if q.commitCalls[0].InputTokens == nil || *q.commitCalls[0].InputTokens != 10 {
		t.Fatalf("expected input tokens=10, got=%v", q.commitCalls[0].InputTokens)
	}
	if q.commitCalls[0].CachedInputTokens == nil || *q.commitCalls[0].CachedInputTokens != 3 {
		t.Fatalf("expected cached input tokens=3, got=%v", q.commitCalls[0].CachedInputTokens)
	}
	if q.commitCalls[0].OutputTokens == nil || *q.commitCalls[0].OutputTokens != 20 {
		t.Fatalf("expected output tokens=20, got=%v", q.commitCalls[0].OutputTokens)
	}
	if len(usage.calls) == 0 {
		t.Fatalf("expected usage finalized")
	}
}

func TestResponsesCompact_Remote_Upstream4xxVoidsQuota(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"bad"}}`)
	}))
	defer ts.Close()

	fs := &fakeStore{}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	sub2api := upstream.NewSub2APIClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, false, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, sub2api)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.reserveCalls) != 1 {
		t.Fatalf("expected reserve called, got=%d", len(q.reserveCalls))
	}
	if len(q.commitCalls) != 0 {
		t.Fatalf("expected commit not called, got=%d", len(q.commitCalls))
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected void called once, got=%d", len(q.voidCalls))
	}
}
