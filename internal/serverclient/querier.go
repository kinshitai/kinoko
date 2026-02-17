package serverclient

import (
	"context"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// HTTPQuerier implements model.SkillQuerier via the server HTTP API.
// It uses POST /api/v1/search with an embedding vector to find the nearest skill.
// TODO: Once the server exposes a dedicated embedding-based novelty endpoint,
// switch to POST /api/v1/novelty with raw embeddings.
type HTTPQuerier struct {
	client *Client
}

// NewHTTPQuerier creates an HTTPQuerier.
func NewHTTPQuerier(client *Client) *HTTPQuerier {
	return &HTTPQuerier{client: client}
}

// QueryNearest finds the nearest skill by embedding similarity.
func (q *HTTPQuerier) QueryNearest(ctx context.Context, embedding []float32, libraryID string) (*model.SkillQueryResult, error) {
	req := searchRequest{
		Embedding:  embedding,
		LibraryIDs: []string{libraryID},
		Limit:      1,
	}
	var resp searchResponse
	if err := q.client.doJSON(ctx, "POST", "/api/v1/search", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return &model.SkillQueryResult{}, nil
	}
	best := resp.Results[0]
	return &model.SkillQueryResult{
		CosineSim: best.CosineSim,
		SkillName: best.Skill.Name,
	}, nil
}
