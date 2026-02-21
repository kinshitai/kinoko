package extraction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/shared/circuitbreaker"
)

// Tests for the stage3 circuit breaker state transitions.
// These mirror the embedding circuit breaker tests to demonstrate duplication (tech debt C.1).

func TestStage3CB_ClosedToOpenToHalfOpenToClosed(t *testing.T) {
	now := time.Now()
	shouldFail := true

	llm := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		if shouldFail {
			return "", &llm.LLMError{StatusCode: 503, Message: "down"}
		}
		return extractVerdictJSON(), nil
	}}

	critic := newTestCriticWithClock(llm, func() time.Time { return now })

	// Closed → Open: 5 consecutive failures
	for i := 0; i < 5; i++ {
		_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
		if err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}

	// Verify open
	_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected circuitbreaker.ErrOpen, got %v", err)
	}

	// Open → Half-open: advance past open duration
	now = now.Add(6 * time.Minute)
	shouldFail = false

	// Half-open → Closed: probe succeeds
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
	if err != nil {
		t.Fatalf("half-open probe should succeed: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass after recovery")
	}

	// Verify closed: subsequent calls work
	result2, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
	if err != nil {
		t.Fatalf("closed circuit should work: %v", err)
	}
	if !result2.Passed {
		t.Error("expected pass")
	}
}

func TestStage3CB_HalfOpenFailEscalates(t *testing.T) {
	now := time.Now()

	llm := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		return "", &llm.LLMError{StatusCode: 503, Message: "down"}
	}}

	critic := newTestCriticWithClock(llm, func() time.Time { return now })

	// Open circuit
	for i := 0; i < 5; i++ {
		_, _ = critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
	}

	// Half-open probe fails → re-open with 10min
	now = now.Add(6 * time.Minute)
	_, _ = critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())

	// Still open at 5min after re-open
	now = now.Add(5 * time.Minute)
	_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected still open (doubled to 10min), got %v", err)
	}

	// Open at ~10min → half-open, fail again → re-open with 20min
	now = now.Add(6 * time.Minute)
	_, _ = critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())

	// Verify doubled again: still open after 10min
	now = now.Add(10 * time.Minute)
	_, err = critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected still open (doubled to 20min), got %v", err)
	}
}
