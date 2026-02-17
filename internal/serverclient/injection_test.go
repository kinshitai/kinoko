package serverclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
)

func TestHTTPInjectionEventWriter_WriteInjectionEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/injection-events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var ev model.InjectionEventRecord
		json.NewDecoder(r.Body).Decode(&ev)
		if ev.ID != "ev-1" || ev.SessionID != "sess-1" {
			t.Errorf("unexpected event: %+v", ev)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "ev-1"})
	}))
	defer srv.Close()

	ew := NewHTTPInjectionEventWriter(New(srv.URL))
	err := ew.WriteInjectionEvent(context.Background(), model.InjectionEventRecord{
		ID:        "ev-1",
		SessionID: "sess-1",
		SkillID:   "skill-1",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPInjectionEventWriter_UpdateInjectionOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/api/v1/injection-events/sess-1/outcome" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req updateOutcomeRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Outcome != "success" {
			t.Errorf("expected outcome 'success', got %q", req.Outcome)
		}
		json.NewEncoder(w).Encode(map[string]string{"updated": "sess-1"})
	}))
	defer srv.Close()

	ew := NewHTTPInjectionEventWriter(New(srv.URL))
	err := ew.UpdateInjectionOutcome(context.Background(), "sess-1", "success")
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPInjectionEventWriter_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal"})
	}))
	defer srv.Close()

	ew := NewHTTPInjectionEventWriter(New(srv.URL))
	err := ew.WriteInjectionEvent(context.Background(), model.InjectionEventRecord{})
	if err == nil {
		t.Fatal("expected error")
	}
}
