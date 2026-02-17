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
	threshold  float64
	httpClient *http.Client
	log        *slog.Logger
}

// NewNoveltyClient creates a NoveltyClient with a 10s timeout.
func NewNoveltyClient(apiURL string, threshold float64, log *slog.Logger) *NoveltyClient {
	return &NoveltyClient{
		apiURL:    apiURL,
		threshold: threshold,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: log,
	}
}

// discoverRequest is the POST body for the discover endpoint (used for novelty checking).
type discoverRequest struct {
	Prompt     string    `json:"prompt,omitempty"`
	Embedding  []float64 `json:"embedding,omitempty"`
	Patterns   []string  `json:"patterns,omitempty"`
	LibraryIDs []string  `json:"library_ids,omitempty"`
	MinQuality float64   `json:"min_quality,omitempty"`
	TopK       int       `json:"top_k,omitempty"`
}

type discoverResponse struct {
	Skills []struct {
		Repo        string  `json:"repo"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Score       float64 `json:"score"`
		CloneURL    string  `json:"clone_url"`
	} `json:"skills"`
}

// Check uses the discover API to check content novelty and performs the novelty check locally.
// If the server is unreachable, it returns novel=true (fail-open) and logs a warning.
func (c *NoveltyClient) Check(ctx context.Context, content string) (*NoveltyResult, error) {
	// Use discover endpoint to find similar skills
	body, err := json.Marshal(discoverRequest{
		Prompt: content, // Search for skills similar to this content
		TopK:   10,      // Get top 10 similar skills for comparison
	})
	if err != nil {
		return nil, fmt.Errorf("novelty: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/v1/discover", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("novelty: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Fail-open: treat as novel when server is unreachable.
		c.log.Warn("novelty server unreachable, treating as novel", "error", err)
		return &NoveltyResult{Novel: true, Score: 0}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fail-open on server errors too.
		c.log.Warn("novelty server returned non-200, treating as novel", "status", resp.StatusCode)
		return &NoveltyResult{Novel: true, Score: 0}, nil
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("novelty: read response: %w", err)
	}

	var discoverResp discoverResponse
	if err := json.Unmarshal(respBody, &discoverResp); err != nil {
		return nil, fmt.Errorf("novelty: unmarshal response: %w", err)
	}

	// Perform novelty check locally: if any skill has score > threshold, it's not novel
	var maxScore float64 = 0
	var similar []SimilarSkill
	
	for _, skill := range discoverResp.Skills {
		if skill.Score > maxScore {
			maxScore = skill.Score
		}
		similar = append(similar, SimilarSkill{
			Name:  skill.Name,
			Score: skill.Score,
		})
	}

	// Content is novel if highest similarity score is below threshold
	novel := maxScore < c.threshold

	return &NoveltyResult{
		Novel:   novel,
		Score:   maxScore,
		Similar: similar,
	}, nil
}
