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

func TestCreateSession(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(CreateSessionRequest{
		Session: model.SessionRecord{ID: "sess-1", LibraryID: "lib1"},
	})
	req := httptest.NewRequest("POST", "/api/v1/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateSession_MissingID(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(CreateSessionRequest{})
	req := httptest.NewRequest("POST", "/api/v1/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateSession_WithExtractionResult(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(CreateSessionRequest{
		Session: model.SessionRecord{ID: "sess-2", LibraryID: "lib1"},
		ExtractionResult: &UpdateSessionBody{
			ExtractionStatus: "rejected",
			RejectedAtStage:  2,
			RejectionReason:  "low quality",
		},
	})
	req := httptest.NewRequest("POST", "/api/v1/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSession(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	// First create
	sess := &model.SessionRecord{ID: "sess-u1", ExtractionStatus: model.StatusPending}
	if err := store.InsertSession(context.Background(), sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	body, _ := json.Marshal(UpdateSessionBody{
		ExtractionStatus: "rejected",
		RejectionReason:  "low quality",
	})
	req := httptest.NewRequest("PUT", "/api/v1/sessions/sess-u1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSession_NotFound(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(UpdateSessionBody{
		ExtractionStatus: "rejected",
		RejectionReason:  "low quality",
	})
	req := httptest.NewRequest("PUT", "/api/v1/sessions/nonexistent-id", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
