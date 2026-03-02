package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister_Success(t *testing.T) {
	var calledPubkey, calledUsername string
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, pubkey, username string) error {
			calledPubkey = pubkey
			calledUsername = username
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test-key",
		Name:      "my-host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RegisterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "registered" {
		t.Errorf("expected status 'registered', got %q", resp.Status)
	}
	if resp.Username != "my-host" {
		t.Errorf("expected username 'my-host', got %q", resp.Username)
	}
	if calledPubkey != "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test-key" {
		t.Errorf("unexpected pubkey passed to registerFn: %q", calledPubkey)
	}
	if calledUsername != "my-host" {
		t.Errorf("unexpected username passed to registerFn: %q", calledUsername)
	}
}

func TestRegister_InvalidBody(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	tests := []struct {
		name string
		body string
	}{
		{"not json", "not json at all"},
		{"empty pubkey", `{"public_key":"","name":"host"}`},
		{"bad pubkey", `{"public_key":"not-ssh-key","name":"host"}`},
		{"empty name", `{"public_key":"ssh-ed25519 AAAA","name":""}`},
		{"invalid name chars", `{"public_key":"ssh-ed25519 AAAA","name":"bad name!"}`},
		{"name starts with hyphen", `{"public_key":"ssh-ed25519 AAAA","name":"-bad"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader([]byte(tt.body)))
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRegister_TokenRequired(t *testing.T) {
	srv := New(Config{
		Port:              0,
		RegistrationToken: "secret-token",
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAA",
		Name:      "host",
	})

	// No token → 401
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}

	// Wrong token → 401
	body, _ = json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAA",
		Name:      "host",
	})
	req = httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", w.Code)
	}
}

func TestRegister_TokenValid(t *testing.T) {
	srv := New(Config{
		Port:              0,
		RegistrationToken: "secret-token",
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAA",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_NotConfigured(t *testing.T) {
	srv := New(Config{Port: 0}) // No RegisterFn

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAA",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without registerFn, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_FnError(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("soft serve exploded")
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAA",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on registerFn error, got %d: %s", w.Code, w.Body.String())
	}
}
