package injection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Match_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		// Return discover response format
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "fix-db", Score: 0.9, Description: "# Fix DB\nDo this."},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	result, err := c.Match(context.Background(), "database timeout", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "fix-db" {
		t.Errorf("name = %q, want fix-db", result.Skills[0].Name)
	}
	if result.Skills[0].Score != 0.9 {
		t.Errorf("score = %f, want 0.9", result.Skills[0].Score)
	}
}

func TestClient_Match_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty discover response
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	result, err := c.Match(context.Background(), "nothing", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(result.Skills))
	}
}

func TestClient_Match_ServerDown(t *testing.T) {
	// Use a URL that won't connect.
	c := NewClient("http://127.0.0.1:1", nil)
	result, err := c.Match(context.Background(), "test", 5)
	if err != nil {
		t.Fatal("expected fail-open (no error), got:", err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills on fail-open, got %d", len(result.Skills))
	}
}

func TestClient_Match_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	result, err := c.Match(context.Background(), "test", 5)
	if err != nil {
		t.Fatal("expected fail-open, got:", err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills on server error, got %d", len(result.Skills))
	}
}
