package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/apiclient"
)

func TestHTTPEmbedder_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/embed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["text"] != "hello world" {
			t.Errorf("unexpected text: %s", req["text"])
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"vector": []float32{0.1, 0.2, 0.3},
		})
	}))
	defer srv.Close()

	client := apiclient.New(srv.URL)
	e := apiclient.NewHTTPEmbedder(client, 3)
	vec, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("unexpected vector: %v", vec)
	}
}

func TestHTTPEmbedder_EmbedBatch(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"vector": []float32{float32(callCount)},
		})
	}))
	defer srv.Close()

	client := apiclient.New(srv.URL)
	e := apiclient.NewHTTPEmbedder(client, 1)
	vecs, err := e.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
	if callCount != 3 {
		t.Errorf("expected 3 API calls, got %d", callCount)
	}
}

func TestHTTPEmbedder_Embed_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := apiclient.New(srv.URL)
	e := apiclient.NewHTTPEmbedder(client, 3)
	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error on server error")
	}
}
