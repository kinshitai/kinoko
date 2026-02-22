package injection

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchWithMinScore_DefaultLimitAndScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return discoverResponse format (matches client_test.go pattern)
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "s1", Score: 0.8, Description: "d1"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	// limit <= 0 and minScore <= 0 trigger defaults
	result, err := c.MatchWithMinScore(context.Background(), "test", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "s1" {
		t.Errorf("name = %q, want s1", result.Skills[0].Name)
	}
	if result.Skills[0].Score != 0.8 {
		t.Errorf("score = %f, want 0.8", result.Skills[0].Score)
	}
}

func TestMatchWithMinScore_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	result, err := c.MatchWithMinScore(context.Background(), "test", 5, 0.5)
	if err != nil {
		t.Fatal("expected fail-open, got error:", err)
	}
	// Decode failure → fail open → empty result
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills on decode error, got %d", len(result.Skills))
	}
}

func TestMatchWithMinScore_NegativeParams(t *testing.T) {
	var capturedBody discoverRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		json.NewEncoder(w).Encode(discoverResponse{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	result, err := c.MatchWithMinScore(context.Background(), "test", -1, -0.5)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Verify negative params were clamped to defaults
	if capturedBody.TopK != 5 {
		t.Errorf("expected limit clamped to 5, got %d", capturedBody.TopK)
	}
	if capturedBody.MinQuality != 0.5 {
		t.Errorf("expected minScore clamped to 0.5, got %f", capturedBody.MinQuality)
	}
}
