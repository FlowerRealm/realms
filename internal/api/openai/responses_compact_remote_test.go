package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shopspring/decimal"
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
	var gotTargetGroup string
	var gotRouteKeyHash string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotSessionID = r.Header.Get("session_id")
		gotOriginator = r.Header.Get("originator")
		gotUA = r.Header.Get("user-agent")
		gotTargetGroup = r.Header.Get(upstream.CompactGatewayTargetGroupHeader)
		gotRouteKeyHash = r.Header.Get(upstream.CompactGatewayRouteKeyHashHeader)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20,"input_tokens_details":{"cached_tokens":3}}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{
		models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: store.DefaultGroupName, Status: 1, PriorityPricingEnabled: true, PriorityInputUSDPer1M: decimalPtrForResponsesCompactTest(t, "1"), PriorityOutputUSDPer1M: decimalPtrForResponsesCompactTest(t, "2")}},
		groupByName: map[string]store.ChannelGroup{
			store.DefaultGroupName: {ID: 1, Name: store.DefaultGroupName, PriceMultiplier: store.DefaultGroupPriceMultiplier, Status: 1},
			"vip":                  {ID: 2, Name: "vip", PriceMultiplier: decimal.RequireFromString("0.1"), Status: 1},
		},
		groupNameByID: map[int64]string{
			1: store.DefaultGroupName,
			2: "vip",
		},
	}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	usage := &recordingUsage{}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"vip", store.DefaultGroupName}}

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
	if gotTargetGroup != "vip" {
		t.Fatalf("expected target group vip, got=%q", gotTargetGroup)
	}
	if strings.TrimSpace(gotRouteKeyHash) == "" {
		t.Fatalf("expected route key hash header")
	}
	if got := rr.Header().Get("X-Realms-Route-Key-Source"); got != "payload" {
		t.Fatalf("expected route key source payload, got=%q", got)
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
	if q.commitCalls[0].RouteGroup == nil || *q.commitCalls[0].RouteGroup != "vip" {
		t.Fatalf("expected route group vip, got=%v", q.commitCalls[0].RouteGroup)
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

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: store.DefaultGroupName, Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, compactGateway)

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

func TestResponsesCompact_Remote_Upstream4xxDoesNotRequireRouteGroup(t *testing.T) {
	var attempts []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts = append(attempts, r.Header.Get(upstream.CompactGatewayTargetGroupHeader))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"bad"}}`)
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "g1", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	usage := &recordingUsage{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"g1", "g2"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(attempts) != 1 || attempts[0] != "g1" {
		t.Fatalf("expected only first group attempt, got=%v", attempts)
	}
	if len(q.commitCalls) != 0 {
		t.Fatalf("expected commit not called, got=%d", len(q.commitCalls))
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected void called once, got=%d", len(q.voidCalls))
	}
	if len(usage.calls) == 0 {
		t.Fatalf("expected usage finalized")
	}
}

func TestResponsesCompact_Remote_PropagatesPriorityServiceTierToQuota(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: store.DefaultGroupName, Status: 1, PriorityPricingEnabled: true, PriorityInputUSDPer1M: decimalPtrForResponsesCompactTest(t, "1"), PriorityOutputUSDPer1M: decimalPtrForResponsesCompactTest(t, "2")}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1","service_tier":"fast"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.reserveCalls) != 1 {
		t.Fatalf("expected reserve called once, got=%d", len(q.reserveCalls))
	}
	if q.reserveCalls[0].ServiceTier == nil || *q.reserveCalls[0].ServiceTier != "priority" {
		t.Fatalf("reserve service_tier=%v, want priority", q.reserveCalls[0].ServiceTier)
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected commit called once, got=%d", len(q.commitCalls))
	}
	if q.commitCalls[0].ServiceTier == nil || *q.commitCalls[0].ServiceTier != "priority" {
		t.Fatalf("commit service_tier=%v, want priority", q.commitCalls[0].ServiceTier)
	}
}

func TestResponsesCompact_Remote_FinalizeUsageCapturesBoundUpstreamModel(t *testing.T) {
	var gotRequestModel string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotRequestModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","model":"gpt-5.2","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{
		models: map[string]store.ManagedModel{
			"alias": {PublicID: "alias", GroupName: "vip", Status: 1},
		},
		bindings: map[string][]store.ChannelModelBinding{
			"alias": {
				{ID: 1, ChannelID: 101, ChannelGroups: "vip", PublicID: "alias", UpstreamModel: "gpt-5.2", Status: 1},
			},
		},
		groupByName: map[string]store.ChannelGroup{
			store.DefaultGroupName: {ID: 1, Name: store.DefaultGroupName, PriceMultiplier: store.DefaultGroupPriceMultiplier, Status: 1},
			"vip":                  {ID: 2, Name: "vip", PriceMultiplier: decimal.RequireFromString("0.1"), Status: 1},
		},
		groupNameByID: map[int64]string{
			1: store.DefaultGroupName,
			2: "vip",
		},
	}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	usage := &recordingUsage{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"vip"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"alias","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if gotRequestModel != "gpt-5.2" {
		t.Fatalf("expected compact gateway request model=%q, got=%q", "gpt-5.2", gotRequestModel)
	}
	if len(usage.calls) != 1 {
		t.Fatalf("expected 1 finalize call, got=%d", len(usage.calls))
	}
	if usage.calls[0].ForwardedModel == nil || *usage.calls[0].ForwardedModel != "gpt-5.2" {
		t.Fatalf("expected forwarded_model=%q, got=%v", "gpt-5.2", usage.calls[0].ForwardedModel)
	}
	if usage.calls[0].UpstreamResponseModel == nil || *usage.calls[0].UpstreamResponseModel != "gpt-5.2" {
		t.Fatalf("expected upstream_response_model=%q, got=%v", "gpt-5.2", usage.calls[0].UpstreamResponseModel)
	}
}

func TestResponsesCompact_Remote_FallsBackToSingleEffectiveGroup(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "solo", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"solo"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected commit called once, got=%d", len(q.commitCalls))
	}
	if q.commitCalls[0].RouteGroup == nil || *q.commitCalls[0].RouteGroup != "solo" {
		t.Fatalf("expected route group solo, got=%v", q.commitCalls[0].RouteGroup)
	}
}

func TestResponsesCompact_Remote_RejectsModelOutsideAllowedGroups(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok"}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "g1", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"solo"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "无权限使用该模型") {
		t.Fatalf("expected unauthorized model error, got body=%s", rr.Body.String())
	}
	if attempts != 0 {
		t.Fatalf("expected no gateway attempts, got=%d", attempts)
	}
	if len(q.reserveCalls) != 0 {
		t.Fatalf("expected reserve not called, got=%d", len(q.reserveCalls))
	}
}

func TestResponsesCompact_Remote_ExhaustsGroupsWhenGatewayReportsGroupUnavailable(t *testing.T) {
	var attempts []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetGroup := r.Header.Get(upstream.CompactGatewayTargetGroupHeader)
		attempts = append(attempts, targetGroup)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(upstream.CompactGatewayErrorClassHeader, "group_unavailable")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"group unavailable"}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "g1", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	usage := &recordingUsage{}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"g1", "g2"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "上游不可用") {
		t.Fatalf("expected upstream unavailable error, got body=%s", rr.Body.String())
	}
	if len(attempts) != 2 || attempts[0] != "g1" || attempts[1] != "g2" {
		t.Fatalf("expected attempts [g1 g2], got=%v", attempts)
	}
	if len(q.commitCalls) != 0 {
		t.Fatalf("expected commit not called, got=%d", len(q.commitCalls))
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected void called once, got=%d", len(q.voidCalls))
	}
	if len(usage.calls) == 0 || usage.calls[0].ErrorClass == nil || *usage.calls[0].ErrorClass != "upstream_unavailable" {
		t.Fatalf("expected upstream_unavailable usage finalization, got=%+v", usage.calls)
	}
}

func TestResponsesCompact_Remote_FailsOverToNextGroupOnGroupUnavailable(t *testing.T) {
	var attempts []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetGroup := r.Header.Get(upstream.CompactGatewayTargetGroupHeader)
		attempts = append(attempts, targetGroup)
		w.Header().Set("Content-Type", "application/json")
		if targetGroup == "g1" {
			w.Header().Set(upstream.CompactGatewayErrorClassHeader, "group_unavailable")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"group unavailable"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "g1", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	usage := &recordingUsage{}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"g1", "g2"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(attempts) != 2 || attempts[0] != "g1" || attempts[1] != "g2" {
		t.Fatalf("expected attempts [g1 g2], got=%v", attempts)
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected commit called once, got=%d", len(q.commitCalls))
	}
	if q.commitCalls[0].RouteGroup == nil || *q.commitCalls[0].RouteGroup != "g2" {
		t.Fatalf("expected route group g2, got=%v", q.commitCalls[0].RouteGroup)
	}
	if len(q.voidCalls) != 0 {
		t.Fatalf("expected void not called, got=%d", len(q.voidCalls))
	}
}

func TestResponsesCompact_Remote_FailsOverToNextGroupOnGatewayTimeout(t *testing.T) {
	var attempts []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetGroup := r.Header.Get(upstream.CompactGatewayTargetGroupHeader)
		attempts = append(attempts, targetGroup)
		if targetGroup == "g1" {
			time.Sleep(50 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "g1", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	usage := &recordingUsage{}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 10*time.Millisecond)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"g1", "g2"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(attempts) != 2 || attempts[0] != "g1" || attempts[1] != "g2" {
		t.Fatalf("expected attempts [g1 g2], got=%v", attempts)
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected commit called once, got=%d", len(q.commitCalls))
	}
	if q.commitCalls[0].RouteGroup == nil || *q.commitCalls[0].RouteGroup != "g2" {
		t.Fatalf("expected route group g2, got=%v", q.commitCalls[0].RouteGroup)
	}
	if len(q.voidCalls) != 0 {
		t.Fatalf("expected void not called, got=%d", len(q.voidCalls))
	}
}

func TestResponsesCompact_Remote_ClientCancelVoidsQuota(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: store.DefaultGroupName, Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(auth.WithPrincipal(ctx, p))
	cancel()

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != 499 {
		t.Fatalf("expected 499, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.reserveCalls) != 1 {
		t.Fatalf("expected reserve called once, got=%d", len(q.reserveCalls))
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected void called once, got=%d", len(q.voidCalls))
	}
}

func TestResponsesCompact_Remote_RequestTimeoutVoidsQuota(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: store.DefaultGroupName, Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, &recordingUsage{}, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{store.DefaultGroupName}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Millisecond)
	defer cancel()
	req = req.WithContext(auth.WithPrincipal(ctx, p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.reserveCalls) != 1 {
		t.Fatalf("expected reserve called once, got=%d", len(q.reserveCalls))
	}
	if len(q.voidCalls) != 1 {
		t.Fatalf("expected void called once, got=%d", len(q.voidCalls))
	}
}

func TestResponsesCompact_Remote_IgnoresRouteGroupResponseHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Realms-Route-Group", "vip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer ts.Close()

	fs := &fakeStore{models: map[string]store.ManagedModel{"gpt-5.2": {PublicID: "gpt-5.2", GroupName: "g1", Status: 1}}}
	sched := scheduler.New(fs)
	q := &fakeQuota{}
	usage := &recordingUsage{}
	features := staticFeatures{fs: store.FeatureState{BillingDisabled: false}}
	compactGateway := upstream.NewCompactGatewayClient(ts.URL, "gwk_test", 5*time.Second)
	h := NewHandler(fs, fs, sched, DoerFunc(func(_ context.Context, _ scheduler.Selection, _ *http.Request, _ []byte) (*http.Response, error) {
		t.Fatalf("unexpected scheduler-based upstream call")
		return nil, nil
	}), nil, features, q, fakeAudit{}, usage, nil, upstream.SSEPumpOptions{}, compactGateway)

	tokenID := int64(123)
	p := auth.Principal{ActorType: auth.ActorTypeToken, UserID: 10, Role: store.UserRoleUser, TokenID: &tokenID, Groups: []string{"g1", "g2"}}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses/compact", bytes.NewReader([]byte(`{"model":"gpt-5.2","input":"hi","session_id":"s1"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	middleware.Chain(http.HandlerFunc(h.ResponsesCompact), middleware.BodyCache(1<<20)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(q.commitCalls) != 1 {
		t.Fatalf("expected commit called once, got=%d", len(q.commitCalls))
	}
	if q.commitCalls[0].RouteGroup == nil || *q.commitCalls[0].RouteGroup != "g1" {
		t.Fatalf("expected route group g1 from local selection, got=%v", q.commitCalls[0].RouteGroup)
	}
	if len(q.voidCalls) != 0 {
		t.Fatalf("expected void not called, got=%d", len(q.voidCalls))
	}
	if got := rr.Header().Get("X-Realms-Route-Group"); got != "" {
		t.Fatalf("expected internal route group header stripped, got=%q", got)
	}
}

func decimalPtrForResponsesCompactTest(t *testing.T, v string) *decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(v)
	if err != nil {
		t.Fatalf("decimal parse: %v", err)
	}
	return &d
}
