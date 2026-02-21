package serverclient

import (
	"context"
	"fmt"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// HTTPDecayClient implements decay.SkillReader and decay.SkillWriter via HTTP.
type HTTPDecayClient struct {
	client *Client
}

// NewHTTPDecayClient creates a new HTTPDecayClient.
func NewHTTPDecayClient(client *Client) *HTTPDecayClient {
	return &HTTPDecayClient{client: client}
}

type decayListResponse struct {
	Skills []model.SkillRecord `json:"skills"`
}

type updateDecayRequest struct {
	DecayScore float64 `json:"decay_score"`
}

// ListByDecay returns skills ordered by decay score.
func (d *HTTPDecayClient) ListByDecay(ctx context.Context, libraryID string, limit int) ([]model.SkillRecord, error) {
	path := fmt.Sprintf("/api/v1/skills/decay?library_id=%s&limit=%d", libraryID, limit)
	var resp decayListResponse
	if err := d.client.doJSON(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Skills, nil
}

// UpdateDecay updates the decay score for a skill.
func (d *HTTPDecayClient) UpdateDecay(ctx context.Context, id string, decayScore float64) error {
	path := fmt.Sprintf("/api/v1/skills/%s/decay", id)
	return d.client.doJSON(ctx, "PATCH", path, updateDecayRequest{DecayScore: decayScore}, nil)
}
