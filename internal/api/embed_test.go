package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
