package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
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

// P1-11/P2-7: Discover GET with limit parameter — verifies route and param parsing.
func TestDiscoverGET_WithLimit(t *testing.T) {
	// Create a real store backed by in-memory SQLite for the happy path.
	store := newTestStore(t)
	srv := New(Config{Port: 0, Embedder: &mockEmbedder{}, Store: store})
	req := httptest.NewRequest("GET", "/api/v1/discover?q=test&limit=3", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	// With empty store, should return 200 with empty skills array.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DiscoverResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Skills == nil {
		t.Fatal("skills should be non-nil (empty array)")
	}
}

func newTestStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	s, err := storage.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mockEmbedder implements embedding.Embedder for testing.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 8), nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = make([]float32, 8)
	}
	return result, nil
}

func (m *mockEmbedder) Dimensions() int { return 8 }

// P1-7: Verify rate limit returns 429 (tested indirectly via structure).
func TestDiscoverRateLimitStructure(t *testing.T) {
	srv := New(Config{Port: 0})
	// Verify semaphore is properly initialized.
	if cap(srv.discoverSem) != 10 {
		t.Errorf("expected discover semaphore capacity 10, got %d", cap(srv.discoverSem))
	}
}

func TestIngest_NilEnqueue_Returns501(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(IngestRequest{SessionID: "s1", Log: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngest_WithEnqueue_Returns200(t *testing.T) {
	srv := New(Config{
		Port: 0,
		Enqueue: func(_ context.Context, _ model.SessionRecord, _ []byte) error {
			return nil
		},
	})
	body, _ := json.Marshal(IngestRequest{SessionID: "s1", Log: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
