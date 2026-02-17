package serverclient

import (
	"context"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// HTTPSkillStore implements a read-only subset of model.SkillStore via HTTP.
type HTTPSkillStore struct {
	client *Client
}

// NewHTTPSkillStore creates an HTTPSkillStore.
func NewHTTPSkillStore(client *Client) *HTTPSkillStore {
	return &HTTPSkillStore{client: client}
}

type searchRequest struct {
	Patterns   []string  `json:"patterns,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	LibraryIDs []string  `json:"library_ids,omitempty"`
	MinQuality float64   `json:"min_quality,omitempty"`
	MinDecay   float64   `json:"min_decay,omitempty"`
	Limit      int       `json:"limit,omitempty"`
}

type searchResponse struct {
	Results []model.ScoredSkill `json:"results"`
}

// Query searches for skills via POST /api/v1/search.
func (s *HTTPSkillStore) Query(ctx context.Context, q model.SkillQuery) ([]model.ScoredSkill, error) {
	req := searchRequest{
		Patterns:   q.Patterns,
		Embedding:  q.Embedding,
		LibraryIDs: q.LibraryIDs,
		MinQuality: q.MinQuality,
		MinDecay:   q.MinDecay,
		Limit:      q.Limit,
	}
	var resp searchResponse
	if err := s.client.doJSON(ctx, "POST", "/api/v1/search", req, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}
