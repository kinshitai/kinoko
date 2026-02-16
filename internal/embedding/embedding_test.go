package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"time"

	"github.com/kinoko-dev/kinoko/internal/circuitbreaker"
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
		_ = json.NewEncoder(w).Encode(resp)
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
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) != 3 {
			t.Errorf("expected 3 inputs, got %d", len(req.Input))
		}
		resp := makeResponse(3, 3)
		_ = json.NewEncoder(w).Encode(resp)
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
			_, _ = w.Write([]byte(`{"error":{"message":"server error"}}`))
			return
		}
		var req embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := makeResponse(len(req.Input), 3)
		_ = json.NewEncoder(w).Encode(resp)
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
		_, _ = w.Write([]byte(`{"error":{"message":"always fails"}}`))
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)
	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

// P1-1: Non-retryable errors (400, 401, 403, 404) return immediately.
func TestNoRetry_PermanentErrors(t *testing.T) {
	codes := []int{400, 401, 403, 404}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(code)
				w.Write([]byte(`{"error":{"message":"client error"}}`))
			}))
			defer srv.Close()

			client := New(testConfig(srv.URL), nil)
			_, err := client.Embed(context.Background(), "hello")
			if err == nil {
				t.Fatal("expected error")
			}
			if !IsPermanent(err) {
				t.Fatalf("expected permanent error, got %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("expected 1 call (no retry), got %d", calls.Load())
			}
		})
	}
}

// P1-1: 429 should be retried.
func TestRetry_429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		var req embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := makeResponse(len(req.Input), 3)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)
	emb, err := client.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected success after 429 retries, got %v", err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(emb))
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

// P1-2: 4xx errors should not trip the circuit breaker.
func TestCircuitBreaker_NotTrippedBy4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	client := New(cfg, nil)

	// Make more calls than the failure threshold.
	for i := 0; i < 5; i++ {
		_, err := client.Embed(context.Background(), "test")
		if err == nil {
			t.Fatal("expected error")
		}
		// Should always be permanent, never circuit open.
		if errors.Is(err, circuitbreaker.ErrOpen) {
			t.Fatalf("circuit breaker should not trip on 4xx (call %d)", i+1)
		}
	}
	// All 5 calls should have reached the server.
	if calls.Load() != 5 {
		t.Fatalf("expected 5 server calls, got %d", calls.Load())
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"fail"}}`))
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
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := makeResponse(len(req.Input), 3)
		_ = json.NewEncoder(w).Encode(resp)
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
	_, err = client.Embed(context.Background(), "test2")
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpenReopen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"still broken"}}`))
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

// P1-3: Open duration escalates on half-open failure.
func TestCircuitBreaker_EscalatingOpenDuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"still broken"}}`))
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	cfg.CircuitBreaker.OpenDuration = 50 * time.Millisecond
	cfg.CircuitBreaker.FailureThreshold = 3
	client := New(cfg, nil)

	// Trip the breaker: base open duration = 50ms.
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Wait for half-open (50ms).
	time.Sleep(60 * time.Millisecond)

	// Half-open test fails → re-open with 100ms.
	client.Embed(context.Background(), "test")

	// Verify: should still be open at 60ms (since new duration is 100ms).
	time.Sleep(60 * time.Millisecond)
	_, err := client.Embed(context.Background(), "test")
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected circuit still open (escalated to 100ms), got %v", err)
	}

	// Wait remaining time for 100ms total, then should transition to half-open.
	time.Sleep(50 * time.Millisecond)
	// Half-open test fails again → re-open with 200ms.
	_, err = client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected failure")
	}

	// Verify the open duration is now 200ms by checking it's still open after 100ms.
	time.Sleep(110 * time.Millisecond)
	_, err = client.Embed(context.Background(), "test")
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected circuit still open (escalated to 200ms), got %v", err)
	}
}

// P1-3: Open duration resets after successful recovery.
func TestCircuitBreaker_OpenDurationResetsOnRecovery(t *testing.T) {
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"fail"}}`))
			return
		}
		var req embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := makeResponse(len(req.Input), 3)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	cfg.CircuitBreaker.OpenDuration = 50 * time.Millisecond
	client := New(cfg, nil)

	// Trip, half-open fail (escalate to 100ms), then recover.
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}
	time.Sleep(60 * time.Millisecond)
	client.Embed(context.Background(), "test") // half-open fail → 100ms

	// Wait 110ms, fix server, recover.
	time.Sleep(110 * time.Millisecond)
	shouldFail.Store(false)
	_, err := client.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}

	// Check that open duration reset: trip again, should be 50ms not 100ms.
	shouldFail.Store(true)
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Verify open duration reset to base (50ms) by checking recovery timing.
	// After 60ms (> 50ms base), half-open should be available.
	time.Sleep(60 * time.Millisecond)
	shouldFail.Store(false)
	_, err = client.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected recovery at base duration (50ms), got %v", err)
	}
}

// P2-3: Dimension validation.
func TestDimensionValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return embeddings with wrong dimensions.
		resp := embeddingResponse{
			Data: []embeddingData{
				{Index: 0, Embedding: []float32{1.0, 2.0}}, // 2 dims instead of 3
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	client := New(cfg, nil)

	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

// P4-1: Context cancellation stops retry loop.
func TestContextCancellation(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"fail"}}`))
	}))
	defer srv.Close()

	client := New(testConfig(srv.URL), nil)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first attempt's backoff starts.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, err := client.Embed(ctx, "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDimensions(t *testing.T) {
	client := New(testConfig("http://unused"), nil)
	if client.Dimensions() != 3 {
		t.Fatalf("expected 3, got %d", client.Dimensions())
	}
}

func TestIsPermanent(t *testing.T) {
	pe := &permanentError{err: errors.New("bad request")}
	if !IsPermanent(pe) {
		t.Fatal("expected IsPermanent to be true")
	}
	if IsPermanent(errors.New("regular error")) {
		t.Fatal("expected IsPermanent to be false for regular error")
	}
}

// Compile-time interface check.
func TestInterfaceCompliance(t *testing.T) {
	var _ Embedder = (*Client)(nil)
}
