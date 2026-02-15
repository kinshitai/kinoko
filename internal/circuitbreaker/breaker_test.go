package circuitbreaker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClock is an injectable clock for deterministic tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func testConfig() Config {
	return Config{
		Threshold:    3,
		BaseDuration: 5 * time.Minute,
		MaxDuration:  30 * time.Minute,
	}
}

func mustNew(t *testing.T, cfg Config, clk Clock) *Breaker {
	t.Helper()
	b, err := New(cfg, clk)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestClosedToOpen(t *testing.T) {
	clk := newFakeClock(time.Now())
	b := mustNew(t, testConfig(), clk)

	// Under threshold: still closed.
	for i := 0; i < 2; i++ {
		if err := b.Allow(); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		b.RecordFailure()
	}
	if err := b.Allow(); err != nil {
		t.Fatal("should still be closed after 2 failures")
	}

	// Third failure trips the breaker.
	b.RecordFailure()

	err := b.Allow()
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
}

func TestOpenToHalfOpenToClosed(t *testing.T) {
	clk := newFakeClock(time.Now())
	b := mustNew(t, testConfig(), clk)

	// Trip breaker.
	for i := 0; i < 3; i++ {
		b.Allow()
		b.RecordFailure()
	}

	// Still open before duration elapses.
	clk.Advance(4 * time.Minute)
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen at 4min, got %v", err)
	}

	// Half-open after duration.
	clk.Advance(2 * time.Minute) // total 6min > 5min
	if err := b.Allow(); err != nil {
		t.Fatalf("expected half-open allow, got %v", err)
	}

	// Success closes the breaker.
	b.RecordSuccess()
	if err := b.Allow(); err != nil {
		t.Fatalf("expected closed after success, got %v", err)
	}
}

func TestHalfOpenFailureEscalates(t *testing.T) {
	clk := newFakeClock(time.Now())
	cfg := testConfig()
	b := mustNew(t, cfg, clk)

	// Trip: 5min base.
	for i := 0; i < 3; i++ {
		b.Allow()
		b.RecordFailure()
	}

	// Half-open, then fail → re-open at 10min.
	clk.Advance(6 * time.Minute)
	b.Allow()
	b.RecordFailure()

	// Still open at 6min after re-open (need 10min).
	clk.Advance(6 * time.Minute)
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen (escalated to 10min), got %v", err)
	}

	// Half-open at 10min+, fail → re-open at 20min.
	clk.Advance(5 * time.Minute) // total 11min
	b.Allow()
	b.RecordFailure()

	// Still open at 10min (need 20min).
	clk.Advance(10 * time.Minute)
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen (escalated to 20min), got %v", err)
	}
}

func TestMaxDurationCap(t *testing.T) {
	clk := newFakeClock(time.Now())
	cfg := Config{
		Threshold:    1,
		BaseDuration: 10 * time.Minute,
		MaxDuration:  15 * time.Minute,
	}
	b := mustNew(t, cfg, clk)

	// Trip.
	b.Allow()
	b.RecordFailure()

	// Half-open fail → would double to 20min, capped at 15min.
	clk.Advance(11 * time.Minute)
	b.Allow()
	b.RecordFailure()

	// Not open at 14min.
	clk.Advance(14 * time.Minute)
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen at 14min (capped at 15min), got %v", err)
	}

	// Open at 16min.
	clk.Advance(2 * time.Minute)
	if err := b.Allow(); err != nil {
		t.Fatalf("expected half-open at 16min, got %v", err)
	}
}

func TestSuccessResetsBackoff(t *testing.T) {
	clk := newFakeClock(time.Now())
	b := mustNew(t, testConfig(), clk)

	// Trip, escalate once.
	for i := 0; i < 3; i++ {
		b.Allow()
		b.RecordFailure()
	}
	clk.Advance(6 * time.Minute)
	b.Allow()
	b.RecordFailure() // escalated to 10min

	// Wait for half-open, succeed.
	clk.Advance(11 * time.Minute)
	b.Allow()
	b.RecordSuccess()

	// Trip again: should use base duration (5min), not 10min.
	for i := 0; i < 3; i++ {
		b.Allow()
		b.RecordFailure()
	}

	clk.Advance(6 * time.Minute)
	if err := b.Allow(); err != nil {
		t.Fatalf("expected half-open at base 5min, got %v", err)
	}
}

func TestHalfOpenRejectsSecondCaller(t *testing.T) {
	clk := newFakeClock(time.Now())
	b := mustNew(t, testConfig(), clk)

	// Trip.
	for i := 0; i < 3; i++ {
		b.Allow()
		b.RecordFailure()
	}

	// First call enters half-open.
	clk.Advance(6 * time.Minute)
	if err := b.Allow(); err != nil {
		t.Fatalf("first half-open call should be allowed: %v", err)
	}

	// Second call while still half-open is rejected.
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("second half-open call should be rejected, got %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	clk := newFakeClock(time.Now())
	b := mustNew(t, testConfig(), clk)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Allow()
			b.RecordFailure()
			b.RecordSuccess()
		}()
	}
	wg.Wait()

	// Should not panic; state should be consistent.
	if err := b.Allow(); err != nil {
		t.Fatalf("unexpected error after concurrent access: %v", err)
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"zero threshold", Config{Threshold: 0, BaseDuration: time.Second, MaxDuration: time.Second}},
		{"negative threshold", Config{Threshold: -1, BaseDuration: time.Second, MaxDuration: time.Second}},
		{"zero base duration", Config{Threshold: 1, BaseDuration: 0, MaxDuration: time.Second}},
		{"negative base duration", Config{Threshold: 1, BaseDuration: -time.Second, MaxDuration: time.Second}},
		{"max < base", Config{Threshold: 1, BaseDuration: 10 * time.Second, MaxDuration: 5 * time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := New(tt.cfg, nil)
			if err == nil {
				t.Fatal("expected error for invalid config")
			}
			if b != nil {
				t.Fatal("expected nil breaker on error")
			}
		})
	}
}

func TestNew_ValidConfig(t *testing.T) {
	b, err := New(Config{Threshold: 1, BaseDuration: time.Second, MaxDuration: time.Second}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil breaker")
	}
}
