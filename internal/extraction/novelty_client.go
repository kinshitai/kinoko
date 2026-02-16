package extraction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// NoveltyResult holds the server response for a novelty check.
type NoveltyResult struct {
	Novel   bool           `json:"novel"`
	Score   float64        `json:"score"`
	Similar []SimilarSkill `json:"similar"`
}

// SimilarSkill describes a skill that is similar to the checked content.
type SimilarSkill struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// NoveltyClient checks content novelty against the server API.
type NoveltyClient struct {
	apiURL     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewNoveltyClient creates a NoveltyClient with a 10s timeout.
func NewNoveltyClient(apiURL string, logger *slog.Logger) *NoveltyClient {
	return &NoveltyClient{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// noveltyRequest is the POST body for the novelty endpoint.
type noveltyRequest struct {
	Content string `json:"content"`
}

// Check posts content to the novelty API and returns the result.
// If the server is unreachable, it returns novel=true (fail-open) and logs a warning.
func (c *NoveltyClient) Check(ctx context.Context, content string) (*NoveltyResult, error) {
	body, err := json.Marshal(noveltyRequest{Content: content})
	if err != nil {
		return nil, fmt.Errorf("novelty: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/v1/novelty", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("novelty: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Fail-open: treat as novel when server is unreachable.
		c.logger.Warn("novelty server unreachable, treating as novel", "error", err)
		return &NoveltyResult{Novel: true, Score: 0}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fail-open on server errors too.
		c.logger.Warn("novelty server returned non-200, treating as novel", "status", resp.StatusCode)
		return &NoveltyResult{Novel: true, Score: 0}, nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("novelty: read response: %w", err)
	}

	var result NoveltyResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("novelty: unmarshal response: %w", err)
	}

	return &result, nil
}
