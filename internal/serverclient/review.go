package serverclient

import (
	"context"
	"encoding/json"
)

// HTTPReviewer writes review samples to the server.
type HTTPReviewer struct {
	client *Client
}

// NewHTTPReviewer creates an HTTPReviewer.
func NewHTTPReviewer(client *Client) *HTTPReviewer {
	return &HTTPReviewer{client: client}
}

type createReviewSampleRequest struct {
	SessionID  string          `json:"session_id"`
	ResultJSON json.RawMessage `json:"result_json"`
}

// InsertReviewSample submits a review sample to the server.
func (r *HTTPReviewer) InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error {
	var resp map[string]string
	return r.client.doJSON(ctx, "POST", "/api/v1/review-samples", createReviewSampleRequest{
		SessionID:  sessionID,
		ResultJSON: json.RawMessage(resultJSON),
	}, &resp)
}
