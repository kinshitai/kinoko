package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Tests for the embedding circuit breaker state transitions.
// These mirror the stage3 circuit breaker tests to demonstrate duplication (tech debt C.1).

func TestEmbeddingCB_ClosedToOpenToHalfOpenToClosed(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"down"}}`))
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
	cfg.CircuitBreaker.FailureThreshold = 3
	cfg.CircuitBreaker.OpenDuration = 50 * time.Millisecond
	client := New(cfg, nil)

	// Closed → Open
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Verify open
	_, err := client.Embed(context.Background(), "test")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Open → Half-open → Closed
	time.Sleep(60 * time.Millisecond)
	emb, err := client.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("half-open probe should succeed: %v", err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(emb))
	}

	// Verify closed
	emb2, err := client.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("closed circuit should work: %v", err)
	}
	if len(emb2) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(emb2))
	}
}

func TestEmbeddingCB_HalfOpenFailEscalates(t *testing.T) {
	// Server always fails
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"down"}}`))
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Retry.MaxRetries = 0
	cfg.CircuitBreaker.FailureThreshold = 3
	cfg.CircuitBreaker.OpenDuration = 50 * time.Millisecond
	client := New(cfg, nil)

	// Trip breaker: base 50ms
	for i := 0; i < 3; i++ {
		client.Embed(context.Background(), "test")
	}

	// Half-open fail → escalate to 100ms
	time.Sleep(60 * time.Millisecond)
	client.Embed(context.Background(), "test")

	// Still open at 60ms (duration is now 100ms)
	time.Sleep(60 * time.Millisecond)
	_, err := client.Embed(context.Background(), "test")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected still open (escalated to 100ms), got %v", err)
	}

	// Half-open fail → escalate to 200ms
	time.Sleep(50 * time.Millisecond)
	client.Embed(context.Background(), "test")

	// Still open at 110ms (duration is now 200ms)
	time.Sleep(110 * time.Millisecond)
	_, err = client.Embed(context.Background(), "test")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected still open (escalated to 200ms), got %v", err)
	}
}
