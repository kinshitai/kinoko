package serverclient

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestDoJSON_ErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.doJSON(context.Background(), "GET", "/test", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "bad request" {
		t.Errorf("expected message 'bad request', got %q", apiErr.Message)
	}
}

func TestHTTPEmbedder_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/embed" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req embedRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Text != "hello" {
			t.Errorf("expected text 'hello', got %q", req.Text)
		}
		json.NewEncoder(w).Encode(embedResponse{
			Vector: []float32{0.1, 0.2, 0.3},
			Model:  "test",
			Dims:   3,
		})
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(New(srv.URL), 3)
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Errorf("expected 3 dims, got %d", len(vec))
	}
	if e.Dimensions() != 3 {
		t.Errorf("expected Dimensions()=3, got %d", e.Dimensions())
	}
}

func TestHTTPEmbedder_EmbedBatch(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(embedResponse{Vector: []float32{1.0}, Dims: 1})
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(New(srv.URL), 1)
	vecs, err := e.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || calls != 2 {
		t.Errorf("expected 2 vecs and 2 calls, got %d and %d", len(vecs), calls)
	}
}

func TestHTTPSkillStore_Query(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Repo: "lib/test-skill", Name: "test-skill", Score: 0.95},
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
	if results[0].Skill.Name != "test-skill" {
		t.Errorf("expected skill name 'test-skill', got %q", results[0].Skill.Name)
	}
}

func TestHTTPQuerier_QueryNearest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "nearest-skill", Score: 0.92},
			},
		})
	}))
	defer srv.Close()

	q := NewHTTPQuerier(New(srv.URL))
	result, err := q.QueryNearest(context.Background(), []float32{0.1, 0.2}, "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillName != "nearest-skill" {
		t.Errorf("expected 'nearest-skill', got %q", result.SkillName)
	}
	if result.CosineSim != 0.92 {
		t.Errorf("expected 0.92, got %f", result.CosineSim)
	}
}

func TestHTTPQuerier_QueryNearest_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(discoverResponse{Skills: []struct {
			Repo        string  `json:"repo"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Score       float64 `json:"score"`
			CloneURL    string  `json:"clone_url"`
		}{}})
	}))
	defer srv.Close()

	q := NewHTTPQuerier(New(srv.URL))
	result, err := q.QueryNearest(context.Background(), []float32{0.1}, "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillName != "" {
		t.Errorf("expected empty skill name, got %q", result.SkillName)
	}
}

func TestGitPushCommitter_PathTraversal(t *testing.T) {
	log := slog.Default()
	g := NewGitPushCommitter("git@example.com:repo.git", t.TempDir(), log)

	cases := []struct {
		name    string
		library string
		skillID string
		wantErr string
	}{
		{"dotdot library", "../etc", "skill-1", "invalid libraryID"},
		{"slash library", "foo/bar", "skill-1", "invalid libraryID"},
		{"dotdot skill", "lib-1", "../../root", "invalid skill.ID"},
		{"slash skill", "lib-1", "a/b", "invalid skill.ID"},
		{"dot library", ".hidden", "skill-1", "invalid libraryID"},
		{"empty library", "", "skill-1", "invalid libraryID"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := g.CommitSkill(context.Background(), tc.library, &model.SkillRecord{
				ID: tc.skillID, Name: "test", Version: 1,
			}, []byte("# Test"))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestHTTPClient_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	// Override to a short timeout for test.
	c.httpClient.Timeout = 100 * time.Millisecond

	err := c.doJSON(context.Background(), "GET", "/slow", nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Should be a timeout-related error.
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestNew_HasTimeout(t *testing.T) {
	c := New("http://localhost")
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("expected timeout %v, got %v", defaultTimeout, c.httpClient.Timeout)
	}
}

// ── DiscoverClient tests for unified endpoint ──

func TestDiscoverClient_PromptOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Prompt != "test prompt" {
			t.Errorf("expected prompt 'test prompt', got %q", req.Prompt)
		}
		if len(req.Embedding) > 0 {
			t.Errorf("expected no embedding, got %v", req.Embedding)
		}
		if len(req.Patterns) > 0 {
			t.Errorf("expected no patterns, got %v", req.Patterns)
		}
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "prompt-skill", Score: 0.88, Description: "Found via prompt"},
			},
		})
	}))
	defer srv.Close()

	client := NewDiscoverClient(New(srv.URL))
	resp, err := client.Discover(context.Background(), discoverRequest{
		Prompt: "test prompt",
		TopK:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Name != "prompt-skill" {
		t.Errorf("expected 'prompt-skill', got %q", resp.Skills[0].Name)
	}
}

func TestDiscoverClient_EmbeddingOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Prompt != "" {
			t.Errorf("expected no prompt, got %q", req.Prompt)
		}
		if len(req.Embedding) != 3 {
			t.Errorf("expected 3-dim embedding, got %v", req.Embedding)
		}
		if len(req.Patterns) > 0 {
			t.Errorf("expected no patterns, got %v", req.Patterns)
		}
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "embedding-skill", Score: 0.92, Description: "Found via embedding"},
			},
		})
	}))
	defer srv.Close()

	client := NewDiscoverClient(New(srv.URL))
	resp, err := client.Discover(context.Background(), discoverRequest{
		Embedding: []float64{0.1, 0.2, 0.3},
		TopK:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Name != "embedding-skill" {
		t.Errorf("expected 'embedding-skill', got %q", resp.Skills[0].Name)
	}
}

func TestDiscoverClient_PatternsOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Prompt != "" {
			t.Errorf("expected no prompt, got %q", req.Prompt)
		}
		if len(req.Embedding) > 0 {
			t.Errorf("expected no embedding, got %v", req.Embedding)
		}
		if len(req.Patterns) != 2 || req.Patterns[0] != "go" || req.Patterns[1] != "testing" {
			t.Errorf("expected patterns [go, testing], got %v", req.Patterns)
		}
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "pattern-skill", Score: 0.85, Description: "Found via patterns"},
			},
		})
	}))
	defer srv.Close()

	client := NewDiscoverClient(New(srv.URL))
	resp, err := client.Discover(context.Background(), discoverRequest{
		Patterns: []string{"go", "testing"},
		TopK:     3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Name != "pattern-skill" {
		t.Errorf("expected 'pattern-skill', got %q", resp.Skills[0].Name)
	}
}

func TestDiscoverClient_AllEmpty_Returns400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Validate that all discovery parameters are empty
		if req.Prompt == "" && len(req.Embedding) == 0 && len(req.Patterns) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "at least one of prompt, embedding, or patterns is required",
			})
			return
		}

		// Should not reach here in this test
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(discoverResponse{Skills: []struct {
			Repo        string  `json:"repo"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Score       float64 `json:"score"`
			CloneURL    string  `json:"clone_url"`
		}{}})
	}))
	defer srv.Close()

	client := NewDiscoverClient(New(srv.URL))
	_, err := client.Discover(context.Background(), discoverRequest{
		// All parameters are empty - this should return 400
		TopK: 5,
	})

	// Should get an API error with status 400
	if err == nil {
		t.Fatal("expected error for empty request")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}
