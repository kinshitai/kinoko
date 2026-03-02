package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterKey_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/register" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %s", ct)
		}

		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["public_key"] != "ssh-ed25519 AAAA" {
			t.Errorf("unexpected public_key: %s", req["public_key"])
		}
		if req["name"] != "my-host" {
			t.Errorf("unexpected name: %s", req["name"])
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered", "username": "my-host"})
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	if err := c.RegisterKey(context.Background(), "ssh-ed25519 AAAA", "my-host", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterKey_WithToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})

	// Without token → error
	if err := c.RegisterKey(context.Background(), "ssh-ed25519 AAAA", "host", ""); err == nil {
		t.Fatal("expected error without token")
	}

	// With token → success
	if err := c.RegisterKey(context.Background(), "ssh-ed25519 AAAA", "host", "my-secret"); err != nil {
		t.Fatalf("expected success with token: %v", err)
	}
}

func TestRegisterKey_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	err := c.RegisterKey(context.Background(), "ssh-ed25519 AAAA", "host", "")
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestRegisterKey_Unreachable(t *testing.T) {
	c := New(ClientConfig{APIURL: "http://127.0.0.1:1"})
	err := c.RegisterKey(context.Background(), "ssh-ed25519 AAAA", "host", "")
	if err == nil {
		t.Fatal("expected error on unreachable")
	}
}
