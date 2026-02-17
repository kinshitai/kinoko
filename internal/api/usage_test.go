package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateUsage(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(UpdateUsageRequest{Outcome: "success"})
	req := httptest.NewRequest("POST", "/api/v1/skills/skill-1/usage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateUsage_InvalidJSON(t *testing.T) {
	srv := New(Config{Port: 0})
	req := httptest.NewRequest("POST", "/api/v1/skills/skill-1/usage", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
