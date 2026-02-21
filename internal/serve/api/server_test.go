package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/serve/storage"
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

// TestDiscoverGET_MissingPrompt removed - GET /api/v1/discover endpoint removed in API consolidation

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

func TestIngest_MissingRepo(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(IngestRequest{})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestDiscoverGET_WithLimit removed - GET /api/v1/discover endpoint removed in API consolidation

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

func TestIngest_NilIndexFn_Returns501(t *testing.T) {
	srv := New(Config{Port: 0})
	body, _ := json.Marshal(IngestRequest{Repo: "local/test-skill"})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngest_WithIndexFn_Returns202(t *testing.T) {
	srv := New(Config{
		Port: 0,
		IndexFn: func(_ context.Context, repo, rev string) error {
			return nil
		},
	})
	body, _ := json.Marshal(IngestRequest{Repo: "local/test-skill", Rev: "abc123"})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngest_InvalidRepo_Returns400(t *testing.T) {
	srv := New(Config{Port: 0, IndexFn: func(_ context.Context, _, _ string) error { return nil }})
	for _, repo := range []string{"", "../etc/passwd", "UPPER/case", "no-slash"} {
		body, _ := json.Marshal(IngestRequest{Repo: repo})
		req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("repo=%q: expected 400, got %d", repo, w.Code)
		}
	}
}

func TestIngest_InvalidRev_Returns400(t *testing.T) {
	srv := New(Config{Port: 0, IndexFn: func(_ context.Context, _, _ string) error { return nil }})
	for _, rev := range []string{"not-hex!", "ABC123", "ab"} {
		body, _ := json.Marshal(IngestRequest{Repo: "local/skill", Rev: rev})
		req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("rev=%q: expected 400, got %d", rev, w.Code)
		}
	}
}
