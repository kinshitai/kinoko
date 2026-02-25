package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Additional test cases to improve coverage

func TestDoJSON_RequestBodyMarshalError(t *testing.T) {
	c := New("http://localhost")

	// Use a value that can't be marshaled to JSON
	invalidBody := make(chan int) // channels can't be marshaled

	err := c.doJSON(context.Background(), "POST", "/test", invalidBody, nil)
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshal request") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

func TestDoJSON_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow handler that should be cancelled
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.doJSON(ctx, "GET", "/slow", nil, nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "cancel") {
		t.Errorf("expected context cancellation error, got %v", err)
	}
}

func TestDoJSON_ResponseDecodingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write invalid JSON
		w.Write([]byte(`{"invalid": json malformed`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	var response map[string]string

	err := c.doJSON(context.Background(), "GET", "/invalid-json", nil, &response)
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestDoJSON_LargeResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		// Write a response larger than maxResponseBytes (1MB)
		largeResponse := strings.Repeat("x", maxResponseBytes+1000)
		w.Write([]byte(largeResponse))
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.doJSON(context.Background(), "GET", "/large", nil, nil)
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
}

func TestDoJSON_PostWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)

		if req["test"] != "value" {
			t.Errorf("expected test=value, got %v", req)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"response": "ok"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	requestBody := map[string]string{"test": "value"}
	var responseBody map[string]string

	err := c.doJSON(context.Background(), "POST", "/test", requestBody, &responseBody)
	if err != nil {
		t.Fatal(err)
	}
	if responseBody["response"] != "ok" {
		t.Errorf("expected response=ok, got %v", responseBody)
	}
}

func TestDoJSON_GetWithoutBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "" {
			t.Errorf("expected no content type for GET, got %s", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.doJSON(context.Background(), "GET", "/test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoJSON_NewRequestError(t *testing.T) {
	c := New("http://localhost")

	// Use an invalid method to trigger NewRequestWithContext error
	err := c.doJSON(context.Background(), "INVALID\nMETHOD", "/test", nil, nil)
	if err == nil {
		t.Fatal("expected request creation error")
	}
	if !strings.Contains(err.Error(), "create request") {
		t.Errorf("expected create request error, got %v", err)
	}
}

func TestHTTPQuerier_QueryNearest_EmptyLibraryID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.LibraryIDs) != 1 || req.LibraryIDs[0] != "" {
			t.Errorf("expected empty library ID, got %v", req.LibraryIDs)
		}

		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "global-skill", Score: 0.85},
			},
		})
	}))
	defer srv.Close()

	q := NewHTTPQuerier(New(srv.URL))
	result, err := q.QueryNearest(context.Background(), []float32{0.1, 0.2}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillName != "global-skill" {
		t.Errorf("expected 'global-skill', got %q", result.SkillName)
	}
	if result.CosineSim != 0.85 {
		t.Errorf("expected 0.85, got %f", result.CosineSim)
	}
}

func TestHTTPSkillStore_Query_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	ss := NewHTTPSkillStore(New(srv.URL))
	results, err := ss.Query(context.Background(), model.SkillQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestHTTPSkillStore_Query_WithPatternsAndMinQuality(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Patterns) != 2 || req.Patterns[0] != "go" || req.Patterns[1] != "web" {
			t.Errorf("expected patterns [go, web], got %v", req.Patterns)
		}
		if req.MinQuality != 0.8 {
			t.Errorf("expected min quality 0.8, got %f", req.MinQuality)
		}
		if len(req.LibraryIDs) != 1 || req.LibraryIDs[0] != "test-lib" {
			t.Errorf("expected library IDs [test-lib], got %v", req.LibraryIDs)
		}

		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Repo: "test-lib/web-skill", Name: "web-skill", Score: 0.9},
			},
		})
	}))
	defer srv.Close()

	ss := NewHTTPSkillStore(New(srv.URL))
	results, err := ss.Query(context.Background(), model.SkillQuery{
		Patterns:   []string{"go", "web"},
		LibraryIDs: []string{"test-lib"},
		MinQuality: 0.8,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Skill.LibraryID != "test-lib" {
		t.Errorf("expected libraryID 'test-lib', got %q", results[0].Skill.LibraryID)
	}
	if results[0].Skill.Name != "web-skill" {
		t.Errorf("expected name 'web-skill', got %q", results[0].Skill.Name)
	}
}

func TestHTTPSkillStore_Query_RepoSingleChar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Repo: "a", Name: "single-char-repo", Score: 0.8}, // Single character repo
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
	// For single char repo (no slash), name should fall back to skill.Name
	if results[0].Skill.Name != "single-char-repo" {
		t.Errorf("expected name 'single-char-repo', got %q", results[0].Skill.Name)
	}
	if results[0].Skill.LibraryID != "" {
		t.Errorf("expected empty libraryID, got %q", results[0].Skill.LibraryID)
	}
}

func TestHTTPSkillStore_Query_EmptyRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Repo: "", Name: "empty-repo-skill", Score: 0.6}, // Empty repo
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
	// For empty repo, name should fall back to skill.Name
	if results[0].Skill.Name != "empty-repo-skill" {
		t.Errorf("expected name 'empty-repo-skill', got %q", results[0].Skill.Name)
	}
	if results[0].Skill.LibraryID != "" {
		t.Errorf("expected empty libraryID, got %q", results[0].Skill.LibraryID)
	}
}

func TestDiscoverClient_Discover_CombinedParameters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req discoverRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Test all parameters together
		if req.Prompt != "test query" {
			t.Errorf("expected prompt 'test query', got %q", req.Prompt)
		}
		if len(req.Embedding) != 2 {
			t.Errorf("expected 2-dim embedding, got %v", req.Embedding)
		}
		if len(req.Patterns) != 1 || req.Patterns[0] != "golang" {
			t.Errorf("expected patterns [golang], got %v", req.Patterns)
		}
		if len(req.LibraryIDs) != 1 || req.LibraryIDs[0] != "core" {
			t.Errorf("expected library_ids [core], got %v", req.LibraryIDs)
		}
		if req.MinQuality != 0.7 {
			t.Errorf("expected min_quality 0.7, got %f", req.MinQuality)
		}
		if req.TopK != 15 {
			t.Errorf("expected top_k 15, got %d", req.TopK)
		}

		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo        string  `json:"repo"`
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Score       float64 `json:"score"`
				CloneURL    string  `json:"clone_url"`
			}{
				{Name: "comprehensive-skill", Score: 0.95, Description: "All params used"},
			},
		})
	}))
	defer srv.Close()

	client := NewDiscoverClient(New(srv.URL))
	resp, err := client.Discover(context.Background(), discoverRequest{
		Prompt:     "test query",
		Embedding:  []float64{0.1, 0.2},
		Patterns:   []string{"golang"},
		LibraryIDs: []string{"core"},
		MinQuality: 0.7,
		TopK:       15,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Name != "comprehensive-skill" {
		t.Errorf("expected 'comprehensive-skill', got %q", resp.Skills[0].Name)
	}
}
