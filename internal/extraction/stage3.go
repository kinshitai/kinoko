package extraction

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kinoko-dev/kinoko/internal/circuitbreaker"
	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/llmutil"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// maxContentBytes is the truncation limit for session content sent to the LLM.
const maxContentBytes = 100 * 1024

// Stage3Critic evaluates a session via LLM and returns an extract/reject verdict.
type Stage3Critic interface {
	Evaluate(ctx context.Context, session model.SessionRecord, content []byte, stage2 *model.Stage2Result) (*model.Stage3Result, error)
}

// clockFunc allows injecting a time source for testing.
type clockFunc func() time.Time

// sleepFunc allows injecting a sleep function for testing.
type sleepFunc func(d time.Duration)

type stage3Critic struct {
	llmClient llm.LLMClient
	llmV2     llm.LLMClientV2 // optional, for token usage + timeout control
	cfg       config.ExtractionConfig
	log       *slog.Logger
	cb        *circuitbreaker.Breaker

	// Injectable clock/sleep for testing
	clock clockFunc
	sleep sleepFunc
}

// NewStage3Critic creates a Stage3Critic.
func NewStage3Critic(
	llmClient llm.LLMClient,
	cfg config.ExtractionConfig,
	log *slog.Logger,
) Stage3Critic {
	c := &stage3Critic{
		llmClient: llmClient,
		cfg:       cfg,
		log:       log,
		cb: mustNewBreaker(circuitbreaker.Config{
			Threshold:    5,
			BaseDuration: 5 * time.Minute,
			MaxDuration:  30 * time.Minute,
		}),
		clock: time.Now,
		sleep: time.Sleep,
	}
	if v2, ok := llmClient.(llm.LLMClientV2); ok {
		c.llmV2 = v2
	}
	return c
}

func mustNewBreaker(cfg circuitbreaker.Config) *circuitbreaker.Breaker {
	b, err := circuitbreaker.New(cfg, nil)
	if err != nil {
		panic(fmt.Sprintf("extraction: invalid circuit breaker config: %v", err))
	}
	return b
}

func (c *stage3Critic) Evaluate(ctx context.Context, session model.SessionRecord, content []byte, stage2 *model.Stage2Result) (*model.Stage3Result, error) {
	start := c.clock()

	// Input validation
	if stage2 == nil {
		return nil, errors.New("stage3: nil stage2 result")
	}
	if len(content) == 0 {
		return nil, errors.New("stage3: empty content")
	}
	if !stage2.Passed {
		return nil, errors.New("stage3: stage2 did not pass")
	}

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.log.Info("stage3 evaluate start", "session_id", session.ID)

	// Check circuit breaker
	if err := c.cb.Allow(); err != nil {
		return nil, err
	}

	// Truncate content if needed
	truncated := truncateContent(content, maxContentBytes)
	if len(truncated) < len(content) {
		c.log.Warn("stage3 content truncated", "session_id", session.ID,
			"original_bytes", len(content), "truncated_bytes", len(truncated))
	}

	// Build prompt
	prompt := buildCriticPrompt(truncated, stage2)

	// Call LLM with retry
	retryResult, err := c.callWithRetry(ctx, prompt, session.ID)
	if err != nil {
		c.cb.RecordFailure()
		elapsed := c.clock().Sub(start).Milliseconds()
		c.log.Error("stage3 LLM call failed", "session_id", session.ID, "error", err, "latency_ms", elapsed)
		return nil, fmt.Errorf("stage3: llm call: %w", err)
	}

	c.cb.RecordSuccess()

	// Parse response
	result, err := c.parseAndValidate(retryResult.content)
	if err != nil {
		elapsed := c.clock().Sub(start).Milliseconds()
		c.log.Warn("stage3 parse error, treating as rejection", "session_id", session.ID, "error", err)
		return &model.Stage3Result{
			Passed:          false,
			CriticVerdict:   "reject",
			CriticReasoning: fmt.Sprintf("critic_parse_error: %v", err),
			LatencyMs:       elapsed,
			TokensUsed:      retryResult.tokensUsed,
		}, nil
	}

	result.TokensUsed = retryResult.tokensUsed
	result.Retries = retryResult.retries
	result.LatencyMs = c.clock().Sub(start).Milliseconds()
	result.CircuitBreakerState = c.cb.State()

	c.log.Info("stage3 verdict", "session_id", session.ID,
		"verdict", result.CriticVerdict,
		"passed", result.Passed,
		"confidence", result.RefinedScores.CriticConfidence,
		"tokens_used", result.TokensUsed,
		"latency_ms", result.LatencyMs)

	return result, nil
}

// criticResponse is the expected JSON from the LLM.
type criticResponse struct {
	Verdict     string           `json:"verdict"`
	Reasoning   string           `json:"reasoning"`
	Scores      rubricScoresJSON `json:"refined_scores"`
	Confidence  float64          `json:"confidence"`
	Reusable    bool             `json:"reusable_pattern"`
	Explicit    bool             `json:"explicit_reasoning"`
	Contradicts bool             `json:"contradicts_best_practices"`
}

