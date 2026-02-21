package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAnthropicClient_Complete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("bad api key header: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("bad version header: %s", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("content-type") != "application/json" {
			t.Errorf("bad content-type: %s", r.Header.Get("content-type"))
		}

		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("unexpected model: %s", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}

		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "world"}},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{InputTokens: 5, OutputTokens: 1},
		})
	}))
	defer srv.Close()

	c := NewAnthropicClient("test-key", "claude-sonnet-4-20250514", srv.URL)
	result, err := c.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != "world" {
		t.Fatalf("got %q, want %q", result, "world")
	}
}

func TestAnthropicClient_CompleteWithTimeout_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "ok"}},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{InputTokens: 10, OutputTokens: 2},
		})
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	result, err := c.CompleteWithTimeout(context.Background(), "test", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "ok" {
		t.Fatalf("got %q, want %q", result.Content, "ok")
	}
	if result.TokensIn != 10 || result.TokensOut != 2 {
		t.Fatalf("tokens: in=%d out=%d, want 10/2", result.TokensIn, result.TokensOut)
	}
}

func TestAnthropicClient_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
	var llmErr *LLMError
	if !errors.As(err, &llmErr) {
		t.Fatalf("expected *LLMError, got %T: %v", err, err)
	}
	if llmErr.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", llmErr.StatusCode)
	}
}

func TestAnthropicClient_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestAnthropicClient_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestAnthropicClient_DefaultBaseURL(t *testing.T) {
	c := NewAnthropicClient("key", "model", "")
	if c.baseURL != defaultAnthropicBaseURL {
		t.Fatalf("baseURL = %q, want %q", c.baseURL, defaultAnthropicBaseURL)
	}
}
