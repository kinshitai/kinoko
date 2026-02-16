package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{}
	c := NewOpenAIClient("key", "model", "", WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Fatal("WithHTTPClient did not set custom client")
	}
}

func TestComplete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("bad auth header: %s", r.Header.Get("Authorization"))
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "gpt-4o-mini" {
			t.Errorf("unexpected model: %v", req["model"])
		}

		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "hello world"}},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAIClient("test-key", "gpt-4o-mini", "", WithHTTPClient(srv.Client()))
	// Override the URL by using a custom transport that redirects to our test server.
	c.httpClient = srv.Client()

	// We need to override the URL. Since complete() hardcodes openai.com,
	// we'll use a custom http.Client with a transport that redirects.
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	result, err := c.Complete(context.Background(), "say hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello world" {
		t.Fatalf("got %q, want %q", result, "hello world")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestComplete_Non200(t *testing.T) {
	c := NewOpenAIClient("key", "model", "")
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(429)
			rec.Write([]byte("rate limited"))
			return rec.Result(), nil
		}),
	}

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

func TestComplete_BadJSON(t *testing.T) {
	c := NewOpenAIClient("key", "model", "")
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(200)
			rec.Write([]byte("not json"))
			return rec.Result(), nil
		}),
	}

	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestComplete_EmptyChoices(t *testing.T) {
	c := NewOpenAIClient("key", "model", "")
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(200)
			json.NewEncoder(rec).Encode(map[string]any{"choices": []any{}})
			return rec.Result(), nil
		}),
	}

	_, err := c.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}