func (c *stage3Critic) parseAndValidate(resp string) (*model.Stage3Result, error) {
	cr, err := llmutil.ExtractJSON[criticResponse](resp)
	if err != nil {
		return nil, err
	}

	// Normalize verdict
	verdict := strings.ToLower(strings.TrimSpace(cr.Verdict))
	if verdict != "extract" && verdict != "reject" {
		return nil, fmt.Errorf("invalid verdict: %q", cr.Verdict)
	}

	// Validate scores
	if err := cr.Scores.validate(); err != nil {
		return nil, fmt.Errorf("invalid refined scores: %w", err)
	}

	// Clamp confidence
	conf := cr.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1.0 {
		conf = 1.0
	}

	scores := cr.Scores.toQualityScores()
	scores.CompositeScore = compositeScore(scores)
	scores.CriticConfidence = conf

	// Contradiction detection.
	// Case 1: extract but all/nearly-all scores are very low → override to reject.
	if verdict == "extract" && averageScore(scores) < 1.5 {
		c.log.Warn("stage3 contradiction: verdict=extract but scores extremely low, overriding to reject")
		verdict = "reject"
	}
	// Case 2: reject but all scores are very high → override to extract.
	if verdict == "reject" && allScoresAbove(scores, 4) {
		c.log.Warn("stage3 contradiction: verdict=reject but all scores>=5, overriding to extract")
		verdict = "extract"
	}

	passed := verdict == "extract"

	return &model.Stage3Result{
		Passed:                   passed,
		CriticVerdict:            verdict,
		CriticReasoning:          cr.Reasoning,
		RefinedScores:            scores,
		ReusablePattern:          cr.Reusable,
		ExplicitReasoning:        cr.Explicit,
		ContradictsBestPractices: cr.Contradicts,
	}, nil
}

// averageScore returns the mean of all 7 rubric scores.
func averageScore(q model.QualityScores) float64 {
	sum := q.ProblemSpecificity + q.SolutionCompleteness + q.ContextPortability +
		q.ReasoningTransparency + q.TechnicalAccuracy + q.VerificationEvidence + q.InnovationLevel
	return float64(sum) / 7.0
}

// allScoresAbove returns true if every score is >= threshold.
func allScoresAbove(q model.QualityScores, threshold int) bool {
	return q.ProblemSpecificity >= threshold &&
		q.SolutionCompleteness >= threshold &&
		q.ContextPortability >= threshold &&
		q.ReasoningTransparency >= threshold &&
		q.TechnicalAccuracy >= threshold &&
		q.VerificationEvidence >= threshold &&
		q.InnovationLevel >= threshold
}

// retryCallResult holds the result of callWithRetry.
type retryCallResult struct {
	content    string
	tokensUsed int
	retries    int
}

func (c *stage3Critic) callWithRetry(ctx context.Context, prompt string, sessionID string) (*retryCallResult, error) {
	maxRetries := baseMaxRetries()
	var lastErr error
	totalTokens := 0

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			backoff := time.Second * time.Duration(1<<(attempt-1)) // 1s, 2s, 4s
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			c.log.Info("stage3 retry", "session_id", sessionID, "attempt", attempt, "backoff", backoff)
			c.sleep(backoff)
		}

		// Determine timeout: 30s normally, 60s on retry after timeout (spec §5.1).
		timeout := 30 * time.Second
		if attempt > 0 && lastErr != nil && llm.IsTimeout(lastErr) {
			timeout = 60 * time.Second
		}

		resp, tokens, err := c.callLLM(ctx, prompt, timeout)
		totalTokens += tokens
		if err == nil {
			return &retryCallResult{content: resp, tokensUsed: totalTokens, retries: attempt}, nil
		}

		lastErr = err

		// Update max retries for rate limits.
		if llm.IsRateLimit(err) && maxRetries < 5 {
			maxRetries = 5
		}

		if !llm.IsRetryable(err) {
			return nil, err
		}
	}

	return nil, lastErr
}

// baseMaxRetries returns the base max retry count.
func baseMaxRetries() int {
	return 3
}

// callLLM calls the LLM with the given timeout. Returns response, token count, error.
func (c *stage3Critic) callLLM(ctx context.Context, prompt string, timeout time.Duration) (string, int, error) {
	if c.llmV2 != nil {
		result, err := c.llmV2.CompleteWithTimeout(ctx, prompt, timeout)
		if err != nil {
			return "", 0, err
		}
		return result.Content, result.TokensIn + result.TokensOut, nil
	}
	// Fallback: use basic client with context timeout.
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := c.llmClient.Complete(callCtx, prompt)
	if err != nil {
		return "", 0, err
	}
	// Estimate tokens from content length when no V2 interface.
	tokens := estimateTokens(prompt, resp)
	return resp, tokens, nil
}

// estimateTokens provides a rough token estimate (~4 chars per token).
func estimateTokens(prompt, response string) int {
	return (len(prompt) + len(response)) / 4
}
