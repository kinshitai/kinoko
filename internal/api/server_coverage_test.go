package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
)

func TestAddr(t *testing.T) {
	srv := New(Config{Host: "127.0.0.1", Port: 0})
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("expected non-empty addr")
	}
}

func TestStartAndStop(t *testing.T) {
	srv := New(Config{Host: "127.0.0.1", Port: 0})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverPOST_InvalidJSON(t *testing.T) {
	srv := New(Config{Port: 0})
	req := httptest.NewRequest("POST", "/api/v1/discover", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDiscoverPOST_HappyPath(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Embedder: &mockEmbedder{}, Store: store, SSHURL: "ssh://localhost:23231"})
	body, _ := json.Marshal(DiscoverRequest{Prompt: "test query", Limit: 5})
	req := httptest.NewRequest("POST", "/api/v1/discover", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDiscoverPOST_EmbedError(t *testing.T) {
	srv := New(Config{Port: 0, Embedder: &failEmbedder{}, Store: newTestStore(t)})
	body, _ := json.Marshal(DiscoverRequest{Prompt: "test", Limit: 5})
	req := httptest.NewRequest("POST", "/api/v1/discover", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

type failEmbedder struct{}

func (f *failEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embed failed")
}
func (f *failEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embed failed")
}
func (f *failEmbedder) Dimensions() int { return 8 }

func TestIngest_EnqueueError(t *testing.T) {
	srv := New(Config{
		Port: 0,
		Enqueue: func(_ context.Context, _ model.SessionRecord, _ []byte) error {
			return fmt.Errorf("queue full")
		},
	})
	body, _ := json.Marshal(IngestRequest{SessionID: "s1", Log: "data"})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestIngest_InvalidJSON(t *testing.T) {
	srv := New(Config{Port: 0})
	req := httptest.NewRequest("POST", "/api/v1/ingest", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHealthWithStore(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Store: store})
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HealthResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}
}

func TestDiscoverPOST_DefaultLimit(t *testing.T) {
	store := newTestStore(t)
	srv := New(Config{Port: 0, Embedder: &mockEmbedder{}, Store: store})
	body, _ := json.Marshal(DiscoverRequest{Prompt: "test", Limit: 0})
	req := httptest.NewRequest("POST", "/api/v1/discover", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
