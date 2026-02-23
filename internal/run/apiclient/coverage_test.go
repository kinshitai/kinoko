package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// ── APIError.Error() ──

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name   string
		err    APIError
		expect string
	}{
		{
			name:   "typical 404",
			err:    APIError{StatusCode: 404, Message: "not found"},
			expect: "server error 404: not found",
		},
		{
			name:   "empty message",
			err:    APIError{StatusCode: 500, Message: ""},
			expect: "server error 500: ",
		},
		{
			name:   "multiline message",
			err:    APIError{StatusCode: 422, Message: "line1\nline2"},
			expect: "server error 422: line1\nline2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

// ── repoLock mutex behavior ──

func TestGitPushCommitter_RepoLock(t *testing.T) {
	g := NewGitPushCommitter("git@example.com:repo.git", t.TempDir(), nil)

	// Same key must return the same mutex.
	m1 := g.repoLock("/path/a")
	m2 := g.repoLock("/path/a")
	if m1 != m2 {
		t.Fatal("expected same mutex for same key")
	}

	// Different keys must return different mutexes.
	m3 := g.repoLock("/path/b")
	if m1 == m3 {
		t.Fatal("expected different mutex for different key")
	}

	// Concurrent access should not race (run with -race).
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			mu := g.repoLock(key)
			mu.Lock()
			_ = mu // exercise lock acquisition under race detector
			mu.Unlock()
		}("/path/c")
	}
	wg.Wait()
}

// ── Query with repo field missing slash ──

func TestHTTPSkillStore_Query_RepoNoSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Repo: "noslash", Name: "fallback-name", Score: 0.7},
			},
		})
	}))
	defer srv.Close()

	ss := NewHTTPSkillStore(New(srv.URL))
	results, err := ss.Query(context.Background(), model.SkillQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// When repo has no slash, libraryID stays empty and name falls back to skill.Name.
	if results[0].Skill.Name != "fallback-name" {
		t.Errorf("expected name 'fallback-name', got %q", results[0].Skill.Name)
	}
	if results[0].Skill.LibraryID != "" {
		t.Errorf("expected empty libraryID, got %q", results[0].Skill.LibraryID)
	}
}

// ── Embed server returning 500 ──

func TestHTTPEmbedder_Embed_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "embedding service unavailable"})
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(New(srv.URL), 3)
	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected 500, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "embedding service unavailable" {
		t.Errorf("expected 'embedding service unavailable', got %q", apiErr.Message)
	}
}

// ── EmbedBatch propagates errors ──

func TestHTTPEmbedder_EmbedBatch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(New(srv.URL), 3)
	_, err := e.EmbedBatch(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error on batch")
	}
}

// ── SkillReader/SkillWriter (decay) happy + error via httptest ──

func TestHTTPDecayClient_UpdateDecay_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "write failed"})
	}))
	defer srv.Close()

	dc := NewHTTPDecayClient(New(srv.URL))
	err := dc.UpdateDecay(context.Background(), "s1", 0.5)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected 500, got %d", apiErr.StatusCode)
	}
}

// ── doJSON: non-JSON error body falls back to raw text ──

func TestDoJSON_NonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("upstream timeout"))
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.doJSON(context.Background(), "GET", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 502 {
		t.Errorf("expected 502, got %d", apiErr.StatusCode)
	}
	// When body is not JSON, Message should be the raw text.
	if apiErr.Message != "upstream timeout" {
		t.Errorf("expected 'upstream timeout', got %q", apiErr.Message)
	}
}

// ── Query with embedding conversion (float32 → float64) ──

func TestHTTPSkillStore_Query_WithEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Embedding) != 2 {
			t.Errorf("expected 2-dim embedding, got %d", len(req.Embedding))
		}
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Repo: "mylib/myskill", Name: "myskill", Score: 0.99},
			},
		})
	}))
	defer srv.Close()

	ss := NewHTTPSkillStore(New(srv.URL))
	results, err := ss.Query(context.Background(), model.SkillQuery{
		Embedding: []float32{0.5, 0.6},
		Limit:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Skill.LibraryID != "mylib" {
		t.Errorf("expected libraryID 'mylib', got %q", results[0].Skill.LibraryID)
	}
	if results[0].Skill.Name != "myskill" {
		t.Errorf("expected name 'myskill', got %q", results[0].Skill.Name)
	}
}
