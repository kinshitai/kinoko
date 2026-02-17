package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

	body, _ := json.Marshal(UpdateDecayRequest{DecayScore: 0.75})
	req := httptest.NewRequest("PATCH", "/api/v1/skills/skill-1/decay", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	// Will succeed even if skill doesn't exist (UPDATE affects 0 rows)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
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
