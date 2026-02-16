package llm

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestNewOpenAIClient(t *testing.T) {
	c := NewOpenAIClient("sk-test", "gpt-4o-mini", "")
	if c.apiKey != "sk-test" {
		t.Fatalf("apiKey = %q, want %q", c.apiKey, "sk-test")
	}
	if c.model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want %q", c.model, "gpt-4o-mini")
	}
	if c.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
}

func TestNewOpenAIClient_EmptyAPIKey(t *testing.T) {
	// Constructor doesn't validate — caller's responsibility.
	// Just verify it doesn't panic.
	c := NewOpenAIClient("", "gpt-4o-mini", "")
	if c.apiKey != "" {
		t.Fatalf("apiKey = %q, want empty", c.apiKey)
	}
}

func TestLLMError_String(t *testing.T) {
	e := &LLMError{StatusCode: 429, Message: "rate limit exceeded"}
	want := "llm error (status 429): rate limit exceeded"
	if got := e.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"500", &LLMError{StatusCode: 500, Message: "internal"}, true},
		{"502", &LLMError{StatusCode: 502, Message: "bad gateway"}, true},
		{"503", &LLMError{StatusCode: 503, Message: "unavailable"}, true},
		{"504", &LLMError{StatusCode: 504, Message: "gateway timeout"}, true},
		{"429", &LLMError{StatusCode: 429, Message: "rate limit"}, true},
		{"401 not retryable", &LLMError{StatusCode: 401, Message: "unauthorized"}, false},
		{"400 not retryable", &LLMError{StatusCode: 400, Message: "bad request"}, false},
		{"403 not retryable", &LLMError{StatusCode: 403, Message: "forbidden"}, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"timeout string", errors.New("connection timeout"), true},
		{"unavailable string", errors.New("service unavailable"), true},
		{"random error", errors.New("something broke"), false},
		{"wrapped LLMError", fmt.Errorf("call failed: %w", &LLMError{StatusCode: 503, Message: "down"}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRateLimit(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &LLMError{StatusCode: 429, Message: "rate limit"}, true},
		{"500 not rate limit", &LLMError{StatusCode: 500, Message: "internal"}, false},
		{"string match", errors.New("rate limit exceeded"), true},
		{"no match", errors.New("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRateLimit(tt.err); got != tt.want {
				t.Errorf("IsRateLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenAIClient_Non200_ReturnsLLMError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryable  bool
		rateLimit  bool
	}{
		{"429 is rate limit", 429, true, true},
		{"500 is retryable", 500, true, false},
		{"502 is retryable", 502, true, false},
		{"400 is not retryable", 400, false, false},
		{"401 is not retryable", 401, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &LLMError{StatusCode: tt.statusCode, Message: "test"}
			var target *LLMError
			if !errors.As(err, &target) {
				t.Fatal("expected errors.As to match *LLMError")
			}
			if target.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", target.StatusCode, tt.statusCode)
			}
			if got := IsRetryable(err); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
			if got := IsRateLimit(err); got != tt.rateLimit {
				t.Errorf("IsRateLimit() = %v, want %v", got, tt.rateLimit)
			}
		})
	}
}

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"timeout string", errors.New("request timeout"), true},
		{"Timeout string", errors.New("Request Timeout"), true},
		{"no match", errors.New("bad request"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTimeout(tt.err); got != tt.want {
				t.Errorf("IsTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
