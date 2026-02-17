package serverclient

import (
	"context"
	"fmt"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// HTTPInjectionEventWriter implements injection.InjectionEventWriter via HTTP.
type HTTPInjectionEventWriter struct {
	client *Client
}

// NewHTTPInjectionEventWriter creates a new HTTPInjectionEventWriter.
func NewHTTPInjectionEventWriter(client *Client) *HTTPInjectionEventWriter {
	return &HTTPInjectionEventWriter{client: client}
}

// WriteInjectionEvent posts an injection event to the server.
func (w *HTTPInjectionEventWriter) WriteInjectionEvent(ctx context.Context, ev model.InjectionEventRecord) error {
	return w.client.doJSON(ctx, "POST", "/api/v1/injection-events", ev, nil)
}

type updateOutcomeRequest struct {
	Outcome string `json:"outcome"`
}

// UpdateInjectionOutcome updates the outcome for a session's injection events.
func (w *HTTPInjectionEventWriter) UpdateInjectionOutcome(ctx context.Context, sessionID string, outcome string) error {
	path := fmt.Sprintf("/api/v1/injection-events/%s/outcome", sessionID)
	return w.client.doJSON(ctx, "PUT", path, updateOutcomeRequest{Outcome: outcome}, nil)
}
