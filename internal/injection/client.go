// Package injection provides the skill injection client and prompt builder.
package injection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// MatchedSkill is a single skill returned by the match API.
type MatchedSkill struct {
	Name    string  `json:"name"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}

// MatchResult is the response from the match API.
type MatchResult struct {
	Skills []MatchedSkill `json:"skills"`
}

// Client calls the Kinoko match API.
type Client struct {
	apiURL     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new injection client.
func NewClient(apiURL string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

type matchRequest struct {
	Context  string  `json:"context"`
	Limit    int     `json:"limit"`
	MinScore float64 `json:"min_score"`
}

// Match queries the match API for skills relevant to the session context.
// Fails open: returns empty result on error.
func (c *Client) Match(ctx context.Context, sessionContext string, limit int) (*MatchResult, error) {
	return c.MatchWithMinScore(ctx, sessionContext, limit, 0.5)
}

// MatchWithMinScore queries the match API with a configurable minimum score threshold.
// Fails open: returns empty result on error.
func (c *Client) MatchWithMinScore(ctx context.Context, sessionContext string, limit int, minScore float64) (*MatchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if minScore <= 0 {
		minScore = 0.5
	}

	body, err := json.Marshal(matchRequest{
		Context:  sessionContext,
		Limit:    limit,
		MinScore: minScore,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.apiURL + "/api/v1/match"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Fail open: log warning and return empty result.
		c.logger.Warn("match API unreachable", "url", url, "error", err)
		return &MatchResult{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("match API error", "status", resp.StatusCode)
		return &MatchResult{}, nil
	}

	var result MatchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.logger.Warn("match API decode failed", "error", err)
		return &MatchResult{}, nil
	}

	return &result, nil
}
