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

// WriteReviewSample submits a review sample to the server.
func (r *HTTPReviewer) WriteReviewSample(ctx context.Context, sessionID string, resultJSON json.RawMessage) error {
	var resp map[string]string
	return r.client.doJSON(ctx, "POST", "/api/v1/review-samples", createReviewSampleRequest{
		SessionID:  sessionID,
		ResultJSON: resultJSON,
	}, &resp)
}
