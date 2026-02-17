package serverclient

import (
	"context"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// HTTPQuerier implements model.SkillQuerier via the server HTTP API.
// It uses POST /api/v1/discover with an embedding vector to find the nearest skill.
type HTTPQuerier struct {
	client *Client
}

// NewHTTPQuerier creates an HTTPQuerier.
func NewHTTPQuerier(client *Client) *HTTPQuerier {
	return &HTTPQuerier{client: client}
}

// QueryNearest finds the nearest skill by embedding similarity via POST /api/v1/discover.
func (q *HTTPQuerier) QueryNearest(ctx context.Context, embedding []float32, libraryID string) (*model.SkillQueryResult, error) {
	// Convert float32 to float64 for API
	embedding64 := make([]float64, len(embedding))
	for i, v := range embedding {
		embedding64[i] = float64(v)
	}

	req := discoverRequest{
		Embedding:  embedding64,
		LibraryIDs: []string{libraryID},
		TopK:       1,
	}

	var resp discoverResponse
	if err := q.client.doJSON(ctx, "POST", "/api/v1/discover", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Skills) == 0 {
		return &model.SkillQueryResult{}, nil
	}
	best := resp.Skills[0]
	return &model.SkillQueryResult{
		CosineSim: best.Score, // Using composite score as similarity
		SkillName: best.Name,
	}, nil
}
