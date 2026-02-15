// Package embedding provides vector embedding computation via OpenAI-compatible
// APIs. It includes retry logic with exponential backoff and a circuit breaker
// to protect against cascading failures from upstream providers.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"
)

// Embedder computes vector embeddings for text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// Compile-time interface check.
var _ Embedder = (*Client)(nil)

// Config configures the embedding service.
type Config struct {
	Provider       string               `yaml:"provider"`
	Model          string               `yaml:"model"`
	Dims           int                  `yaml:"dimensions"`
	BaseURL        string               `yaml:"base_url"`
	APIKey         string               `yaml:"api_key"`
	Retry          RetryConfig          `yaml:"retry"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
}

// RetryConfig controls retry with exponential backoff.
type RetryConfig struct {
	MaxRetries     int           `yaml:"max_retries"`
	InitialBackoff time.Duration `yaml:"initial_backoff"`
	MaxBackoff     time.Duration `yaml:"max_backoff"`
	BackoffFactor  float64       `yaml:"backoff_factor"`
}

// CircuitBreakerConfig controls the circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	OpenDuration     time.Duration `yaml:"open_duration"`
	HalfOpenMax      int           `yaml:"half_open_max"`
}

// DefaultConfig returns a Config with spec defaults.
func DefaultConfig() Config {
	return Config{
		Provider: "openai",
		Model:    "text-embedding-3-small",
		Dims:     1536,
		BaseURL:  "https://api.openai.com",
		Retry: RetryConfig{
			MaxRetries:     3,
			InitialBackoff: time.Second,
			MaxBackoff:     30 * time.Second,
			BackoffFactor:  2.0,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 5,
			OpenDuration:     5 * time.Minute,
			HalfOpenMax:      1,
		},
	}
}

// circuitState represents the circuit breaker state.
type circuitState int

const (
	circuitClosed   circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// maxOpenDuration caps the escalating open duration.
const maxOpenDuration = 30 * time.Minute

// maxResponseBody caps response body reads (10 MB).
const maxResponseBody = 10 << 20

// maxErrorBodyLog caps body bytes included in error messages.
const maxErrorBodyLog = 512

func (s circuitState) String() string {
	switch s {
	case circuitClosed:
		return "closed"
	case circuitOpen:
		return "open"
	case circuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// permanentError wraps errors that should not be retried (4xx except 429).
type permanentError struct {
	err error
}

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// IsPermanent reports whether err is a non-retryable error.
func IsPermanent(err error) bool {
	var pe *permanentError
	return errors.As(err, &pe)
}

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Client is the OpenAI-compatible embedding client.
type Client struct {
	cfg    Config
	http   *http.Client
	logger *slog.Logger

	mu                 sync.Mutex
	cbState            circuitState
	cbFailures         int
	cbOpenedAt         time.Time
	cbHalfOpenInFlight int
	cbCurrentOpenDur   time.Duration // current open duration (escalates on half-open failure)
}

// New creates a new embedding Client.
func New(cfg Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:              cfg,
		http:             &http.Client{Timeout: 30 * time.Second},
		logger:           logger.With("component", "embedding"),
		cbCurrentOpenDur: cfg.CircuitBreaker.OpenDuration,
	}
}

// Dimensions returns the configured embedding dimensions.
func (c *Client) Dimensions() int {
	return c.cfg.Dims
}

// Embed computes an embedding for a single text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// EmbedBatch computes embeddings for multiple texts in one API call.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	var result [][]float32
	var lastErr error

	backoff := c.cfg.Retry.InitialBackoff
	attempts := 1 + c.cfg.Retry.MaxRetries

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			c.logger.Info("retrying embedding request",
				"attempt", attempt+1,
				"backoff", backoff,
				"error", lastErr,
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff = time.Duration(math.Min(
				float64(backoff)*c.cfg.Retry.BackoffFactor,
				float64(c.cfg.Retry.MaxBackoff),
			))
		}

		if err := c.cbAllow(); err != nil {
			lastErr = err
			continue
		}

		result, lastErr = c.doRequest(ctx, texts)
		if lastErr != nil {
			// Permanent errors (4xx except 429): don't retry, don't trip breaker.
			if IsPermanent(lastErr) {
				return nil, lastErr
			}
			c.cbRecordFailure()
			continue
		}

		c.cbRecordSuccess()
		return result, nil
	}

	return nil, fmt.Errorf("embedding request failed after %d attempts: %w", attempts, lastErr)
}

// --- OpenAI API types ---

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *apiError       `json:"error,omitempty"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// truncateBody returns at most maxErrorBodyLog bytes of body for error messages.
func truncateBody(body []byte) string {
	if len(body) <= maxErrorBodyLog {
		return string(body)
	}
	return string(body[:maxErrorBodyLog]) + "...(truncated)"
}

func (c *Client) doRequest(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody, err := json.Marshal(embeddingRequest{
		Input: texts,
		Model: c.cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.cfg.BaseURL + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodySnippet := truncateBody(body)
		c.logger.Error("embedding API error",
			"status", resp.StatusCode,
			"body", bodySnippet,
		)
		apiErr := fmt.Errorf("embedding API returned status %d: %s", resp.StatusCode, bodySnippet)

		// 4xx (except 429) are permanent — don't retry.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, &permanentError{err: apiErr}
		}
		return nil, apiErr
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("embedding API error: %s", embResp.Error.Message)
	}

	if len(embResp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embResp.Data))
	}

	// Sort by index to match input order; validate dimensions.
	results := make([][]float32, len(texts))
	for _, d := range embResp.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("invalid embedding index %d", d.Index)
		}
		if c.cfg.Dims > 0 && len(d.Embedding) != c.cfg.Dims {
			return nil, fmt.Errorf("expected %d dimensions, got %d for index %d", c.cfg.Dims, len(d.Embedding), d.Index)
		}
		results[d.Index] = d.Embedding
	}

	return results, nil
}

