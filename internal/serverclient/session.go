package serverclient

import (
	"context"
	"fmt"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// HTTPSessionWriter implements session CRUD via the server HTTP API.
type HTTPSessionWriter struct {
	client *Client
}

// NewHTTPSessionWriter creates an HTTPSessionWriter.
func NewHTTPSessionWriter(client *Client) *HTTPSessionWriter {
	return &HTTPSessionWriter{client: client}
}

type createSessionRequest struct {
	Session          model.SessionRecord `json:"session"`
	ExtractionResult *updateSessionBody  `json:"extraction_result,omitempty"`
}

type updateSessionBody struct {
	ExtractionStatus string `json:"extraction_status"`
	RejectedAtStage  int    `json:"rejected_at_stage"`
	RejectionReason  string `json:"rejection_reason"`
	ExtractedSkillID string `json:"extracted_skill_id"`
}

// InsertSession creates a new session record on the server.
func (w *HTTPSessionWriter) InsertSession(ctx context.Context, session *model.SessionRecord) error {
	var resp map[string]string
	return w.client.doJSON(ctx, "POST", "/api/v1/sessions", createSessionRequest{Session: *session}, &resp)
}

// UpdateSessionResult updates the extraction result for a session.
func (w *HTTPSessionWriter) UpdateSessionResult(ctx context.Context, id string, result *model.ExtractionResult) error {
	var skillID string
	if result.Skill != nil {
		skillID = result.Skill.ID
	}
	body := updateSessionBody{
		ExtractionStatus: string(result.Status),
		ExtractedSkillID: skillID,
	}
	if result.Error != "" {
		body.RejectionReason = result.Error
	}
	var resp map[string]string
	return w.client.doJSON(ctx, "PUT", fmt.Sprintf("/api/v1/sessions/%s", id), body, &resp)
}

// GetSession retrieves a session by ID.
// TODO: The server does not yet expose GET /api/v1/sessions/{id}. Add endpoint in T4 follow-up.
func (w *HTTPSessionWriter) GetSession(ctx context.Context, id string) (*model.SessionRecord, error) {
	var session model.SessionRecord
	if err := w.client.doJSON(ctx, "GET", fmt.Sprintf("/api/v1/sessions/%s", id), nil, &session); err != nil {
		return nil, err
	}
	return &session, nil
}
