package extraction

import (
	"context"
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
	llm LLMClient
	cfg config.ExtractionConfig
	log *slog.Logger

	// Circuit breaker state
	mu              sync.Mutex
	consecutiveFail int
	circuitOpenAt   time.Time
	openDuration    time.Duration
	failedInHalfOpen bool

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
	return &stage3Critic{
		llm:          llm,
		cfg:          cfg,
		log:          log,
		openDuration: 5 * time.Minute,
		clock:        time.Now,
		sleep:        time.Sleep,
	}
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
	resp, err := c.callWithRetry(ctx, prompt, session.ID)
	if err != nil {
		c.recordFailure()
		elapsed := c.clock().Sub(start).Milliseconds()
		c.log.Error("stage3 LLM call failed", "session_id", session.ID, "error", err, "latency_ms", elapsed)
		return nil, fmt.Errorf("stage3: llm call: %w", err)
	}

	c.recordSuccess()

	// Parse response
	result, err := c.parseAndValidate(resp)
	if err != nil {
		elapsed := c.clock().Sub(start).Milliseconds()
		c.log.Warn("stage3 parse error, treating as rejection", "session_id", session.ID, "error", err)
		return &Stage3Result{
			Passed:        false,
			CriticVerdict: "reject",
			CriticReasoning: fmt.Sprintf("critic_parse_error: %v", err),
			LatencyMs:     elapsed,
		}, nil
	}

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

	// Contradiction detection: extract but all scores are 1
	if verdict == "extract" && allScoresAre(scores, 1) {
		c.log.Warn("stage3 contradiction: verdict=extract but all scores=1, overriding to reject")
		verdict = "reject"
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

func allScoresAre(q QualityScores, val int) bool {
	return q.ProblemSpecificity == val &&
		q.SolutionCompleteness == val &&
		q.ContextPortability == val &&
		q.ReasoningTransparency == val &&
		q.TechnicalAccuracy == val &&
		q.VerificationEvidence == val &&
		q.InnovationLevel == val
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
	msg := err.Error()
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "5") || // 5xx
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "unavailable")
}

func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "429") || strings.Contains(msg, "rate limit")
}

func (c *stage3Critic) callWithRetry(ctx context.Context, prompt string, sessionID string) (string, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			backoff := time.Second * time.Duration(1<<(attempt-1)) // 1s, 2s, 4s
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			c.log.Info("stage3 retry", "session_id", sessionID, "attempt", attempt, "backoff", backoff)
			c.sleep(backoff)
		}

		resp, err := c.llm.Complete(ctx, prompt)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Rate limit: allow up to 5 retries
		if isRateLimit(err) && attempt == maxRetries && maxRetries < 5 {
			maxRetries = 5
		}

		if !isRetryable(err) {
			return "", err
		}
	}

	return "", lastErr
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
	// Don't cut mid-rune
	truncated := content[:maxBytes]
	for len(truncated) > 0 && !utf8.Valid(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

func buildCriticPrompt(content []byte, stage2 *Stage2Result) string {
	stage2JSON, _ := json.Marshal(stage2)
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

---BEGIN SESSION---
%s
---END SESSION---`, string(stage2JSON), string(content))
}
