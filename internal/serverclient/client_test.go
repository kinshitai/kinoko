package serverclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
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

func TestHTTPSessionWriter_InsertSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/sessions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req createSessionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Session.ID != "sess-1" {
			t.Errorf("expected session id 'sess-1', got %q", req.Session.ID)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "sess-1"})
	}))
	defer srv.Close()

	sw := NewHTTPSessionWriter(New(srv.URL))
	err := sw.InsertSession(context.Background(), &model.SessionRecord{
		ID:        "sess-1",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPSessionWriter_UpdateSessionResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/api/v1/sessions/sess-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body updateSessionBody
		json.NewDecoder(r.Body).Decode(&body)
		if body.ExtractionStatus != "extracted" {
			t.Errorf("expected status 'extracted', got %q", body.ExtractionStatus)
		}
		json.NewEncoder(w).Encode(map[string]string{"updated": "sess-1"})
	}))
	defer srv.Close()

	sw := NewHTTPSessionWriter(New(srv.URL))
	err := sw.UpdateSessionResult(context.Background(), "sess-1", &model.ExtractionResult{
		Status: model.StatusExtracted,
		Skill:  &model.SkillRecord{ID: "skill-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPSessionWriter_GetSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/sessions/sess-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(model.SessionRecord{ID: "sess-1"})
	}))
	defer srv.Close()

	sw := NewHTTPSessionWriter(New(srv.URL))
	sess, err := sw.GetSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "sess-1" {
		t.Errorf("expected id 'sess-1', got %q", sess.ID)
	}
}

func TestHTTPReviewer_WriteReviewSample(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/review-samples" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req createReviewSampleRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.SessionID != "sess-1" {
			t.Errorf("expected session_id 'sess-1', got %q", req.SessionID)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-1"})
	}))
	defer srv.Close()

	rv := NewHTTPReviewer(New(srv.URL))
	err := rv.WriteReviewSample(context.Background(), "sess-1", json.RawMessage(`{"test":true}`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPSkillStore_Query(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(searchResponse{
			Results: []model.ScoredSkill{
				{Skill: model.SkillRecord{ID: "s1", Name: "test-skill"}, CosineSim: 0.95},
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
		json.NewEncoder(w).Encode(searchResponse{
			Results: []model.ScoredSkill{
				{Skill: model.SkillRecord{Name: "nearest-skill"}, CosineSim: 0.92},
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
		json.NewEncoder(w).Encode(searchResponse{Results: []model.ScoredSkill{}})
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
