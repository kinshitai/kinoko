package embedding

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", cfg.Provider)
	}
	if cfg.Model != "text-embedding-3-small" {
		t.Fatalf("Model = %q", cfg.Model)
	}
	if cfg.Dims != 1536 {
		t.Fatalf("Dims = %d, want 1536", cfg.Dims)
	}
	if cfg.BaseURL != "https://api.openai.com" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Retry.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d", cfg.Retry.MaxRetries)
	}
	if cfg.Retry.InitialBackoff != time.Second {
		t.Fatalf("InitialBackoff = %v", cfg.Retry.InitialBackoff)
	}
	if cfg.Retry.BackoffFactor != 2.0 {
		t.Fatalf("BackoffFactor = %f", cfg.Retry.BackoffFactor)
	}
	if cfg.CircuitBreaker.FailureThreshold != 5 {
		t.Fatalf("FailureThreshold = %d", cfg.CircuitBreaker.FailureThreshold)
	}
}

func TestPermanentError_Error(t *testing.T) {
	pe := &permanentError{err: fmt.Errorf("bad request")}
	got := pe.Error()
	if got != "bad request" {
		t.Fatalf("Error() = %q, want 'bad request'", got)
	}
}

func TestTruncateBody_Short(t *testing.T) {
	body := []byte("short")
	got := truncateBody(body)
	if got != "short" {
		t.Fatalf("truncateBody(short) = %q", got)
	}
}

func TestTruncateBody_ExactBoundary(t *testing.T) {
	body := make([]byte, maxErrorBodyLog)
	for i := range body {
		body[i] = 'a'
	}
	got := truncateBody(body)
	if len(got) != maxErrorBodyLog {
		t.Fatalf("expected len=%d, got %d", maxErrorBodyLog, len(got))
	}
}

func TestTruncateBody_Long(t *testing.T) {
	body := make([]byte, maxErrorBodyLog+100)
	for i := range body {
		body[i] = 'b'
	}
	got := truncateBody(body)
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("expected truncated suffix, got %q", got[len(got)-20:])
	}
	if len(got) != maxErrorBodyLog+len("...(truncated)") {
		t.Fatalf("unexpected length: %d", len(got))
	}
}
