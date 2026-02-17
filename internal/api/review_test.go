package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
)

func TestCreateReviewSample(t *testing.T) {
	store := newTestStore(t)
	// Seed a session for FK constraint
	ctx := context.Background()
	_ = store.InsertSession(ctx, &model.SessionRecord{
		ID:               "sess-1",
		ExtractionStatus: model.StatusPending,
	})
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(CreateReviewSampleRequest{
		SessionID:  "sess-1",
		ResultJSON: json.RawMessage(`{"quality": 4}`),
	})
	req := httptest.NewRequest("POST", "/api/v1/review-samples", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateReviewSample_MissingFields(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(CreateReviewSampleRequest{})
	req := httptest.NewRequest("POST", "/api/v1/review-samples", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
