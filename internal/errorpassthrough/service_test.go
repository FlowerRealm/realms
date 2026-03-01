package errorpassthrough

import (
	"context"
	"errors"
	"testing"
	"time"

	"realms/internal/store"
)

type fakeSource struct {
	rules []store.ErrorPassthroughRule
	err   error
}

func (f *fakeSource) ListErrorPassthroughRules(_ context.Context) ([]store.ErrorPassthroughRule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rules, nil
}

func TestMatch_AnyByStatusCode(t *testing.T) {
	svc := NewService(&fakeSource{
		rules: []store.ErrorPassthroughRule{
			{
				Enabled:         true,
				Priority:        1,
				ErrorCodes:      []int{429},
				Platforms:       []string{"openai"},
				MatchMode:       "any",
				PassthroughCode: true,
				PassthroughBody: false,
				CustomMessage:   ptr("请求太多，请稍后重试"),
			},
		},
	}, time.Minute)

	status, message, skip, matched := svc.Match("openai", 429, []byte(`{"error":{"message":"upstream raw"}}`))
	if !matched {
		t.Fatalf("expected matched=true")
	}
	if status != 429 {
		t.Fatalf("expected status=429, got=%d", status)
	}
	if message != "请求太多，请稍后重试" {
		t.Fatalf("unexpected message=%q", message)
	}
	if skip {
		t.Fatalf("expected skip=false")
	}
}

func TestMatch_AllModeNeedsKeywordAndStatus(t *testing.T) {
	svc := NewService(&fakeSource{
		rules: []store.ErrorPassthroughRule{
			{
				Enabled:         true,
				Priority:        1,
				ErrorCodes:      []int{400},
				Keywords:        []string{"context length"},
				MatchMode:       "all",
				Platforms:       []string{"openai"},
				PassthroughCode: false,
				ResponseCode:    intPtr(422),
				PassthroughBody: true,
			},
		},
	}, time.Minute)

	status, message, _, matched := svc.Match("openai", 400, []byte(`{"error":{"message":"context length exceeded"}}`))
	if !matched {
		t.Fatalf("expected matched=true")
	}
	if status != 422 {
		t.Fatalf("expected status=422, got=%d", status)
	}
	if message != "context length exceeded" {
		t.Fatalf("unexpected message=%q", message)
	}

	_, _, _, matched = svc.Match("openai", 400, []byte(`{"error":{"message":"other"}}`))
	if matched {
		t.Fatalf("expected no match when keyword missing")
	}
}

func TestMatch_UsesCachedRulesWhenReloadFails(t *testing.T) {
	src := &fakeSource{
		rules: []store.ErrorPassthroughRule{
			{
				Enabled:         true,
				Priority:        1,
				ErrorCodes:      []int{500},
				MatchMode:       "any",
				PassthroughCode: false,
				ResponseCode:    intPtr(503),
				PassthroughBody: false,
				CustomMessage:   ptr("服务暂不可用"),
			},
		},
	}
	svc := NewService(src, time.Second)

	if _, _, _, matched := svc.Match("openai", 500, []byte(`{"error":{"message":"boom"}}`)); !matched {
		t.Fatalf("expected initial match")
	}

	src.err = errors.New("db down")
	// 强制缓存过期，验证会退回上一次缓存。
	svc.mu.Lock()
	svc.expiresAt = time.Now().Add(-time.Second)
	svc.mu.Unlock()

	status, message, _, matched := svc.Match("openai", 500, []byte(`{"error":{"message":"boom"}}`))
	if !matched {
		t.Fatalf("expected cached match when reload fails")
	}
	if status != 503 || message != "服务暂不可用" {
		t.Fatalf("unexpected fallback result status=%d message=%q", status, message)
	}
}

func ptr(s string) *string { return &s }

func intPtr(v int) *int { return &v }
