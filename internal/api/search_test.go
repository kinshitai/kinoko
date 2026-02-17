package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearch_Empty(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})

	body, _ := json.Marshal(SearchRequest{Limit: 5})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp SearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Results == nil {
		t.Fatal("results should be non-nil")
	}
}

func TestSearch_InvalidJSON(t *testing.T) {
	srv := New(Config{Port: 0})
	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
