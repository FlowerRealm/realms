package limits

import "testing"

func TestChannelLimits_AcquireRelease(t *testing.T) {
	l := NewChannelLimits()
	limit := 2

	if !l.Acquire(1, &limit) {
		t.Fatalf("expected first acquire ok")
	}
	if !l.Acquire(1, &limit) {
		t.Fatalf("expected second acquire ok")
	}
	if l.Acquire(1, &limit) {
		t.Fatalf("expected third acquire to be blocked")
	}

	l.Release(1)
	if !l.Acquire(1, &limit) {
		t.Fatalf("expected acquire ok after release")
	}
}

func TestChannelLimits_Unlimited(t *testing.T) {
	l := NewChannelLimits()

	if !l.Acquire(1, nil) {
		t.Fatalf("expected nil limit to be unlimited")
	}
	zero := 0
	if !l.Acquire(1, &zero) {
		t.Fatalf("expected zero limit to be treated as unlimited")
	}
	neg := -1
	if !l.Acquire(1, &neg) {
		t.Fatalf("expected negative limit to be treated as unlimited")
	}
}
