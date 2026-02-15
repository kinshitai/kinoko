package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(url string) Config {
	return Config{
		Provider: "openai",
		Model:    "text-embedding-3-small",
		Dims:     3,
		BaseURL:  url,
		APIKey:   "test-key",
		Retry: RetryConfig{
			MaxRetries:     2,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     50 * time.Millisecond,
			BackoffFactor:  2.0,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 3,
			OpenDuration:     100 * time.Millisecond,
			HalfOpenMax:      1,
		},
	}
}

func makeResponse(texts int, dims int) embeddingResponse {
	resp := embeddingResponse{}
	for i := 0; i < texts; i++ {
		emb := make([]float32, dims)
		for j := range emb {
			emb[j] = float32(i+1) * 0.1
		}
		resp.Data = append(resp.Data, embeddingData{Index: i, Embedding: emb})
	}
	return resp
}

func TestEmbed_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("expected /v1/embeddings, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "text-embedding-3-small" {
			t.Errorf("expected model text-embedding-3-small, got %s", req.Model)
		}

		resp := makeResponse(len(req.Input), 3)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)

	emb, err := client.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(emb))
	}
}

func TestEmbedBatch_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) != 3 {
			t.Errorf("expected 3 inputs, got %d", len(req.Input))
		}
		resp := makeResponse(3, 3)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)
	results, err := client.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestEmbedBatch_Empty(t *testing.T) {
	client := New(testConfig("http://unused"), nil)
	results, err := client.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Fatalf("expected nil, got %v", results)
	}
}

func TestRetry_TransientFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"server error"}}`))
			return
		}
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp := makeResponse(len(req.Input), 3)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)
	emb, err := client.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(emb))
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestRetry_AllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"always fails"}}`))
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)
	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"fail"}}`))
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0 // no retries, one attempt per call
	client := New(cfg, nil)

	// Exhaust failure threshold (3 calls = 3 failures).
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Next call should fail immediately with circuit open.
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected circuit open error")
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"fail"}}`))
			return
		}
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp := makeResponse(len(req.Input), 3)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	cfg.CircuitBreaker.OpenDuration = 50 * time.Millisecond
	client := New(cfg, nil)

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Circuit is open — should fail fast.
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected circuit open error")
	}

	// Wait for open duration to elapse, fix server.
	time.Sleep(60 * time.Millisecond)
	shouldFail.Store(false)

	// Half-open: should allow one request and recover.
	emb, err := client.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected half-open recovery, got %v", err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(emb))
	}

	// Circuit should be closed now — subsequent calls work.
	emb, err = client.Embed(context.Background(), "test2")
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpenReopen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"still broken"}}`))
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	cfg.CircuitBreaker.OpenDuration = 50 * time.Millisecond
	client := New(cfg, nil)

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Wait for half-open.
	time.Sleep(60 * time.Millisecond)

	// Half-open test request fails → re-opens.
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected failure in half-open")
	}

	// Should be open again — fails fast.
	_, err = client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected circuit open after half-open failure")
	}
}

func TestDimensions(t *testing.T) {
	client := New(testConfig("http://unused"), nil)
	if client.Dimensions() != 3 {
		t.Fatalf("expected 3, got %d", client.Dimensions())
	}
}
