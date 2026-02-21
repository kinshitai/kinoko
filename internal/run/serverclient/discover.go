package serverclient

import (
	"context"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// HTTPSkillStore implements a read-only subset of model.SkillStore via HTTP.
type HTTPSkillStore struct {
	client *Client
}

// NewHTTPSkillStore creates an HTTPSkillStore.
func NewHTTPSkillStore(client *Client) *HTTPSkillStore {
	return &HTTPSkillStore{client: client}
}

// DiscoverClient performs unified discovery queries via POST /api/v1/discover.
type DiscoverClient struct {
	client *Client
}

// NewDiscoverClient creates a DiscoverClient.
func NewDiscoverClient(client *Client) *DiscoverClient {
	return &DiscoverClient{client: client}
}

type discoverRequest struct {
	Prompt     string    `json:"prompt,omitempty"`
	Embedding  []float64 `json:"embedding,omitempty"` // Note: API uses float64, server converts internally
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

// Discover performs a unified query via POST /api/v1/discover.
func (d *DiscoverClient) Discover(ctx context.Context, req discoverRequest) (*discoverResponse, error) {
	var resp discoverResponse
	if err := d.client.doJSON(ctx, "POST", "/api/v1/discover", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Query searches for skills via POST /api/v1/discover (backwards compatibility).
func (s *HTTPSkillStore) Query(ctx context.Context, q model.SkillQuery) ([]model.ScoredSkill, error) {
	// Convert float32 embedding to float64 for API
	var embedding []float64
	if len(q.Embedding) > 0 {
		embedding = make([]float64, len(q.Embedding))
		for i, v := range q.Embedding {
			embedding[i] = float64(v)
		}
	}

	req := discoverRequest{
		Embedding:  embedding,
		Patterns:   q.Patterns,
		LibraryIDs: q.LibraryIDs,
		MinQuality: q.MinQuality,
		TopK:       q.Limit,
	}

	var resp discoverResponse
	if err := s.client.doJSON(ctx, "POST", "/api/v1/discover", req, &resp); err != nil {
		return nil, err
	}

	// Convert discover response to ScoredSkill format
	results := make([]model.ScoredSkill, 0, len(resp.Skills))
	for _, skill := range resp.Skills {
		// Parse library and name from repo field
		libName := skill.Repo // format: "library/name"
		var libraryID, name string
		if idx := len(libName) - 1; idx > 0 {
			for i := len(libName) - 1; i >= 0; i-- {
				if libName[i] == '/' {
					libraryID = libName[:i]
					name = libName[i+1:]
					break
				}
			}
		}
		if name == "" {
			name = skill.Name
		}

		results = append(results, model.ScoredSkill{
			Skill: model.SkillRecord{
				Name:      name,
				LibraryID: libraryID,
			},
			CompositeScore: skill.Score,
		})
	}

	return results, nil
}
