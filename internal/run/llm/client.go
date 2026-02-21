// Package llm defines LLM client interfaces, error types, and implementations.
package llm

import (
	"context"
	"time"
)

// LLMClient is a lightweight LLM interface for rubric scoring.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// LLMCompleteResult is the return value from LLMClientV2.CompleteWithTimeout.
type LLMCompleteResult struct {
	Content   string
	TokensIn  int
	TokensOut int
}

// LLMClientV2 extends LLMClient with token usage and timeout control.
// Implementations should respect the context deadline/timeout.
type LLMClientV2 interface {
	LLMClient
	CompleteWithTimeout(ctx context.Context, prompt string, timeout time.Duration) (*LLMCompleteResult, error)
}
