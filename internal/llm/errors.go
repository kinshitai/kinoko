package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// LLMError is a structured error carrying an HTTP status code from an LLM call.
type LLMError struct {
	StatusCode int
	Message    string
}

func (e *LLMError) Error() string {
	return fmt.Sprintf("llm error (status %d): %s", e.StatusCode, e.Message)
}

// IsRetryable returns true for errors that should be retried.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr.StatusCode == 429 ||
			(llmErr.StatusCode >= 500 && llmErr.StatusCode <= 599)
	}
	if IsTimeout(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "unavailable")
}

// IsTimeout checks if an error represents a timeout.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "Timeout")
}

// IsRateLimit checks if an error represents a rate limit (HTTP 429).
func IsRateLimit(err error) bool {
	if err == nil {
		return false
	}
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr.StatusCode == 429
	}
	msg := err.Error()
	return strings.Contains(msg, "rate limit")
}
