package extraction

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/shared/circuitbreaker"
)

func TestStage3Critic_Retry(t *testing.T) {
	tests := []struct {
		name       string
		failures   int
		failErr    error
		wantCalls  int
		wantErr    bool
		wantPassed bool
	}{
		{"succeeds first try", 0, nil, 1, false, true},
		{"fails once then succeeds", 1, &llm.LLMError{StatusCode: 503, Message: "unavailable"}, 2, false, true},
		{"fails 3 then succeeds", 3, &llm.LLMError{StatusCode: 500, Message: "internal"}, 4, false, true},
		{"fails 4 exceeds max retries", 4, &llm.LLMError{StatusCode: 502, Message: "bad gateway"}, 4, true, false},
		{"rate limit 429 gets 5 retries", 5, &llm.LLMError{StatusCode: 429, Message: "rate limit"}, 6, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			l := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
				callCount++
				if callCount <= tt.failures {
					return "", tt.failErr
				}
				return extractVerdictJSON(), nil
			}}
			critic := newTestCritic(l)
			result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2(), SourceTypeSession, "")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Passed != tt.wantPassed {
					t.Errorf("passed: got %v, want %v", result.Passed, tt.wantPassed)
				}
			}
			if callCount != tt.wantCalls {
				t.Errorf("calls: got %d, want %d", callCount, tt.wantCalls)
			}
		})
	}
}

func TestNonRetryableErrorSkipsRetry(t *testing.T) {
	callCount := 0
	l := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		callCount++
		return "", &llm.LLMError{StatusCode: 401, Message: "unauthorized"}
	}}
	critic := newTestCritic(l)
	_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2(), SourceTypeSession, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Errorf("non-retryable error should not retry, got %d calls", callCount)
	}
}

func TestStage3Critic_CircuitBreaker(t *testing.T) {
	t.Run("opens after 5 consecutive failures", func(t *testing.T) {
		l := s3errLLM(&llm.LLMError{StatusCode: 503, Message: "unavailable"})
		critic := newTestCritic(l)
		for i := 0; i < 5; i++ {
			_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2(), SourceTypeSession, "")
			if err == nil {
				t.Fatal("expected error")
			}
		}
		_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2(), SourceTypeSession, "")
		if !errors.Is(err, circuitbreaker.ErrOpen) {
			t.Errorf("expected circuitbreaker.ErrOpen, got %v", err)
		}
	})

	t.Run("half-open after duration success closes", func(t *testing.T) {
		now := time.Now()
		shouldFail := true
		l := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
			if shouldFail {
				return "", &llm.LLMError{StatusCode: 503, Message: "unavailable"}
			}
			return extractVerdictJSON(), nil
		}}
		critic := newTestCriticWithClock(l, func() time.Time { return now })
		for i := 0; i < 5; i++ {
			critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
		}
		now = now.Add(6 * time.Minute)
		shouldFail = false
		result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
		if err != nil {
			t.Fatalf("half-open should succeed: %v", err)
		}
		if !result.Passed {
			t.Error("expected pass")
		}
		result2, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
		if err != nil {
			t.Fatalf("closed circuit should work: %v", err)
		}
		if !result2.Passed {
			t.Error("expected pass")
		}
	})

	t.Run("success resets failure counter", func(t *testing.T) {
		succeedNext := false
		l := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
			if succeedNext {
				return extractVerdictJSON(), nil
			}
			return "", &llm.LLMError{StatusCode: 503, Message: "unavailable"}
		}}
		critic := newTestCritic(l)
		for i := 0; i < 4; i++ {
			critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
		}
		succeedNext = true
		_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
		if err != nil {
			t.Fatalf("expected success: %v", err)
		}
		succeedNext = false
		for i := 0; i < 4; i++ {
			_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
			if errors.Is(err, circuitbreaker.ErrOpen) {
				t.Fatalf("circuit should not be open after reset, failed on call %d", i+1)
			}
		}
	})
}

func TestStage3Critic_HalfOpenFailureDoublesDuration(t *testing.T) {
	now := time.Now()
	shouldFail := true
	l := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		if shouldFail {
			return "", &llm.LLMError{StatusCode: 503, Message: "unavailable"}
		}
		return extractVerdictJSON(), nil
	}}
	critic := newTestCriticWithClock(l, func() time.Time { return now })
	for i := 0; i < 5; i++ {
		critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
	}
	now = now.Add(6 * time.Minute)
	_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
	if err == nil {
		t.Fatal("expected error from half-open probe failure")
	}
	now = now.Add(5 * time.Minute)
	_, err = critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Errorf("circuit should still be open after 5 min, got %v", err)
	}
	now = now.Add(6 * time.Minute)
	shouldFail = false
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
	if err != nil {
		t.Fatalf("should succeed after doubled duration: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass")
	}
}

func TestStage3Critic_ConcurrentHalfOpen(t *testing.T) {
	now := time.Now()
	var probeCount int32
	l := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		atomic.AddInt32(&probeCount, 1)
		time.Sleep(10 * time.Millisecond)
		return extractVerdictJSON(), nil
	}}
	critic := newTestCriticWithClock(l, func() time.Time { return now })
	for i := 0; i < 5; i++ {
		_ = critic.cb.Allow()
		critic.cb.RecordFailure()
	}
	now = now.Add(6 * time.Minute)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2(), SourceTypeSession, "")
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&probeCount) == 0 {
		t.Error("expected at least one probe call")
	}
}

func TestStage3Critic_TimeoutEscalation(t *testing.T) {
	var timeouts []time.Duration
	l := &mockLLMV2{
		completeFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("timeout")
		},
		completeWithTimeoutFn: func(_ context.Context, _ string, timeout time.Duration) (*llm.LLMCompleteResult, error) {
			timeouts = append(timeouts, timeout)
			if len(timeouts) < 3 {
				return nil, errors.New("timeout")
			}
			return &llm.LLMCompleteResult{Content: extractVerdictJSON(), TokensIn: 50, TokensOut: 20}, nil
		},
	}
	c := NewStage3Critic(l, s3testConfig(), s3testLogger()).(*stage3Critic)
	c.sleep = func(d time.Duration) {}
	_, err := c.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2(), SourceTypeSession, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timeouts) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(timeouts))
	}
	if timeouts[0] != 30*time.Second {
		t.Errorf("first attempt timeout = %v, want 30s", timeouts[0])
	}
	if timeouts[1] != 60*time.Second {
		t.Errorf("retry after timeout should use 60s, got %v", timeouts[1])
	}
}
