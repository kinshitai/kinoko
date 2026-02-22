package injection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchWithMinScore_DefaultLimitAndScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[{"name":"s1","score":0.8,"description":"d1"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	// limit <= 0 and minScore <= 0 trigger defaults
	result, err := c.MatchWithMinScore(context.Background(), "test", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(result.Skills))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[]}`))
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
}
