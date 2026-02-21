package circuitbreaker

import (
	"testing"
	"time"
)

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time { return m.now }

func TestState_Transitions(t *testing.T) {
	clk := &mockClock{now: time.Now()}
	b, err := New(Config{
		Threshold:    2,
		BaseDuration: 100 * time.Millisecond,
		MaxDuration:  time.Second,
	}, clk)
	if err != nil {
		t.Fatal(err)
	}

	// Initially closed.
	if s := b.State(); s != "closed" {
		t.Fatalf("state = %q, want closed", s)
	}

	// Trip it.
	b.RecordFailure()
	b.RecordFailure()
	if s := b.State(); s != "open" {
		t.Fatalf("state = %q, want open", s)
	}

	// Advance past open duration → half-open.
	clk.now = clk.now.Add(150 * time.Millisecond)
	if s := b.State(); s != "half-open" {
		t.Fatalf("state = %q, want half-open", s)
	}

	// Allow transitions to half-open state, then success closes it.
	b.Allow()
	b.RecordSuccess()
	if s := b.State(); s != "closed" {
		t.Fatalf("state = %q, want closed", s)
	}
}
