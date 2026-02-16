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

// --- OpenAI CompleteWithTimeout tests ---

func TestOpenAI_CompleteWithTimeout_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "hello"}},
			},
			"usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 2},
		})
	}))
	defer srv.Close()

	c := NewOpenAIClient("key", "gpt-4o-mini", srv.URL)
	result, err := c.CompleteWithTimeout(context.Background(), "test", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "hello" {
		t.Fatalf("content = %q, want %q", result.Content, "hello")
	}
	if result.TokensIn != 5 || result.TokensOut != 2 {
		t.Fatalf("tokens: in=%d out=%d", result.TokensIn, result.TokensOut)
	}
}

func TestOpenAI_CompleteWithTimeout_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := NewOpenAIClient("key", "model", srv.URL)
	_, err := c.CompleteWithTimeout(context.Background(), "test", 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	var llmErr *LLMError
	if !errors.As(err, &llmErr) {
		t.Fatalf("expected *LLMError, got %T", err)
	}
	if llmErr.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", llmErr.StatusCode)
	}
}

func TestOpenAI_CompleteWithTimeout_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewOpenAIClient("key", "model", srv.URL)
	_, err := c.CompleteWithTimeout(context.Background(), "test", 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenAI_CompleteWithTimeout_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	c := NewOpenAIClient("key", "model", srv.URL)
	_, err := c.CompleteWithTimeout(context.Background(), "test", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestOpenAI_CompleteWithTimeout_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "late"}},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAIClient("key", "model", srv.URL)
	_, err := c.CompleteWithTimeout(context.Background(), "test", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// --- Anthropic additional coverage ---

func TestAnthropicClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(anthropicResponse{
			Error: &struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{Type: "invalid_request_error", Message: "bad input"},
		})
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "anthropic error: bad input" {
		t.Errorf("error = %q", got)
	}
}

func TestAnthropicClient_NoTextBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return content with non-text type only
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "tool_use", "text": ""},
			},
		})
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for no text content")
	}
}

func TestAnthropicClient_CompleteWithTimeout_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "late"}},
		})
	}))
	defer srv.Close()

	c := NewAnthropicClient("key", "model", srv.URL)
	_, err := c.CompleteWithTimeout(context.Background(), "test", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAnthropicClient_WithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 99 * time.Second}
	c := NewAnthropicClient("key", "model", "", WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Fatal("WithHTTPClient not applied to AnthropicClient")
	}
}

func TestNewClient_Providers(t *testing.T) {
	tests := []struct {
		provider string
		wantErr  bool
	}{
		{"openai", false},
		{"", false},
		{"anthropic", false},
		{"invalid", true},
	}
	for _, tt := range tests {
		c, err := NewClient(tt.provider, "key", "model", "")
		if tt.wantErr {
			if err == nil {
				t.Errorf("NewClient(%q): expected error", tt.provider)
			}
		} else {
			if err != nil {
				t.Errorf("NewClient(%q): %v", tt.provider, err)
			}
			if c == nil {
				t.Errorf("NewClient(%q): nil client", tt.provider)
			}
		}
	}
}
