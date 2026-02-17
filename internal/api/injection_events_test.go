package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

func seedSessionAndSkill(t *testing.T, store interface {
	InsertSession(ctx context.Context, session *model.SessionRecord) error
	Put(ctx context.Context, skill *model.SkillRecord, body []byte) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := store.InsertSession(ctx, &model.SessionRecord{
		ID:               "sess-1",
		ExtractionStatus: model.StatusPending,
		LibraryID:        "lib1",
	}); err != nil {
		t.Fatalf("seed session-1: %v", err)
	}
	if err := store.InsertSession(ctx, &model.SessionRecord{
		ID:               "sess-2",
		ExtractionStatus: model.StatusPending,
		LibraryID:        "lib1",
	}); err != nil {
		t.Fatalf("seed session-2: %v", err)
	}
	if err := store.Put(ctx, &model.SkillRecord{
		ID:        "skill-1",
		Name:      "test-skill",
		LibraryID: "lib1",
		Category:  "tactical",
	}, nil); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
}

func TestCreateInjectionEvent(t *testing.T) {
	store := newTestStore(t)
	seedSessionAndSkill(t, store)
	srv := New(Config{Port: 0, Store: store})

	ev := model.InjectionEventRecord{
		ID:         "ie-1",
		SessionID:  "sess-1",
		SkillID:    "skill-1",
		InjectedAt: time.Now().UTC(),
	}
	body, _ := json.Marshal(ev)
	req := httptest.NewRequest("POST", "/api/v1/injection-events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateInjectionEvent_MissingFields(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(model.InjectionEventRecord{})
	req := httptest.NewRequest("POST", "/api/v1/injection-events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateInjectionOutcome(t *testing.T) {
	store := newTestStore(t)
	seedSessionAndSkill(t, store)
	srv := New(Config{Port: 0, Store: store})

	// Insert an event first
	ctx := context.Background()
	ev := model.InjectionEventRecord{
		ID:         "ie-2",
		SessionID:  "sess-2",
		SkillID:    "skill-1",
		InjectedAt: time.Now().UTC(),
	}
	if err := store.WriteInjectionEvent(ctx, ev); err != nil {
		t.Fatalf("seed injection event: %v", err)
	}

	body, _ := json.Marshal(UpdateInjectionOutcomeRequest{Outcome: "success"})
	req := httptest.NewRequest("PUT", "/api/v1/injection-events/sess-2/outcome", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateInjectionOutcome_MissingOutcome(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(UpdateInjectionOutcomeRequest{})
	req := httptest.NewRequest("PUT", "/api/v1/injection-events/sess-1/outcome", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
