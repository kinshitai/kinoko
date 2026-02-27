package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Final tests to push coverage as high as possible

func TestHTTPQuerier_QueryNearest_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection to simulate network error
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer srv.Close()

	q := NewHTTPQuerier(New(srv.URL))
	_, err := q.QueryNearest(context.Background(), []float32{0.1}, "lib1")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestHTTPQuerier_QueryNearest_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "server error"})
	}))
	defer srv.Close()

	q := NewHTTPQuerier(New(srv.URL))
	_, err := q.QueryNearest(context.Background(), []float32{0.1, 0.2}, "test-lib")
	if err == nil {
		t.Fatal("expected server error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected 500, got %d", apiErr.StatusCode)
	}
}

func TestHTTPSkillStore_Query_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer srv.Close()

	ss := NewHTTPSkillStore(New(srv.URL))
	_, err := ss.Query(context.Background(), model.SkillQuery{Limit: 5})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestHTTPSkillStore_Query_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("gateway error"))
	}))
	defer srv.Close()

	ss := NewHTTPSkillStore(New(srv.URL))
	_, err := ss.Query(context.Background(), model.SkillQuery{
		Embedding: []float32{0.1, 0.2},
		Limit:     5,
	})
	if err == nil {
		t.Fatal("expected server error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 502 {
		t.Errorf("expected 502, got %d", apiErr.StatusCode)
	}
}

func TestHTTPSkillStore_Query_ComplexRepoName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(discoverResponse{
			Skills: []struct {
				Repo           string  `json:"repo"`
				Name           string  `json:"name"`
				Description    string  `json:"description"`
				PatternOverlap float64 `json:"pattern_overlap"`
				CosineSim      float64 `json:"cosine_sim"`
				CloneURL       string  `json:"clone_url"`
			}{
				{Repo: "org/suborg/skill-name", Name: "skill-name", PatternOverlap: 0.9, CosineSim: 0.9}, // Complex repo path
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
	// Should parse the last slash for library/skill separation
	if results[0].Skill.LibraryID != "org/suborg" {
		t.Errorf("expected libraryID 'org/suborg', got %q", results[0].Skill.LibraryID)
	}
	if results[0].Skill.Name != "skill-name" {
		t.Errorf("expected name 'skill-name', got %q", results[0].Skill.Name)
	}
}

func TestDiscoverClient_Discover_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid parameters"})
	}))
	defer srv.Close()

	client := NewDiscoverClient(New(srv.URL))
	_, err := client.Discover(context.Background(), discoverRequest{
		Prompt: "test",
		TopK:   5,
	})
	if err == nil {
		t.Fatal("expected server error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestHTTPEmbedder_Embed_EmptyVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embedResponse{
			Vector: []float32{}, // Empty vector
			Model:  "test-model",
			Dims:   0,
		})
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(New(srv.URL), 0)
	vec, err := e.Embed(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 0 {
		t.Errorf("expected 0 dims, got %d", len(vec))
	}
	if e.Dimensions() != 0 {
		t.Errorf("expected Dimensions()=0, got %d", e.Dimensions())
	}
}

func TestHTTPEmbedder_EmbedBatch_EmptyInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call server for empty input")
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(New(srv.URL), 3)
	vecs, err := e.EmbedBatch(context.Background(), []string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 0 {
		t.Errorf("expected 0 vecs, got %d", len(vecs))
	}
}

// Test validateID function more thoroughly
func TestValidateID_AdditionalCases(t *testing.T) {
	testCases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid simple", "test", false},
		{"valid with hyphen", "test-skill", false},
		{"valid with underscore", "test_skill", false},
		{"valid with numbers", "skill123", false},
		{"valid mixed", "my_skill-v2", false},
		{"invalid starts with hyphen", "-invalid", true},
		{"invalid starts with underscore", "_invalid", true},
		{"valid starts with number", "123invalid", false},
		{"invalid with space", "test skill", true},
		{"invalid with slash", "test/skill", true},
		{"invalid with dot", "test.skill", true},
		{"invalid special chars", "test@skill", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateID("test", tc.id)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q", tc.id)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.id, err)
			}
		})
	}
}

// Test additional HTTP methods and edge cases
func TestDoJSON_PutMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusAccepted) // 202
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	var response map[string]string
	err := c.doJSON(context.Background(), "PUT", "/test", map[string]string{"data": "test"}, &response)
	if err != nil {
		t.Fatal(err)
	}
	if response["status"] != "accepted" {
		t.Errorf("expected status 'accepted', got %q", response["status"])
	}
}

func TestDoJSON_PatchMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent) // 204
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.doJSON(context.Background(), "PATCH", "/test", map[string]string{"data": "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoJSON_DeleteMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.doJSON(context.Background(), "DELETE", "/test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// Test API errors with different status codes
func TestDoJSON_VariousStatusCodes(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{"401 Unauthorized", 401, `{"error": "unauthorized"}`},
		{"403 Forbidden", 403, `{"error": "forbidden"}`},
		{"404 Not Found", 404, `{"error": "not found"}`},
		{"422 Unprocessable", 422, `{"error": "validation failed"}`},
		{"503 Unavailable", 503, `{"error": "service unavailable"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.body))
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
			if apiErr.StatusCode != tc.statusCode {
				t.Errorf("expected status %d, got %d", tc.statusCode, apiErr.StatusCode)
			}
		})
	}
}
