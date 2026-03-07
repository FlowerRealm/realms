package concurrency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestNewManagerDerivesWaitTTLFromTimeoutAndBackoff(t *testing.T) {
	mgr := NewManager(Options{
		WaitTimeout: 45 * time.Second,
		MaxBackoff:  1500 * time.Millisecond,
	})

	want := 45*time.Second + 1500*time.Millisecond + time.Second
	if got := mgr.opts.WaitTTL; got != want {
		t.Fatalf("WaitTTL = %v, want %v", got, want)
	}
}

func TestAcquireUserSlotWithWaitRefreshesWaitTTLOnEveryIncrement(t *testing.T) {
	s := miniredis.RunT(t)
	mgr := NewManager(Options{
		Addr:           s.Addr(),
		KeyPrefix:      "test-refresh",
		WaitTTL:        4 * time.Second,
		WaitTimeout:    5 * time.Second,
		WaitQueueExtra: 2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = mgr.Close() }()

	holderRelease, err := mgr.AcquireUserSlotWithWait(context.Background(), 42, 1, nil)
	if err != nil {
		t.Fatalf("AcquireUserSlotWithWait(holder): %v", err)
	}
	defer holderRelease()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	errCh1 := make(chan error, 1)
	go func() {
		_, err := mgr.AcquireUserSlotWithWait(ctx1, 42, 1, nil)
		errCh1 <- err
	}()

	waitKey := mgr.key("wait:user:42")
	waitForCondition(t, func() bool {
		v, err := s.Get(waitKey)
		return err == nil && v == "1"
	})

	s.FastForward(2 * time.Second)
	ttlAfterFastForward := s.TTL(waitKey)
	if ttlAfterFastForward <= 0 || ttlAfterFastForward >= 4*time.Second {
		t.Fatalf("ttl after fast forward = %v, want (0s, 4s)", ttlAfterFastForward)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	errCh2 := make(chan error, 1)
	go func() {
		_, err := mgr.AcquireUserSlotWithWait(ctx2, 42, 1, nil)
		errCh2 <- err
	}()

	waitForCondition(t, func() bool {
		v, err := s.Get(waitKey)
		return err == nil && v == "2"
	})

	ttlAfterSecondIncrement := s.TTL(waitKey)
	if ttlAfterSecondIncrement <= ttlAfterFastForward {
		t.Fatalf("ttl after second increment = %v, want > %v", ttlAfterSecondIncrement, ttlAfterFastForward)
	}

	cancel2()
	if err := <-errCh2; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter2 err = %v, want context.Canceled", err)
	}
	cancel1()
	if err := <-errCh1; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter1 err = %v, want context.Canceled", err)
	}
}

func TestAcquireUserSlotWithLongWaitTimeoutKeepsQueueGuardPastLegacyTTL(t *testing.T) {
	s := miniredis.RunT(t)
	mgr := NewManager(Options{
		Addr:           s.Addr(),
		KeyPrefix:      "test-legacy-ttl",
		WaitTimeout:    3 * time.Minute,
		WaitQueueExtra: 0,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = mgr.Close() }()

	holderRelease, err := mgr.AcquireUserSlotWithWait(context.Background(), 7, 1, nil)
	if err != nil {
		t.Fatalf("AcquireUserSlotWithWait(holder): %v", err)
	}
	defer holderRelease()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	errCh1 := make(chan error, 1)
	go func() {
		_, err := mgr.AcquireUserSlotWithWait(ctx1, 7, 1, nil)
		errCh1 <- err
	}()

	waitKey := mgr.key("wait:user:7")
	waitForCondition(t, func() bool {
		v, err := s.Get(waitKey)
		return err == nil && v == "1"
	})

	s.FastForward(121 * time.Second)
	if !s.Exists(waitKey) {
		t.Fatalf("wait key %q expired before derived wait TTL", waitKey)
	}

	_, err = mgr.AcquireUserSlotWithWait(context.Background(), 7, 1, nil)
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("second waiter err = %v, want %v", err, ErrQueueFull)
	}

	cancel1()
	if err := <-errCh1; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter1 err = %v, want context.Canceled", err)
	}
}
