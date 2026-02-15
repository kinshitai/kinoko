package extraction

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mycelium-dev/mycelium/internal/config"
)

// LLMError is a structured error carrying an HTTP status code from an LLM call.
type LLMError struct {
	StatusCode int
	Message    string
}

func (e *LLMError) Error() string {
	return fmt.Sprintf("llm error (status %d): %s", e.StatusCode, e.Message)
}

// LLMCompleteResult is the return value from LLMClientV2.CompleteV2.
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

// maxContentBytes is the truncation limit for session content sent to the LLM.
const maxContentBytes = 100 * 1024

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("stage3: circuit breaker open")

// Stage3Critic evaluates a session via LLM and returns an extract/reject verdict.
type Stage3Critic interface {
	Evaluate(ctx context.Context, session SessionRecord, content []byte, stage2 *Stage2Result) (*Stage3Result, error)
}

// clockFunc allows injecting a time source for testing.
type clockFunc func() time.Time

// sleepFunc allows injecting a sleep function for testing.
type sleepFunc func(d time.Duration)

type stage3Critic struct {
	llm   LLMClient
	llmV2 LLMClientV2 // optional, for token usage + timeout control
	cfg   config.ExtractionConfig
	log   *slog.Logger

	// Circuit breaker state
	mu              sync.Mutex
	consecutiveFail int
	circuitOpenAt   time.Time
	openDuration    time.Duration

	// Injectable clock/sleep for testing
	clock clockFunc
	sleep sleepFunc
}

// NewStage3Critic creates a Stage3Critic.
func NewStage3Critic(
	llm LLMClient,
	cfg config.ExtractionConfig,
	log *slog.Logger,
) Stage3Critic {
	c := &stage3Critic{
		llm:          llm,
		cfg:          cfg,
		log:          log,
		openDuration: 5 * time.Minute,
		clock:        time.Now,
		sleep:        time.Sleep,
	}
	if v2, ok := llm.(LLMClientV2); ok {
		c.llmV2 = v2
	}
	return c
}

func (c *stage3Critic) Evaluate(ctx context.Context, session SessionRecord, content []byte, stage2 *Stage2Result) (*Stage3Result, error) {
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
	if err := c.checkCircuit(); err != nil {
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
		c.recordFailure()
		elapsed := c.clock().Sub(start).Milliseconds()
		c.log.Error("stage3 LLM call failed", "session_id", session.ID, "error", err, "latency_ms", elapsed)
		return nil, fmt.Errorf("stage3: llm call: %w", err)
	}

	c.recordSuccess()

	// Parse response
	result, err := c.parseAndValidate(retryResult.content)
	if err != nil {
		elapsed := c.clock().Sub(start).Milliseconds()
		c.log.Warn("stage3 parse error, treating as rejection", "session_id", session.ID, "error", err)
		return &Stage3Result{
			Passed:          false,
			CriticVerdict:   "reject",
			CriticReasoning: fmt.Sprintf("critic_parse_error: %v", err),
			LatencyMs:       elapsed,
			TokensUsed:      retryResult.tokensUsed,
		}, nil
	}

	result.TokensUsed = retryResult.tokensUsed
	result.LatencyMs = c.clock().Sub(start).Milliseconds()

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
	Verdict    string           `json:"verdict"`
	Reasoning  string           `json:"reasoning"`
	Scores     rubricScoresJSON `json:"refined_scores"`
	Confidence float64          `json:"confidence"`
	Reusable   bool             `json:"reusable_pattern"`
	Explicit   bool             `json:"explicit_reasoning"`
	Contradicts bool            `json:"contradicts_best_practices"`
}