// --- Circuit breaker ---

func (c *Client) cbAllow() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.cbState {
	case circuitClosed:
		return nil
	case circuitOpen:
		if time.Since(c.cbOpenedAt) >= c.cbCurrentOpenDur {
			c.cbState = circuitHalfOpen
			c.cbHalfOpenInFlight = 0
			c.logger.Info("circuit breaker transition", "from", "open", "to", "half-open")
			c.cbHalfOpenInFlight++
			return nil
		}
		return ErrCircuitOpen
	case circuitHalfOpen:
		if c.cbHalfOpenInFlight < c.cfg.CircuitBreaker.HalfOpenMax {
			c.cbHalfOpenInFlight++
			return nil
		}
		return ErrCircuitOpen
	}
	return nil
}

func (c *Client) cbRecordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cbState == circuitHalfOpen {
		c.logger.Info("circuit breaker transition", "from", "half-open", "to", "closed")
	}
	c.cbState = circuitClosed
	c.cbFailures = 0
	c.cbHalfOpenInFlight = 0
	c.cbCurrentOpenDur = c.cfg.CircuitBreaker.OpenDuration // reset to base
}

func (c *Client) cbRecordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cbFailures++
	c.logger.Warn("embedding request failed",
		"consecutive_failures", c.cbFailures,
		"circuit_state", c.cbState.String(),
	)

	switch c.cbState {
	case circuitClosed:
		if c.cbFailures >= c.cfg.CircuitBreaker.FailureThreshold {
			c.cbState = circuitOpen
			c.cbOpenedAt = time.Now()
			c.cbCurrentOpenDur = c.cfg.CircuitBreaker.OpenDuration // base duration
			c.logger.Warn("circuit breaker transition",
				"from", "closed",
				"to", "open",
				"open_duration", c.cbCurrentOpenDur,
			)
		}
	case circuitHalfOpen:
		// Escalate: double the open duration, capped at max.
		nextDur := c.cbCurrentOpenDur * 2
		if nextDur > maxOpenDuration {
			nextDur = maxOpenDuration
		}
		c.cbCurrentOpenDur = nextDur
		c.cbState = circuitOpen
		c.cbOpenedAt = time.Now()
		c.logger.Warn("circuit breaker transition",
			"from", "half-open",
			"to", "open",
			"open_duration", c.cbCurrentOpenDur,
		)
	}
}
