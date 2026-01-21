package middleware

import (
	"encoding/hex"
	"errors"
	"testing"
)

func TestNewRequestID_IsHex32(t *testing.T) {
	rid := newRequestID()
	if len(rid) != 32 {
		t.Fatalf("expected request_id length 32, got %d (%q)", len(rid), rid)
	}
	if _, err := hex.DecodeString(rid); err != nil {
		t.Fatalf("expected request_id to be hex, got %q: %v", rid, err)
	}
}

func TestNewRequestID_FallbackAvoidsCollisions(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) {
		return 0, errors.New("rand unavailable")
	}
	t.Cleanup(func() {
		randRead = old
	})

	a := newRequestID()
	b := newRequestID()
	if a == b {
		t.Fatalf("expected fallback request_id to differ, got %q", a)
	}
	if len(a) != 32 || len(b) != 32 {
		t.Fatalf("expected fallback request_id length 32, got %d and %d", len(a), len(b))
	}
}
