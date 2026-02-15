package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := New(Config{Port: 0})
	// Use httptest to hit the handler directly
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}
}

func TestDiscoverGET_MissingPrompt(t *testing.T) {
	srv := New(Config{Port: 0})
	req := httptest.NewRequest("GET", "/api/v1/discover", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDiscoverPOST_MissingPrompt(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(DiscoverRequest{})
	req := httptest.NewRequest("POST", "/api/v1/discover", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestIngest_MissingFields(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(IngestRequest{})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestIngest_Success(t *testing.T) {
	// nil enqueue = no-op, still returns queued
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(IngestRequest{SessionID: "s1", Log: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
