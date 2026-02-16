package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/embedding"
)

func TestHandleEmbed_Success(t *testing.T) {
	s := &Server{
		embedEngine: embedding.NewMockEngine(8),
		noveltyMux:  http.NewServeMux(),
	}

	body := `{"text":"hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleEmbed(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp EmbedResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Vector) != 8 {
		t.Errorf("expected 8 dims, got %d", len(resp.Vector))
	}
	if resp.Model != "mock" {
		t.Errorf("expected model 'mock', got %q", resp.Model)
	}
	if resp.Dims != 8 {
		t.Errorf("expected dims 8, got %d", resp.Dims)
	}
}

func TestHandleEmbed_NoEngine(t *testing.T) {
	s := &Server{}

	body := `{"text":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleEmbed(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleEmbed_EmptyText(t *testing.T) {
	s := &Server{
		embedEngine: embedding.NewMockEngine(8),
	}

	body := `{"text":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleEmbed(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleEmbed_InvalidJSON(t *testing.T) {
	s := &Server{
		embedEngine: embedding.NewMockEngine(8),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	s.handleEmbed(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleEmbed_BodyExceeds1MB(t *testing.T) {
	s := &Server{
		embedEngine: embedding.NewMockEngine(8),
		logger:      slog.Default(),
	}

	// Build a JSON payload whose text field exceeds the 1MB MaxBytesReader limit.
	bigText := strings.Repeat("A", 1<<20+1)
	body, _ := json.Marshal(EmbedRequest{Text: bigText})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleEmbed(w, req)

	// MaxBytesReader truncates the body → json.Decoder fails → 400.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbed_ConcurrentRequests(t *testing.T) {
	s := &Server{
		embedEngine: embedding.NewMockEngine(8),
		logger:      slog.Default(),
	}

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan string, n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			body := `{"text":"concurrent test"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewBufferString(body))
			w := httptest.NewRecorder()
			s.handleEmbed(w, req)
			if w.Code != http.StatusOK {
				errs <- w.Body.String()
			}
		}()
	}
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("concurrent request failed: %s", e)
	}
}

func TestHandleEmbed_ResponseStructure(t *testing.T) {
	dims := 16
	s := &Server{
		embedEngine: embedding.NewMockEngine(dims),
		noveltyMux:  http.NewServeMux(),
	}

	body := `{"text":"structure validation"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleEmbed(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp EmbedResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Vector length must equal the dims field and the engine's configured dims.
	if len(resp.Vector) != resp.Dims {
		t.Errorf("vector length %d != dims field %d", len(resp.Vector), resp.Dims)
	}
	if resp.Dims != dims {
		t.Errorf("response dims %d != engine dims %d", resp.Dims, dims)
	}
	if resp.Model == "" {
		t.Error("model field must not be empty")
	}
}