func (c *stage3Critic) parseAndValidate(resp string) (*Stage3Result, error) {
	var cr criticResponse
	if err := parseCriticResponse(resp, &cr); err != nil {
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

	return &Stage3Result{
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
func averageScore(q QualityScores) float64 {
	sum := q.ProblemSpecificity + q.SolutionCompleteness + q.ContextPortability +
		q.ReasoningTransparency + q.TechnicalAccuracy + q.VerificationEvidence + q.InnovationLevel
	return float64(sum) / 7.0
}

// allScoresAbove returns true if every score is >= threshold.
func allScoresAbove(q QualityScores, threshold int) bool {
	return q.ProblemSpecificity >= threshold &&
		q.SolutionCompleteness >= threshold &&
		q.ContextPortability >= threshold &&
		q.ReasoningTransparency >= threshold &&
		q.TechnicalAccuracy >= threshold &&
		q.VerificationEvidence >= threshold &&
		q.InnovationLevel >= threshold
}

func parseCriticResponse(resp string, out *criticResponse) error {
	if strings.TrimSpace(resp) == "" {
		return errors.New("empty LLM response")
	}

	// Strategy 1: direct parse
	if err := json.Unmarshal([]byte(resp), out); err == nil {
		return nil
	}

	// Strategy 2: ```json blocks
	if start := strings.Index(resp, "```json"); start >= 0 {
		inner := resp[start+7:]
		if end := strings.Index(inner, "```"); end >= 0 {
			if err := json.Unmarshal([]byte(strings.TrimSpace(inner[:end])), out); err == nil {
				return nil
			}
		}
	}

	// Strategy 3: generic ``` blocks
	if start := strings.Index(resp, "```"); start >= 0 {
		inner := resp[start+3:]
		if end := strings.Index(inner, "```"); end >= 0 {
			candidate := strings.TrimSpace(inner[:end])
			if err := json.Unmarshal([]byte(candidate), out); err == nil {
				return nil
			}
		}
	}

	// Strategy 4: first { to last }
	first := strings.Index(resp, "{")
	last := strings.LastIndex(resp, "}")
	if first >= 0 && last > first {
		if err := json.Unmarshal([]byte(resp[first:last+1]), out); err == nil {
			return nil
		}
	}

	return errors.New("could not extract valid JSON from LLM response")
}

// isRetryable returns true for errors that should be retried.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Check for structured LLMError with status code.
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr.StatusCode == 429 ||
			(llmErr.StatusCode >= 500 && llmErr.StatusCode <= 599)
	}
	// Check for timeout errors.
	if isTimeout(err) {
		return true
	}
	// Fallback: check for known retryable strings (e.g., from non-HTTP transports).
	msg := err.Error()
	return strings.Contains(msg, "unavailable")
}

// isTimeout checks if an error represents a timeout.
func isTimeout(err error) bool {
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

func isRateLimit(err error) bool {
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

// retryCallResult holds the result of callWithRetry.
type retryCallResult struct {
	content    string
	tokensUsed int
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
		if attempt > 0 && lastErr != nil && isTimeout(lastErr) {
			timeout = 60 * time.Second
		}

		resp, tokens, err := c.callLLM(ctx, prompt, timeout)
		totalTokens += tokens
		if err == nil {
			return &retryCallResult{content: resp, tokensUsed: totalTokens}, nil
		}

		lastErr = err

		// Update max retries for rate limits.
		if isRateLimit(err) && maxRetries < 5 {
			maxRetries = 5
		}

		if !isRetryable(err) {
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
	// Fallback: use basic LLMClient with context timeout.
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := c.llm.Complete(callCtx, prompt)
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

// Circuit breaker methods

func (c *stage3Critic) checkCircuit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.consecutiveFail < 5 {
		return nil
	}

	elapsed := c.clock().Sub(c.circuitOpenAt)
	if elapsed < c.openDuration {
		return ErrCircuitOpen
	}

	// Half-open: allow one probe
	return nil
}

func (c *stage3Critic) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.consecutiveFail++
	if c.consecutiveFail == 5 {
		c.circuitOpenAt = c.clock()
		c.openDuration = 5 * time.Minute
		c.log.Warn("stage3 circuit breaker opened")
	} else if c.consecutiveFail > 5 {
		// Failed during half-open probe → re-open with doubled duration
		c.circuitOpenAt = c.clock()
		c.openDuration = c.openDuration * 2
		c.log.Warn("stage3 circuit breaker re-opened", "duration", c.openDuration)
	}
}

func (c *stage3Critic) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.consecutiveFail = 0
}

func truncateContent(content []byte, maxBytes int) []byte {
	if len(content) <= maxBytes {
		return content
	}
	// Don't cut mid-rune — back off incomplete trailing bytes (at most 3).
	truncated := content[:maxBytes]
	for i := 0; i < 3 && len(truncated) > 0; i++ {
		r, _ := utf8.DecodeLastRune(truncated)
		if r != utf8.RuneError {
			break
		}
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

// generateNonce returns a short random hex string for delimiter uniqueness.
func generateNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sanitizeDelimiters replaces any occurrence of the delimiter markers in content.
func sanitizeDelimiters(content []byte, beginDelim, endDelim string) []byte {
	s := string(content)
	s = strings.ReplaceAll(s, beginDelim, "[SANITIZED_DELIMITER]")
	s = strings.ReplaceAll(s, endDelim, "[SANITIZED_DELIMITER]")
	return []byte(s)
}

func buildCriticPrompt(content []byte, stage2 *Stage2Result) string {
	nonce := generateNonce()
	beginDelim := fmt.Sprintf("---BEGIN SESSION %s---", nonce)
	endDelim := fmt.Sprintf("---END SESSION %s---", nonce)

	stage2JSON, err := json.Marshal(stage2)
	if err != nil {
		stage2JSON = []byte("{}")
	}

	sanitized := sanitizeDelimiters(content, beginDelim, endDelim)

	return fmt.Sprintf(`You are a critical evaluator for an AI skill extraction system. Your job is to decide whether this session should be extracted as a reusable skill.

Review the session content and the Stage 2 scoring results, then provide your independent verdict.

Respond with ONLY a JSON object (no markdown, no explanation outside JSON):
{
  "verdict": "extract" or "reject",
  "reasoning": "Your reasoning for the verdict",
  "refined_scores": {
    "problem_specificity": 1-5,
    "solution_completeness": 1-5,
    "context_portability": 1-5,
    "reasoning_transparency": 1-5,
    "technical_accuracy": 1-5,
    "verification_evidence": 1-5,
    "innovation_level": 1-5
  },
  "confidence": 0.0-1.0,
  "reusable_pattern": true/false,
  "explicit_reasoning": true/false,
  "contradicts_best_practices": true/false
}

Stage 2 results:
%s

%s
%s
%s`, string(stage2JSON), beginDelim, string(sanitized), endDelim)
}
