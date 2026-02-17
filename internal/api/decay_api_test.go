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

func TestListByDecay(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	req := httptest.NewRequest("GET", "/api/v1/skills/decay?limit=10", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DecayListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Skills == nil {
		t.Fatal("skills should be non-nil")
	}
}

func TestUpdateDecay(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	// Seed a skill first
	ctx := context.Background()
	if err := store.Put(ctx, &model.SkillRecord{
		ID: "skill-1", Name: "test", LibraryID: "lib1", Category: "tactical",
	}, nil); err != nil {
		t.Fatalf("seed skill: %v", err)
	}

	body, _ := json.Marshal(UpdateDecayRequest{DecayScore: 0.75})
	req := httptest.NewRequest("PATCH", "/api/v1/skills/skill-1/decay", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateDecay_NotFound(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(UpdateDecayRequest{DecayScore: 0.5})
	req := httptest.NewRequest("PATCH", "/api/v1/skills/nonexistent/decay", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateDecay_OutOfRange(t *testing.T) {
	srv := New(Config{Port: 0})

	// Test > 1.0
	body, _ := json.Marshal(UpdateDecayRequest{DecayScore: 1.5})
	req := httptest.NewRequest("PATCH", "/api/v1/skills/skill-1/decay", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >1.0, got %d: %s", w.Code, w.Body.String())
	}

	// Test < 0.0
	body, _ = json.Marshal(UpdateDecayRequest{DecayScore: -0.1})
	req = httptest.NewRequest("PATCH", "/api/v1/skills/skill-1/decay", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for <0.0, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListByDecay_WithSkills(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	ctx := context.Background()
	for _, sk := range []model.SkillRecord{
		{ID: "sk-1", Name: "alpha", LibraryID: "lib1", Category: "tactical", DecayScore: 0.3},
		{ID: "sk-2", Name: "beta", LibraryID: "lib1", Category: "tactical", DecayScore: 0.1},
	} {
		if err := store.Put(ctx, &sk, nil); err != nil {
			t.Fatalf("seed skill %s: %v", sk.ID, err)
		}
		// Set decay scores via direct update
		if err := store.UpdateDecay(ctx, sk.ID, sk.DecayScore); err != nil {
			t.Fatalf("set decay %s: %v", sk.ID, err)
		}
	}

	req := httptest.NewRequest("GET", "/api/v1/skills/decay?library_id=lib1&limit=10", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DecayListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(resp.Skills))
	}
	// Should be ordered by decay_score ASC
	if resp.Skills[0].ID != "sk-2" {
		t.Fatalf("expected sk-2 first (lower decay), got %s", resp.Skills[0].ID)
	}
}

func TestUpdateDecay_InvalidJSON(t *testing.T) {
	srv := New(Config{Port: 0})
	req := httptest.NewRequest("PATCH", "/api/v1/skills/skill-1/decay", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
