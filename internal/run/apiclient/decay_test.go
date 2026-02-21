package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestHTTPDecayClient_ListByDecay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/skills/decay" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("library_id") != "lib-1" {
			t.Errorf("expected library_id=lib-1, got %q", r.URL.Query().Get("library_id"))
		}
		json.NewEncoder(w).Encode(decayListResponse{
			Skills: []model.SkillRecord{
				{ID: "s1", Name: "skill-1", DecayScore: 0.8},
				{ID: "s2", Name: "skill-2", DecayScore: 0.3},
			},
		})
	}))
	defer srv.Close()

	dc := NewHTTPDecayClient(New(srv.URL))
	skills, err := dc.ListByDecay(context.Background(), "lib-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	if skills[0].ID != "s1" {
		t.Errorf("expected s1, got %q", skills[0].ID)
	}
}

func TestHTTPDecayClient_UpdateDecay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/skills/s1/decay" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req updateDecayRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.DecayScore != 0.5 {
			t.Errorf("expected decay_score 0.5, got %f", req.DecayScore)
		}
		json.NewEncoder(w).Encode(map[string]string{"updated": "s1"})
	}))
	defer srv.Close()

	dc := NewHTTPDecayClient(New(srv.URL))
	err := dc.UpdateDecay(context.Background(), "s1", 0.5)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPDecayClient_ListByDecay_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "db error"})
	}))
	defer srv.Close()

	dc := NewHTTPDecayClient(New(srv.URL))
	_, err := dc.ListByDecay(context.Background(), "lib-1", 10)
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
